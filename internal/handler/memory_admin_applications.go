package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"control-panel/internal/domain/provider"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memoryapp"

	"github.com/gin-gonic/gin"
)

func (h *MemoryAdminHandler) SetApplicationManagement(registry *memoryapp.Registry, models provider.MultiRAGMyLLMsSource) {
	h.applicationsRegistry, h.applicationModels = registry, models
}

func adminApplicationError(c *gin.Context, err error) {
	status, code, message := 503, "memory_application_unavailable", "记忆应用管理暂不可用，请核对原创建操作"
	switch {
	case errors.Is(err, memoryapp.ErrInvalid):
		status, code, message = 400, "memory_application_invalid", "应用标识、名称或创建参数无效"
	case errors.Is(err, memoryapp.ErrNotFound):
		status, code, message = 404, "memory_application_not_found", "记忆应用不存在"
	case errors.Is(err, memoryapp.ErrConflict):
		status, code, message = 409, "memory_application_conflict", "应用标识或创建操作冲突，请核对已有应用"
	case errors.Is(err, memoryapp.ErrQuota):
		status, code, message = 409, "memory_application_quota", "已达到当前租户的记忆应用数量上限"
	}
	respondError(c, status, code, message)
}

type memoryApplicationModels struct {
	Chat      []MultiRAGModel `json:"chat"`
	Embedding []MultiRAGModel `json:"embedding"`
}

func (h *MemoryAdminHandler) availableApplicationModels(ctx context.Context) (memoryApplicationModels, error) {
	out := memoryApplicationModels{Chat: []MultiRAGModel{}, Embedding: []MultiRAGModel{}}
	if h.applicationModels == nil {
		return out, memoryapp.ErrUnavailable
	}
	raw, err := h.applicationModels.ListMyLLMs(ctx)
	if err != nil {
		return out, memoryapp.ErrUnavailable
	}
	for kind, target := range map[string]*[]MultiRAGModel{"chat": &out.Chat, "embedding": &out.Embedding} {
		models, err := projectMultiRAGModels(raw, kind)
		if err != nil {
			return out, memoryapp.ErrUnavailable
		}
		for _, model := range models {
			if model.Status == "1" {
				*target = append(*target, model)
			}
		}
	}
	return out, nil
}

func (h *MemoryAdminHandler) applicationManager(c *gin.Context) (string, bool) {
	hub := tenant.GetTenantID(c)
	if !memoryHeaderID(hub) {
		respondError(c, 401, "memory_admin_unauthorized", "需要登录后台后管理记忆应用")
		return "", false
	}
	if h.applicationsRegistry == nil {
		respondError(c, 503, "memory_unavailable", "记忆应用管理暂不可用")
		return "", false
	}
	return hub, true
}

func (h *MemoryAdminHandler) listApplicationModels(c *gin.Context) {
	c.Set("memory.admin.action", "memory.application.models.read")
	if _, ok := h.applicationManager(c); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	out, err := h.availableApplicationModels(ctx)
	if err != nil {
		respondError(c, 503, "memory_models_unavailable", "无法加载记忆服务模型，请检查服务连接后重试")
		return
	}
	respondSuccess(c, out)
}

type memoryAdminCreateApplication struct {
	OperationID    string `json:"operationId"`
	ApplicationID  string `json:"applicationId"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ChatModel      string `json:"chatModel"`
	EmbeddingModel string `json:"embeddingModel"`
}

func (h *MemoryAdminHandler) createApplication(c *gin.Context) {
	c.Set("memory.admin.action", "memory.application.create")
	hub, ok := h.applicationManager(c)
	if !ok {
		return
	}
	var in memoryAdminCreateApplication
	if !memoryAdminBody(c, &in) {
		return
	}
	target := sha256.Sum256([]byte(hub + "\x00" + in.ApplicationID))
	c.Set("memory.admin.target", hex.EncodeToString(target[:]))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	// An existing receipt is reconciled against its persisted request, even if
	// the model catalog is unavailable or has since changed. It cannot dispatch
	// another bank creation. New applications must select enabled models.
	_, lookupErr := h.applicationsRegistry.Get(ctx, hub, in.ApplicationID, false)
	if errors.Is(lookupErr, memoryapp.ErrNotFound) {
		models, err := h.availableApplicationModels(ctx)
		if err != nil {
			respondError(c, 503, "memory_models_unavailable", "无法核对记忆服务模型，请检查服务连接后重试")
			return
		}
		has := func(models []MultiRAGModel, id string) bool {
			for _, model := range models {
				if model.FullID == id {
					return true
				}
			}
			return false
		}
		if !has(models.Chat, in.ChatModel) || !has(models.Embedding, in.EmbeddingModel) {
			respondError(c, 400, "memory_model_invalid", "请选择当前可用的提取模型与向量模型")
			return
		}
	} else if lookupErr != nil {
		adminApplicationError(c, lookupErr)
		return
	}
	// The authenticated Hub tenant owns this application. The request cannot
	// select another business tenant, delegate access, or bind an existing bank.
	out, err := h.applicationsRegistry.CreateWithModels(ctx, memoryapp.Owner{TenantID: hub, HubTenantID: hub, ChatModel: in.ChatModel, EmbeddingModel: in.EmbeddingModel, MaxApplications: 32}, memoryapp.CreateInput{OperationID: in.OperationID, ApplicationID: in.ApplicationID, Name: in.Name, Description: in.Description, PolicyVersion: "synthetic-v1"})
	if errors.Is(err, memoryapp.ErrUnknown) {
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": out})
		return
	}
	if err != nil {
		adminApplicationError(c, err)
		return
	}
	respondSuccess(c, out)
}

func (h *MemoryAdminHandler) applicationProvisioning(c *gin.Context) {
	c.Set("memory.admin.action", "memory.application.provisioning.read")
	hub, ok := h.applicationManager(c)
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		adminApplicationError(c, memoryapp.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	app, err := h.applicationsRegistry.Get(ctx, hub, c.Param("applicationId"), false)
	if err == nil && app.HubTenantID != hub {
		err = memoryapp.ErrNotFound
	}
	if err == nil {
		app, err = h.applicationsRegistry.Get(ctx, hub, app.ApplicationID, true)
	}
	if err != nil {
		adminApplicationError(c, err)
		return
	}
	respondSuccess(c, app)
}
