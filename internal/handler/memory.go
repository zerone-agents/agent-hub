package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/config"
	"control-panel/internal/domain/audit"
	"control-panel/internal/memory"

	"github.com/gin-gonic/gin"
)

// A credential belongs to one trusted Java application, never an App user.
// User/lifecycle headers are assertions from that service, checked against the
// registered lifecycle by every ledger operation.
type MemoryBinding struct {
	Client  config.MemoryClient
	Service *memory.Service
}

type MemoryHandler struct {
	bindings []MemoryBinding
	audit    *services.AuditRecorder
}

func NewMemoryHandler(bindings []MemoryBinding, recorder *services.AuditRecorder) *MemoryHandler {
	return &MemoryHandler{bindings: append([]MemoryBinding(nil), bindings...), audit: recorder}
}

func RegisterMemoryRoutes(r *gin.Engine, h *MemoryHandler) {
	g := r.Group("/api/v1/memory", h.authenticate)
	g.GET("/capabilities", h.capabilities)
	g.POST("/conversations", h.ingestConversation)
	g.GET("/conversations/:operationId", h.conversationStatus)
	g.GET("/native-messages", h.nativeMessages)
	g.POST("/native-context", h.nativeContext)
	g.POST("/lifecycle/activate", h.activate)
	g.POST("/lifecycle/deactivate", h.deactivate)
	g.POST("/candidates", h.candidate)
	g.POST("/candidates/:candidateId/reject", h.rejectCandidate)
	g.POST("/tasks/claim", h.claim)
	g.POST("/tasks/publish", h.publish)
	g.GET("/facts", h.list)
	g.GET("/facts/:factId", h.get)
	g.PATCH("/facts/:factId", h.update)
	g.DELETE("/facts/:factId", h.delete)
	g.POST("/consents/revoke", h.revoke)
	g.POST("/context", h.recall)
	g.POST("/context/validate", h.validate)
	g.GET("/operations/:operationId", h.operation)
}

func (h *MemoryHandler) authenticate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	parts := strings.Split(c.GetHeader("Authorization"), " ")
	selected := -1
	if len(parts) == 2 && parts[0] == "Bearer" {
		actual := sha256.Sum256([]byte(parts[1]))
		for i, b := range h.bindings {
			expected := sha256.Sum256([]byte(b.Client.Token))
			if subtle.ConstantTimeCompare(actual[:], expected[:]) == 1 {
				selected = i
			}
		}
	}
	if selected < 0 {
		memoryFailure(c, http.StatusUnauthorized, "MEMORY_UNAUTHORIZED", "服务鉴权失败")
		c.Abort()
		return
	}
	b := h.bindings[selected]
	trace := c.GetHeader("X-Trace-Id")
	if !memoryHeaderID(trace) {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "需要有效的 X-Trace-Id")
		c.Abort()
		return
	}
	c.Set("memory.trace", trace)
	sc := memory.Scope{TenantID: b.Client.TenantID, ApplicationID: b.Client.ApplicationID, UserID: c.GetHeader("X-Memory-User-Id"), LifecycleID: c.GetHeader("X-Memory-Lifecycle-Id")}
	if c.FullPath() != "/api/v1/memory/capabilities" && (!memoryHeaderID(sc.UserID) || !memoryHeaderID(sc.LifecycleID)) {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "需要有效的用户和账户生命周期标识")
		c.Abort()
		return
	}
	c.Set("memory.scope", sc)
	c.Set("memory.service", b.Service)
	c.Next()
	if h.audit != nil {
		scopeJSON, _ := json.Marshal(sc)
		digest := sha256.Sum256(scopeJSON)
		appDigest := sha256.Sum256([]byte(sc.ApplicationID))
		entry := audit.SimpleEvent(audit.Actor{TenantID: sc.TenantID, UserID: "memory-service:" + hex.EncodeToString(appDigest[:16])}, audit.Action("memory.access"), audit.TargetType("memory"), hex.EncodeToString(digest[:]), "")
		entry.Detail = map[string]any{"route": c.FullPath(), "method": c.Request.Method, "httpStatus": c.Writer.Status(), "traceId": trace}
		if c.Writer.Status() >= 400 {
			entry.Status = audit.StatusFailure
		}
		h.audit.Record(c, entry)
	}
}

func memoryHeaderID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

func memoryRequest(c *gin.Context) (*memory.Service, memory.Scope) {
	svc := c.MustGet("memory.service").(*memory.Service)
	return svc, c.MustGet("memory.scope").(memory.Scope)
}

func memoryBody(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	body, err := io.ReadAll(c.Request.Body)
	body = bytes.TrimSpace(body)
	if err != nil || len(body) == 0 || body[0] != '{' {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "请求必须为 JSON 对象")
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "请求字段或 JSON 无效")
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "请求必须为单个 JSON 对象")
		return false
	}
	return true
}

func memorySuccess(c *gin.Context, data any, err error) {
	if err != nil {
		switch {
		case errors.Is(err, memory.ErrUnsupported):
			memoryFailure(c, http.StatusNotImplemented, "MEMORY_OPERATION_UNSUPPORTED", "当前应用未开放此记忆操作，请先查询应用能力")
		case errors.Is(err, memory.ErrNotFound):
			memoryFailure(c, 404, "MEMORY_NOT_FOUND", "记录不存在")
		case errors.Is(err, memory.ErrConflict):
			memoryFailure(c, 409, "MEMORY_CONFLICT", "版本、任务或幂等键冲突")
		case errors.Is(err, memory.ErrForbidden):
			memoryFailure(c, 403, "MEMORY_SCOPE_INACTIVE", "当前账户生命周期不可用")
		case errors.Is(err, memory.ErrInvalid):
			memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "请求不符合当前合成数据联调合同")
		case errors.Is(err, memory.ErrIndexUnknown):
			memoryFailure(c, 503, "MEMORY_OUTCOME_UNKNOWN", "索引结果尚未确认，请查询操作状态")
		default:
			memoryFailure(c, 503, "MEMORY_UNAVAILABLE", "记忆服务暂不可用")
		}
		return
	}
	c.JSON(200, gin.H{"code": 200, "data": data, "traceId": c.GetString("memory.trace")})
}

