package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memory"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

type MemoryAdminHandler struct {
	bindings []MemoryBinding
	source   MemoryBindingSource
	audit    *services.AuditRecorder
	// agentMemory resolves the reserved hub-agent application's per-tenant
	// ledger（spec 2026-11-11-memory-mcp-design §4）——管理台按 agent scope
	// 查看 Agent 自主记忆，记录自带 source（写入者）。nil（knowledge 连接
	// 未配置的部署形态）时该分支返回 503。
	agentMemory *services.AgentMemoryRegistry
}

func NewMemoryAdminHandler(bindings []MemoryBinding, recorder *services.AuditRecorder) *MemoryAdminHandler {
	return &MemoryAdminHandler{bindings: append([]MemoryBinding(nil), bindings...), audit: recorder}
}

// SetAgentMemoryRegistry wires the per-tenant agent-memory registry; main.go
// 注入于 knowledge 连接可用时（registry 依赖检索引擎）。
func (h *MemoryAdminHandler) SetAgentMemoryRegistry(r *services.AgentMemoryRegistry) {
	h.agentMemory = r
}

func RegisterMemoryAdminRoutes(writeGroup, readGroup *gin.RouterGroup, h *MemoryAdminHandler) {
	// The general admin read group also admits members. Memory identity and
	// evidence are manager-only even if this helper receives that broad group.
	read := readGroup.Group("/memory", h.observe, middleware.RequireManager())
	write := writeGroup.Group("/memory", h.observe, middleware.RequireManager())
	read.GET("/applications", h.applications)
	read.GET("/subjects", h.subjects)
	read.GET("/facts", h.facts)
	read.GET("/native-messages", h.nativeMessages)
	read.GET("/native-config", h.nativeConfiguration)
	write.PATCH("/native-messages/:messageId", h.setNativeStatus)
	write.POST("/native-search", h.nativeSearch)
	write.POST("/native-context", h.nativeContext)
	read.GET("/operations/:operationId", h.operation)
	write.PATCH("/facts/:factId", h.update)
	write.DELETE("/facts/:factId", h.delete)
	write.POST("/consents/revoke", h.revoke)
	write.POST("/context", h.recall)
}

type memoryAdminApplication struct {
	memory.BackendCapabilities
	TenantID      string `json:"tenantId"`
	ApplicationID string `json:"applicationId"`
	PolicyVersion string `json:"policyVersion"`
	DataClass     string `json:"dataClass"`
	Name          string `json:"name,omitempty"`
	Status        string `json:"status,omitempty"`
	Origin        string `json:"origin,omitempty"`
}

