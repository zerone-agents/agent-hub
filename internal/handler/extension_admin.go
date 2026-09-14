// ExtensionAdminHandler 是 H7 扩展注册中心的管理 API。
//
// 全部端点走 /api/v1/admin 分组（RequireManager/RequireRole 中间件由
// cmd/server/main.go 接线时决定），并按租户隔离查询。注册端点的请求体
// 即扩展 manifest JSON（严格校验，中文错误）；重复内容哈希幂等返回既有版本。
package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ExtensionAdminHandler struct {
	service   *services.ExtensionService
	lifecycle *services.ExtensionLifecycleService // H7.1：生命周期操作；接线后可用
}

func NewExtensionAdminHandler(s *services.ExtensionService) *ExtensionAdminHandler {
	return &ExtensionAdminHandler{service: s}
}

// NewExtensionAdminHandlerWithLifecycle 返回带生命周期能力的 handler（H7.1）。
func NewExtensionAdminHandlerWithLifecycle(s *services.ExtensionService, l *services.ExtensionLifecycleService) *ExtensionAdminHandler {
	return &ExtensionAdminHandler{service: s, lifecycle: l}
}

// respondLifecycleError 把生命周期业务错误映射为对应的 HTTP 状态码与中文信息。
func respondLifecycleError(c *gin.Context, err error) {
	if code, ok := services.IsLifecycleError(err); ok {
		respondError(c, code, err.Error())
		return
	}
	respondError(c, http.StatusInternalServerError, err.Error())
}

// Register 注册扩展 + 版本：body 为严格模式 manifest JSON。
func (h *ExtensionAdminHandler) Register(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		respondError(c, http.StatusBadRequest, "请求体必须是扩展 manifest JSON")
		return
	}
	result, err := h.service.Register(tenant.GetTenantID(c), services.RegisterExtensionInput{
		Manifest:  raw,
		Source:    c.Query("source"),
		Changelog: c.Query("changelog"),
		CreatedBy: actorID(c),
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondCreated(c, result)
}

// List 扩展列表：tenant/status/source 过滤 + 分页。
func (h *ExtensionAdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	result, err := h.service.List(tenant.GetTenantID(c), services.ExtensionListFilter{
		Status:   strings.TrimSpace(c.Query("status")),
		Source:   strings.TrimSpace(c.Query("source")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, result)
}

// Get 扩展详情：版本列表 + 各版本 manifest 摘要 + 权限清单。
func (h *ExtensionAdminHandler) Get(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	detail, err := h.service.Get(tenant.GetTenantID(c), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "扩展不存在")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondSuccess(c, detail)
}

// GetVersion 单版本完整 manifest。
func (h *ExtensionAdminHandler) GetVersion(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
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
			respondError(c, http.StatusNotFound, "扩展版本不存在")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondSuccess(c, ver)
}

// parseExtensionID 解析路径参数 :id。
func parseExtensionID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}

// Install 安装扩展：body 可为 {"version":"1.0.0"}（幂等同版本返回 200 +
// idempotent:true；已安装其他版本返回 409 提示走升级）。
func (h *ExtensionAdminHandler) Install(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	var req struct {
		Version string `json:"version"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	if strings.TrimSpace(req.Version) == "" {
		req.Version = c.Query("version")
	}
	result, err := h.lifecycle.Install(tenant.GetTenantID(c), id, req.Version, actorID(c))
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, result)
}

// Enable 启用扩展（幂等）。
func (h *ExtensionAdminHandler) Enable(c *gin.Context) {
	h.setEnabled(c, true)
}

// Disable 停用扩展：新事件不再生效，历史数据依旧可读。
func (h *ExtensionAdminHandler) Disable(c *gin.Context) {
	h.setEnabled(c, false)
}

func (h *ExtensionAdminHandler) setEnabled(c *gin.Context, enabled bool) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	var inst any
	if enabled {
		inst, err = h.lifecycle.Enable(tenant.GetTenantID(c), id)
	} else {
		inst, err = h.lifecycle.Disable(tenant.GetTenantID(c), id)
	}
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, inst)
}

// Upgrade 升级扩展：body {"target_version":"2.0.0"}，含数据迁移（事务内执行）。
func (h *ExtensionAdminHandler) Upgrade(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	var req struct {
		TargetVersion string `json:"target_version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.TargetVersion) == "" {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON：{\"target_version\":\"x.y.z\"}")
		return
	}
	result, err := h.lifecycle.Upgrade(tenant.GetTenantID(c), id, req.TargetVersion)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, result)
}

// Rollback 回滚扩展：body 可空（默认回滚到上一版本）或
// {"target_version":"1.0.0"}。
func (h *ExtensionAdminHandler) Rollback(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	var req struct {
		TargetVersion string `json:"target_version"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	result, err := h.lifecycle.Rollback(tenant.GetTenantID(c), id, req.TargetVersion)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, result)
}

// Uninstall 卸载扩展：?force=true 先自动停用依赖方；?purge=true 额外删除
// 本租户版本数据。历史 run_states 数据始终保留。
func (h *ExtensionAdminHandler) Uninstall(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	force := c.Query("force") == "true" || c.Query("force") == "1"
	purge := c.Query("purge") == "true" || c.Query("purge") == "1"
	result, err := h.lifecycle.Uninstall(tenant.GetTenantID(c), id, force, purge)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, result)
}

// Impact 依赖影响范围预览：谁依赖它、它依赖谁、安装/升级后新增的
// stateSchemas 与权限。卸载前必显。
func (h *ExtensionAdminHandler) Impact(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	impact, err := h.lifecycle.Impact(tenant.GetTenantID(c), id)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, impact)
}
