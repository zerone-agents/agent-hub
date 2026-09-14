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
	service *services.ExtensionService
}

func NewExtensionAdminHandler(s *services.ExtensionService) *ExtensionAdminHandler {
	return &ExtensionAdminHandler{service: s}
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

func parseExtensionID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