func delegatedHubTenant(binding MemoryBinding) string {
	if binding.Client.HubTenantID != "" {
		return binding.Client.HubTenantID
	}
	return binding.Client.TenantID
}
func (h *MemoryAdminHandler) observe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
	if h.audit == nil {
		return
	}
	action := c.GetString("memory.admin.action")
	if action == "" {
		action = "memory.admin.access"
	}
	entry := audit.SimpleEvent(audit.Actor{}, audit.Action(action), audit.TargetType("memory"), c.GetString("memory.admin.target"), "")
	var nativeEnabled, nativeExpectedEnabled *bool
	if value, exists := c.Get("memory.admin.native.enabled"); exists {
		if enabled, ok := value.(bool); ok {
			nativeEnabled = &enabled
		}
	}
	if value, exists := c.Get("memory.admin.native.expectedEnabled"); exists {
		if enabled, ok := value.(bool); ok {
			nativeExpectedEnabled = &enabled
		}
	}
	entry.Detail = struct {
		Route           string `json:"route"`
		Method          string `json:"method"`
		HTTPStatus      int    `json:"httpStatus"`
		NativeMessageID string `json:"nativeMessageId,omitempty"`
		Enabled         *bool  `json:"enabled,omitempty"`
		ExpectedEnabled *bool  `json:"expectedEnabled,omitempty"`
	}{c.FullPath(), c.Request.Method, c.Writer.Status(), c.GetString("memory.admin.native.messageId"), nativeEnabled, nativeExpectedEnabled}
	if c.Writer.Status() >= 400 {
		entry.Status = audit.StatusFailure
	}
	h.audit.Record(c, entry)
}
func (h *MemoryAdminHandler) selectService(c *gin.Context, scope memory.Scope) (*memory.Service, bool) {
	hubTenant := tenant.GetTenantID(c)
	if hubTenant == "" {
		respondError(c, 401, "memory_admin_unauthorized", "需要登录后台后操作记忆")
		return nil, false
	}
	if !memoryHeaderID(scope.TenantID) || !memoryHeaderID(scope.ApplicationID) {
		respondError(c, 400, "memory_invalid_request", "需要有效的业务租户和应用标识")
		return nil, false
	}
	bindings, ok := h.currentBindings(c, hubTenant)
	if !ok {
		return nil, false
	}
	for _, binding := range bindings {
		if delegatedHubTenant(binding) != hubTenant || binding.Client.TenantID != scope.TenantID || binding.Client.ApplicationID != scope.ApplicationID {
			continue
		}
		if binding.Service == nil {
			respondError(c, 503, "memory_unavailable", "记忆服务暂不可用")
			return nil, false
		}
		encoded, _ := json.Marshal(scope)
		sum := sha256.Sum256(encoded)
		c.Set("memory.admin.target", hex.EncodeToString(sum[:]))
		return binding.Service, true
	}
	respondError(c, 403, "memory_admin_application_forbidden", "当前后台租户无权管理该记忆应用")
	return nil, false
}
func memoryAdminSubjectScope(c *gin.Context, scope memory.Scope) bool {
	if !memoryHeaderID(scope.UserID) || !memoryHeaderID(scope.LifecycleID) {
		respondError(c, 400, "memory_invalid_request", "需要有效的业务用户和账户生命周期标识")
		return false
	}
	return true
}
func memoryAdminQuery(c *gin.Context, subject bool) (memory.Scope, bool) {
	allowed := map[string]bool{"tenantId": true, "applicationId": true}
	if subject {
		allowed["userId"] = true
		allowed["accountLifecycleId"] = true
	}
	for key, values := range c.Request.URL.Query() {
		if !allowed[key] || len(values) != 1 {
			respondError(c, 400, "memory_invalid_request", "查询字段或重复参数无效")
			return memory.Scope{}, false
		}
	}
	scope := memory.Scope{TenantID: c.Query("tenantId"), ApplicationID: c.Query("applicationId"), UserID: c.Query("userId"), LifecycleID: c.Query("accountLifecycleId")}
	if subject && !memoryAdminSubjectScope(c, scope) {
		return scope, false
	}
	return scope, true
}
func memoryAdminBody(c *gin.Context, out any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	raw, err := io.ReadAll(c.Request.Body)
	raw = bytes.TrimSpace(raw)
	if err != nil || len(raw) == 0 || raw[0] != '{' {
		respondError(c, 400, "memory_invalid_request", "请求必须为单个 JSON 对象")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		respondError(c, 400, "memory_invalid_request", "请求字段或 JSON 无效")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		respondError(c, 400, "memory_invalid_request", "请求必须为单个 JSON 对象")
		return false
	}
	return true
}
func memoryAdminResult(c *gin.Context, out any, err error) {
	if err == nil {
		respondSuccess(c, out)
		return
	}
	switch {
	case errors.Is(err, memory.ErrUnsupported):
		respondError(c, http.StatusNotImplemented, "MEMORY_OPERATION_UNSUPPORTED", "当前应用未开放此记忆操作，请先查询应用能力")
	case errors.Is(err, memory.ErrNotFound):
		respondError(c, 404, "memory_not_found", "记忆记录不存在")
	case errors.Is(err, memory.ErrConflict):
		respondError(c, 409, "memory_conflict", "记忆版本、任务或幂等键冲突，请刷新后重试")
	case errors.Is(err, memory.ErrForbidden):
		respondError(c, 403, "memory_scope_inactive", "当前账户生命周期不可用")
	case errors.Is(err, memory.ErrInvalid):
		respondError(c, 400, "memory_invalid_request", "请求不符合当前合成数据记忆合同")
	case errors.Is(err, memory.ErrIndexUnknown):
		respondError(c, 503, "memory_outcome_unknown", "记忆状态尚未确认，请刷新核对后再操作")
	case errors.Is(err, context.DeadlineExceeded):
		respondError(c, 504, "memory_timeout", "记忆读取已超时，请重试")
	default:
		respondError(c, 503, "memory_unavailable", "记忆服务暂不可用")
	}
}
func (h *MemoryAdminHandler) applications(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.applications.read")
	hubTenant := tenant.GetTenantID(c)
	if hubTenant == "" {
		respondError(c, 401, "memory_admin_unauthorized", "需要登录后台后操作记忆")
		return
	}
	out := []memoryAdminApplication{}
	seen := map[string]bool{}
	bindings, ok := h.currentBindings(c, hubTenant)
	if !ok {
		return
	}
	for _, binding := range bindings {
		if delegatedHubTenant(binding) != hubTenant {
			continue
		}
		key := binding.Client.TenantID + "\x00" + binding.Client.ApplicationID
		if seen[key] {
			continue
		}
		seen[key] = true
		capabilities := memory.BackendCapabilities{}
		if binding.Service != nil {
			capabilities = binding.Service.Capabilities()
		}
		out = append(out, memoryAdminApplication{BackendCapabilities: capabilities, TenantID: binding.Client.TenantID, ApplicationID: binding.Client.ApplicationID, PolicyVersion: binding.Client.PolicyVersion, DataClass: "synthetic", Name: binding.Name, Status: binding.Status, Origin: binding.Origin})
	}
	respondSuccess(c, out)
}
func (h *MemoryAdminHandler) subjects(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.subjects.read")
	scope, ok := memoryAdminQuery(c, false)
	if !ok {
		return
	}
	service, ok := h.selectService(c, scope)
	if !ok {
		return
	}
	out, err := service.ListSubjects(c.Request.Context())
	memoryAdminResult(c, out, err)
}
func (h *MemoryAdminHandler) facts(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.facts.read")
	scope, ok := memoryAdminQuery(c, true)
	if !ok {
		return
	}
	// 保留应用 hub-agent（Agent 自主记忆，spec 2026-11-11 §4）：走 per-tenant
	// registry 而非业务应用绑定；ListAgentMemories 返回的记录自带 source
	//（写入者）。Agent 记忆归属 hub 租户自身——管理租户须与记忆租户一致。
	if scope.ApplicationID == services.AgentMemoryApplication {
		hubTenant := tenant.GetTenantID(c)
		if hubTenant == "" || hubTenant != scope.TenantID {
			respondError(c, 403, "memory_admin_forbidden", "无权查看该租户的 Agent 记忆")
			return
		}
		if h.agentMemory == nil {
			respondError(c, 503, "memory_unavailable", "记忆服务暂不可用")
			return
		}
		svc, err := h.agentMemory.Service(scope.TenantID)
		if err != nil {
			respondError(c, 503, "memory_unavailable", "记忆服务暂不可用")
			return
		}
		records, err := svc.ListAgentMemories(c.Request.Context(), scope, "", "", 50)
		if err != nil {
			memoryAdminResult(c, nil, err)
			return
		}
		// 映射为管理台既有 facts 契约形状（factId 键控动作弹窗），扩展
		// source（写入者）与 quote（来源摘录）两键——业务应用 facts 无此
		// 二键，前端按可选字段渲染。
		out := make([]map[string]any, 0, len(records))
		for _, r := range records {
			out = append(out, map[string]any{
				"factId": r.ID, "version": r.Version, "content": r.Content,
				"quote": r.Quote, "subject": r.Subject, "temporal": "",
				"category": r.Category, "status": "active", "syncStatus": "",
				"source": r.Source, "updatedAt": r.UpdatedAt,
			})
		}
		memoryAdminResult(c, out, nil)
		return
	}
	service, ok := h.selectService(c, scope)
	if !ok {
		return
	}
	out, err := service.List(c.Request.Context(), scope)
	memoryAdminResult(c, out, err)
}

