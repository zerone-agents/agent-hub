// H7.3 模板库管理 API。
//
// 全部端点走 /api/v1/admin 分组（RequireManager/RequireRole 由
// cmd/server/main.go 接线决定），并按租户隔离查询（共享种子模板可读可装，
// 安装产物落在请求租户内）。错误映射：TemplateError 携带状态码，
// 校验失败 400、冲突/依赖缺失 409（附执行计划）、不存在 404。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TemplateAdminHandler struct {
	service *services.TemplateService
}

func NewTemplateAdminHandler(s *services.TemplateService) *TemplateAdminHandler {
	return &TemplateAdminHandler{service: s}
}

// respondTemplateError 把模板业务错误映射为对应 HTTP 状态码；409 时附带
// 执行计划（冲突/缺失依赖详情）。
func respondTemplateError(c *gin.Context, err error) {
	if te, ok := err.(*services.TemplateError); ok {
		if te.Plan != nil {
			c.JSON(te.Code, gin.H{"success": false, "error": te.Message, "plan": te.Plan})
			return
		}
		respondError(c, te.Code, te.Message)
		return
	}
	if code, ok := services.IsTemplateError(err); ok {
		respondError(c, code, err.Error())
		return
	}
	respondError(c, http.StatusInternalServerError, err.Error())
}

type registerTemplateRequest struct {
	Name        string                 `json:"name"`
	DisplayName string                 `json:"displayName"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Icon        string                 `json:"icon"`
	Version     string                 `json:"version"`
	Spec        map[string]interface{} `json:"spec"`
}

// Register 注册模板 + 版本：body 为模板元信息与 spec（严格校验，中文错误）。
func (h *TemplateAdminHandler) Register(c *gin.Context) {
	var req registerTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON："+err.Error())
		return
	}
	if len(req.Spec) == 0 {
		respondError(c, http.StatusBadRequest, "spec 不能为空")
		return
	}
	result, err := h.service.Register(tenant.GetTenantID(c), services.RegisterTemplateInput{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Category:    req.Category,
		Icon:        req.Icon,
		Version:     req.Version,
		Spec:        mustJSONRaw(req.Spec),
		Source:      c.Query("source"),
		CreatedBy:   actorID(c),
	})
	if err != nil {
		respondTemplateError(c, err)
		return
	}
	respondCreated(c, result)
}

// mustJSONRaw 序列化请求 spec（encoding/json 对 map key 排序，哈希稳定）。
func mustJSONRaw(v map[string]interface{}) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// List 模板列表：category 过滤 + 分页（含共享种子模板）。
func (h *TemplateAdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	result, err := h.service.List(tenant.GetTenantID(c), services.TemplateListFilter{
		Category: strings.TrimSpace(c.Query("category")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, result)
}

// Get 模板详情：版本列表 + spec 摘要（将创建什么）。
func (h *TemplateAdminHandler) Get(c *gin.Context) {
	id, err := parseTemplateID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid template id")
		return
	}
	detail, err := h.service.Get(tenant.GetTenantID(c), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "模板不存在")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondSuccess(c, detail)
}

// GetVersion 单版本完整 spec。
func (h *TemplateAdminHandler) GetVersion(c *gin.Context) {
	id, err := parseTemplateID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid template id")
		return
	}
	version := strings.TrimSpace(c.Param("version"))
	if version == "" {
		respondError(c, http.StatusBadRequest, "version is required")
		return
	}
	ver, err := h.service.GetVersion(tenant.GetTenantID(c), id, version)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "模板版本不存在")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondSuccess(c, ver)
}

type templateInstallRequest struct {
	Version        string                   `json:"version,omitempty"`
	Mapping        services.TemplateMapping `json:"mapping"`
	Sections       []string                 `json:"sections,omitempty"`
	Strategy       string                   `json:"strategy,omitempty"`
	Force          bool                     `json:"force,omitempty"`
	IdempotencyKey string                   `json:"idempotencyKey,omitempty"`
	TargetRunID    string                   `json:"targetRunId,omitempty"`
}

// Preview 安装预览：body 同 install（idempotencyKey/targetRunId 忽略），
// 返回执行计划（不落库）。
func (h *TemplateAdminHandler) Preview(c *gin.Context) {
	id, req, ok := h.bindInstallRequest(c)
	if !ok {
		return
	}
	plan, err := h.service.Resolve(tenant.GetTenantID(c), id, req.Version, req.Mapping, req.Sections, req.Strategy)
	if err != nil {
		respondTemplateError(c, err)
		return
	}
	respondSuccess(c, plan)
}

// Install 一键安装：sections 部分选择、mapping 模型映射+前缀、strategy
// 冲突策略、idempotencyKey 幂等重放、targetRunId 种子状态写入目标运行。
func (h *TemplateAdminHandler) Install(c *gin.Context) {
	id, req, ok := h.bindInstallRequest(c)
	if !ok {
		return
	}
	result, err := h.service.Install(tenant.GetTenantID(c), id, services.InstallOptions{
		Version:        req.Version,
		Mapping:        req.Mapping,
		Sections:       req.Sections,
		Strategy:       req.Strategy,
		Force:          req.Force,
		IdempotencyKey: req.IdempotencyKey,
		TargetRunID:    req.TargetRunID,
		Actor:          actorID(c),
	})
	if err != nil {
		respondTemplateError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *TemplateAdminHandler) bindInstallRequest(c *gin.Context) (uint64, *templateInstallRequest, bool) {
	id, err := parseTemplateID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid template id")
		return 0, nil, false
	}
	var req templateInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON："+err.Error())
		return 0, nil, false
	}
	return id, &req, true
}

type exportTemplateRequest struct {
	AgentIDs []uint64 `json:"agentIds,omitempty"`
	GroupIDs []string `json:"groupIds,omitempty"`
}

// Export 从当前租户配置反向导出模板 spec（可作为新模板注册）。
func (h *TemplateAdminHandler) Export(c *gin.Context) {
	id, err := parseTemplateID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid template id")
		return
	}
	_ = id // 导出作用域为整个租户，路径参数保留以兼容 REST 资源层级
	var req exportTemplateRequest
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	spec, err := h.service.Export(tenant.GetTenantID(c), services.ExportSelection{
		AgentIDs: req.AgentIDs,
		GroupIDs: req.GroupIDs,
	})
	if err != nil {
		respondTemplateError(c, err)
		return
	}
	respondSuccess(c, spec)
}

func parseTemplateID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