func memoryFailure(c *gin.Context, status int, reason, message string) {
	c.JSON(status, gin.H{"code": status, "error": gin.H{"reason": reason, "message": message}, "traceId": c.GetString("memory.trace")})
}

func (h *MemoryHandler) capabilities(c *gin.Context) {
	service, _ := memoryRequest(c)
	capabilities := service.Capabilities()
	memorySuccess(c, gin.H{"contractVersion": "memory-v1-synthetic", "dataClass": "synthetic", "sensitiveData": false, "kms": false, "maxFactsPerContext": 5, "maxContextChars": 200, "maxProfileChars": 300, "characterCounting": "unicode-code-points", "runtimeHistory": "separate-runtime-contract", "scope": "credential-bound-application-and-registered-lifecycle", "backend": capabilities.Backend, "rawOnly": capabilities.RawOnly, "automaticExtractionSupported": capabilities.AutomaticExtractionSupported, "nativeConfigurationSupported": capabilities.NativeConfigurationSupported, "nativeStatusSupported": capabilities.NativeStatusSupported, "nativeDiagnosticsSupported": capabilities.NativeDiagnosticsSupported, "nativeMaxContextChars": capabilities.NativeMaxContextChars, "nativeDefaultMessages": capabilities.NativeDefaultMessages, "nativeMaxMessages": capabilities.NativeMaxMessages, "conversationCorrectionsSupported": capabilities.ConversationCorrectionsSupported, "memoryTypes": capabilities.MemoryTypes, "correctionSupported": capabilities.CorrectionSupported, "forgetSupported": capabilities.ForgetSupported, "physicalDeleteSupported": capabilities.PhysicalDeleteSupported, "consentRevokeSupported": capabilities.ConsentRevokeSupported, "deactivateSupported": capabilities.DeactivateSupported}, nil)
}

type memoryOperationBody struct {
	OperationID string `json:"operationId"`
}

func (h *MemoryHandler) activate(c *gin.Context) {
	var req memoryOperationBody
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Activate(c.Request.Context(), sc, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) deactivate(c *gin.Context) {
	var req memoryOperationBody
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Deactivate(c.Request.Context(), sc, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) candidate(c *gin.Context) {
	var req memory.CandidateInput
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.SubmitCandidate(c.Request.Context(), sc, req)
	memorySuccess(c, out, err)
}

func (h *MemoryHandler) rejectCandidate(c *gin.Context) {
	var req struct {
		OperationID     string `json:"operationId"`
		ExpectedVersion uint64 `json:"expectedVersion"`
		Reason          string `json:"reason"`
	}
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.RejectCandidate(c.Request.Context(), sc, c.Param("candidateId"), req.ExpectedVersion, req.Reason, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) claim(c *gin.Context) {
	var req memoryOperationBody
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.ClaimTask(c.Request.Context(), sc, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) publish(c *gin.Context) {
	var req memory.PublishInput
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Publish(c.Request.Context(), sc, req)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) list(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "本版列表不接受筛选参数")
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.List(c.Request.Context(), sc)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) get(c *gin.Context) {
	svc, sc := memoryRequest(c)
	out, err := svc.Get(c.Request.Context(), sc, c.Param("factId"))
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) update(c *gin.Context) {
	var req struct {
		OperationID     string `json:"operationId"`
		ExpectedVersion uint64 `json:"expectedVersion"`
		Content         string `json:"content"`
	}
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Update(c.Request.Context(), sc, c.Param("factId"), req.ExpectedVersion, req.Content, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) delete(c *gin.Context) {
	var req struct {
		OperationID     string `json:"operationId"`
		ExpectedVersion uint64 `json:"expectedVersion"`
	}
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Delete(c.Request.Context(), sc, c.Param("factId"), req.ExpectedVersion, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) revoke(c *gin.Context) {
	var req struct {
		OperationID string `json:"operationId"`
		ConsentRef  string `json:"consentRef"`
	}
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Revoke(c.Request.Context(), sc, req.ConsentRef, req.OperationID)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) recall(c *gin.Context) {
	var req struct {
		Query      string   `json:"query"`
		Categories []string `json:"categories,omitempty"`
		DeadlineMS int      `json:"deadlineMs,omitempty"`
	}
	if !memoryBody(c, &req) {
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "query 不能为空")
		return
	}
	if req.DeadlineMS == 0 {
		req.DeadlineMS = 500
	}
	if req.DeadlineMS < 50 || req.DeadlineMS > 2000 || len(req.Categories) > 4 {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "读取期限需为50至2000毫秒，类别最多4项")
		return
	}
	svc, sc := memoryRequest(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(req.DeadlineMS)*time.Millisecond)
	defer cancel()
	out, err := svc.RecallCategories(ctx, sc, req.Query, req.Categories)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) validate(c *gin.Context) {
	var req struct {
		ContextVersion uint64 `json:"contextVersion"`
	}
	if !memoryBody(c, &req) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.Validate(c.Request.Context(), sc, req.ContextVersion)
	memorySuccess(c, gin.H{"valid": out}, err)
}
func (h *MemoryHandler) operation(c *gin.Context) {
	svc, sc := memoryRequest(c)
	out, err := svc.GetOperation(c.Request.Context(), sc, c.Param("operationId"))
	memorySuccess(c, out, err)
}
