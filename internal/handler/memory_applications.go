package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/config"
	"control-panel/internal/domain/audit"
	"control-panel/internal/memoryapp"
	"github.com/gin-gonic/gin"
)

// MemoryBindingSource resolves current admission from durable state on every
// request. A previously constructed service is not an authorization cache.
type MemoryBindingSource interface {
	Authenticate(context.Context, string) (MemoryBinding, error)
	BindingsForHubTenant(context.Context, string) ([]MemoryBinding, error)
}

func (h *MemoryHandler) SetBindingSource(source MemoryBindingSource)      { h.source = source }
func (h *MemoryAdminHandler) SetBindingSource(source MemoryBindingSource) { h.source = source }
func (h *MemoryAdminHandler) currentBindings(c *gin.Context, tenant string) ([]MemoryBinding, bool) {
	if h.source == nil {
		return h.bindings, true
	}
	bindings, err := h.source.BindingsForHubTenant(c.Request.Context(), tenant)
	if err != nil {
		respondError(c, 503, "memory_unavailable", "记忆应用配置暂不可用")
		return nil, false
	}
	return bindings, true
}

type MemoryApplicationHandler struct {
	registry *memoryapp.Registry
	managers []config.MemoryManager
	audit    *services.AuditRecorder
}

func NewMemoryApplicationHandler(registry *memoryapp.Registry, managers []config.MemoryManager, recorder *services.AuditRecorder) *MemoryApplicationHandler {
	return &MemoryApplicationHandler{registry: registry, managers: append([]config.MemoryManager(nil), managers...), audit: recorder}
}
func RegisterMemoryApplicationRoutes(r *gin.Engine, h *MemoryApplicationHandler) {
	g := r.Group("/api/v1/memory/applications", h.authenticate)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:applicationId", h.get)
	g.PATCH("/:applicationId", h.update)
	g.GET("/:applicationId/provisioning", h.provisioning)
}
func (h *MemoryApplicationHandler) authenticate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	parts := strings.Split(c.GetHeader("Authorization"), " ")
	selected := -1
	if len(parts) == 2 && parts[0] == "Bearer" {
		actual := sha256.Sum256([]byte(parts[1]))
		for i, m := range h.managers {
			expected := sha256.Sum256([]byte(m.Token))
			if subtle.ConstantTimeCompare(actual[:], expected[:]) == 1 {
				selected = i
			}
		}
	}
	if selected < 0 {
		memoryFailure(c, 401, "MEMORY_MANAGER_UNAUTHORIZED", "需要租户应用管理凭据")
		c.Abort()
		return
	}
	if !memoryHeaderID(c.GetHeader("X-Trace-Id")) {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "需要有效的 X-Trace-Id")
		c.Abort()
		return
	}
	c.Set("memory.trace", c.GetHeader("X-Trace-Id"))
	if h.registry == nil {
		memoryFailure(c, 503, "MEMORY_APPLICATION_UNAVAILABLE", "应用管理暂不可用")
		c.Abort()
		return
	}
	m := h.managers[selected]
	c.Set("memory.application.owner", memoryapp.Owner{TenantID: m.TenantID, HubTenantID: m.HubTenantID, ChatModel: m.ChatModel, EmbeddingModel: m.EmbeddingModel, MaxApplications: m.MaxApplications})
	c.Next()
	if h.audit != nil {
		applicationID := c.Param("applicationId")
		if applicationID == "" {
			applicationID = c.GetString("memory.application.id")
		}
		target := sha256.Sum256([]byte(m.TenantID + "\x00" + applicationID))
		entry := audit.SimpleEvent(audit.Actor{TenantID: m.TenantID, UserID: "memory-application-manager"}, audit.Action("memory.application.manage"), audit.TargetType("memory_application"), hex.EncodeToString(target[:]), "")
		entry.Detail = map[string]any{"route": c.FullPath(), "method": c.Request.Method, "httpStatus": c.Writer.Status(), "traceId": c.GetHeader("X-Trace-Id"), "applicationStatus": c.GetString("memory.application.status")}
		if c.Writer.Status() >= 400 {
			entry.Status = audit.StatusFailure
		}
		h.audit.Record(c, entry)
	}
}
func appOwner(c *gin.Context) memoryapp.Owner {
	return c.MustGet("memory.application.owner").(memoryapp.Owner)
}
func appResult(c *gin.Context, out any, err error) {
	if err == nil {
		memorySuccess(c, out, nil)
		return
	}
	switch {
	case errors.Is(err, memoryapp.ErrInvalid):
		memoryFailure(c, 400, "MEMORY_APPLICATION_INVALID", "应用管理请求无效")
	case errors.Is(err, memoryapp.ErrNotFound):
		memoryFailure(c, 404, "MEMORY_APPLICATION_NOT_FOUND", "应用不存在")
	case errors.Is(err, memoryapp.ErrConflict):
		memoryFailure(c, 409, "MEMORY_APPLICATION_CONFLICT", "应用标识、创建操作或版本冲突")
	case errors.Is(err, memoryapp.ErrQuota):
		memoryFailure(c, 409, "MEMORY_APPLICATION_QUOTA", "已达到租户应用数量上限")
	default:
		memoryFailure(c, 503, "MEMORY_APPLICATION_UNAVAILABLE", "应用管理暂不可用，请查询原创建操作的状态")
	}
}
func (h *MemoryApplicationHandler) create(c *gin.Context) {
	var in memoryapp.CreateInput
	if !memoryBody(c, &in) {
		return
	}
	c.Set("memory.application.id", in.ApplicationID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	out, err := h.registry.Create(ctx, appOwner(c), in)
	c.Set("memory.application.status", out.Application.Status)
	if errors.Is(err, memoryapp.ErrUnknown) {
		c.JSON(http.StatusAccepted, gin.H{"code": http.StatusAccepted, "data": out, "traceId": c.GetString("memory.trace")})
		return
	}
	appResult(c, out, err)
}
func (h *MemoryApplicationHandler) list(c *gin.Context) {
	offset, limit := 0, 50
	for key, values := range c.Request.URL.Query() {
		if len(values) != 1 || (key != "offset" && key != "limit") {
			appResult(c, nil, memoryapp.ErrInvalid)
			return
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			appResult(c, nil, memoryapp.ErrInvalid)
			return
		}
		if key == "offset" {
			offset = n
		} else {
			limit = n
		}
	}
	out, err := h.registry.List(c.Request.Context(), appOwner(c).TenantID, offset, limit)
	appResult(c, out, err)
}
func (h *MemoryApplicationHandler) read(c *gin.Context, reconcile bool) {
	if len(c.Request.URL.Query()) != 0 {
		appResult(c, nil, memoryapp.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	out, err := h.registry.Get(ctx, appOwner(c).TenantID, c.Param("applicationId"), reconcile)
	appResult(c, out, err)
}
func (h *MemoryApplicationHandler) get(c *gin.Context)          { h.read(c, false) }
func (h *MemoryApplicationHandler) provisioning(c *gin.Context) { h.read(c, true) }
func (h *MemoryApplicationHandler) update(c *gin.Context) {
	var in memoryapp.UpdateInput
	if !memoryBody(c, &in) {
		return
	}
	out, err := h.registry.Update(c.Request.Context(), appOwner(c).TenantID, c.Param("applicationId"), in)
	appResult(c, out, err)
}