type memoryAdminMutation struct {
	Scope           memory.Scope `json:"scope"`
	OperationID     string       `json:"operationId"`
	ExpectedVersion uint64       `json:"expectedVersion"`
}

func (h *MemoryAdminHandler) update(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.fact.update")
	var req struct {
		Scope           memory.Scope `json:"scope"`
		OperationID     string       `json:"operationId"`
		ExpectedVersion uint64       `json:"expectedVersion"`
		Content         string       `json:"content"`
	}
	if !memoryAdminBody(c, &req) || !memoryAdminSubjectScope(c, req.Scope) {
		return
	}
	service, ok := h.selectService(c, req.Scope)
	if !ok {
		return
	}
	out, err := service.Update(c.Request.Context(), req.Scope, c.Param("factId"), req.ExpectedVersion, req.Content, req.OperationID)
	memoryAdminResult(c, out, err)
}
func (h *MemoryAdminHandler) delete(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.fact.delete")
	var req memoryAdminMutation
	if !memoryAdminBody(c, &req) || !memoryAdminSubjectScope(c, req.Scope) {
		return
	}
	service, ok := h.selectService(c, req.Scope)
	if !ok {
		return
	}
	out, err := service.Delete(c.Request.Context(), req.Scope, c.Param("factId"), req.ExpectedVersion, req.OperationID)
	memoryAdminResult(c, out, err)
}
func (h *MemoryAdminHandler) revoke(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.consent.revoke")
	var req struct {
		Scope       memory.Scope `json:"scope"`
		OperationID string       `json:"operationId"`
		ConsentRef  string       `json:"consentRef"`
	}
	if !memoryAdminBody(c, &req) || !memoryAdminSubjectScope(c, req.Scope) {
		return
	}
	service, ok := h.selectService(c, req.Scope)
	if !ok {
		return
	}
	out, err := service.Revoke(c.Request.Context(), req.Scope, req.ConsentRef, req.OperationID)
	memoryAdminResult(c, out, err)
}
func (h *MemoryAdminHandler) recall(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.context.read")
	var req struct {
		Scope memory.Scope `json:"scope"`
		Query string       `json:"query"`
	}
	if !memoryAdminBody(c, &req) || !memoryAdminSubjectScope(c, req.Scope) {
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		respondError(c, 400, "memory_invalid_request", "检索问题不能为空")
		return
	}
	service, ok := h.selectService(c, req.Scope)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	out, err := service.Recall(ctx, req.Scope, req.Query)
	memoryAdminResult(c, out, err)
}
func (h *MemoryAdminHandler) operation(c *gin.Context) {
	c.Set("memory.admin.action", "memory.admin.operation.read")
	scope, ok := memoryAdminQuery(c, true)
	if !ok {
		return
	}
	if !memoryHeaderID(c.Param("operationId")) {
		respondError(c, 400, "memory_invalid_request", "操作标识无效")
		return
	}
	service, ok := h.selectService(c, scope)
	if !ok {
		return
	}
	out, err := service.GetOperation(c.Request.Context(), scope, c.Param("operationId"))
	memoryAdminResult(c, out, err)
}
