package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/provider"

	"github.com/gin-gonic/gin"
)

// ProviderHandler handles HTTP requests for vendor preset CRUD and probing.
type ProviderHandler struct {
	service        *services.ProviderService
	multiragClient provider.MultiRAGClient
}

// NewProviderHandler wires a ProviderHandler. multiragClient is the
// provider.MultiRAGClient used by SyncToMultiRAG; pass nil when MultiRAG
// is not configured (the endpoint will then return 503).
func NewProviderHandler(service *services.ProviderService, multiragClient provider.MultiRAGClient) *ProviderHandler {
	return &ProviderHandler{service: service, multiragClient: multiragClient}
}

// ── Public endpoints (for Electron app, JWTAuth required) ───────

func (h *ProviderHandler) List(c *gin.Context) {
	typeFilter, ok := parseTypeQuery(c)
	if !ok {
		return
	}
	providers, err := h.service.ListAll(typeFilter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]*services.ProviderDTO, 0, len(providers))
	for _, p := range providers {
		dto, err := h.service.ToDTO(p)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, dto)
	}
	respondSuccess(c, items)
}

func (h *ProviderHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	p, err := h.service.GetByID(id)
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	dto, err := h.service.ToDTO(p)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, dto)
}

// ── Admin CRUD ──────────────────────────────────────────────────

func (h *ProviderHandler) ListAdmin(c *gin.Context) {
	typeFilter, ok := parseTypeQuery(c)
	if !ok {
		return
	}
	providers, err := h.service.ListAll(typeFilter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]*services.ProviderDTO, 0, len(providers))
	for _, p := range providers {
		dto, err := h.service.ToDTO(p)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, dto)
	}
	respondSuccess(c, items)
}

// parseTypeQuery validates the optional ?type= query param shared by the
// list endpoints. Returns ok=false after writing the 400 response.
func parseTypeQuery(c *gin.Context) (typeFilter string, ok bool) {
	typeFilter = c.Query("type")
	if typeFilter != "" && typeFilter != string(provider.TypeLLM) && typeFilter != string(provider.TypeOCR) && typeFilter != string(provider.TypeEmbedding) && typeFilter != string(provider.TypeVLM) {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("type 不支持: %s（可选: llm, ocr, embedding, vlm）", typeFilter))
		return "", false
	}
	return typeFilter, true
}

type createProviderRequest struct {
	Key           string                        `json:"key" binding:"required"`
	Name          string                        `json:"name" binding:"required"`
	Description   string                        `json:"description"`
	DescriptionEn string                        `json:"descriptionEn"`
	Protocol      string                        `json:"protocol" binding:"required"`
	AuthStyle     string                        `json:"authStyle" binding:"required"`
	BaseURL       string                        `json:"baseUrl"`
	DefaultModels []provider.CatalogModel       `json:"defaultModels"`
	Fields        []provider.PresetField        `json:"fields"`
	IconKey       string                        `json:"iconKey"`
	Builtin       bool                          `json:"builtin"`
	Attributes    map[string]provider.AttrValue `json:"attributes"`
	LockedAPIKey  string                        `json:"lockedApiKey"`
}

func (h *ProviderHandler) Create(c *gin.Context) {
	var req createProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	dto, err := h.service.Create(&services.CreateProviderInput{
		Key:           req.Key,
		Name:          req.Name,
		Description:   req.Description,
		DescriptionEn: req.DescriptionEn,
		Protocol:      req.Protocol,
		AuthStyle:     req.AuthStyle,
		BaseURL:       req.BaseURL,
		DefaultModels: req.DefaultModels,
		Fields:        req.Fields,
		IconKey:       req.IconKey,
		Builtin:       req.Builtin,
		Attributes:    req.Attributes,
		LockedAPIKey:  req.LockedAPIKey,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondCreated(c, dto)
}

type updateProviderRequest struct {
	Name          *string                       `json:"name"`
	Description   *string                       `json:"description"`
	DescriptionEn *string                       `json:"descriptionEn"`
	Protocol      *string                       `json:"protocol"`
	AuthStyle     *string                       `json:"authStyle"`
	BaseURL       *string                       `json:"baseUrl"`
	DefaultModels *[]provider.CatalogModel      `json:"defaultModels"`
	Fields        *[]provider.PresetField       `json:"fields"`
	IconKey       *string                       `json:"iconKey"`
	Builtin       *bool                         `json:"builtin"`
	Attributes    map[string]provider.AttrValue `json:"attributes"`
	LockedAPIKey  *string                       `json:"lockedApiKey"`
}

func (h *ProviderHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req updateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	dto, err := h.service.Update(id, &services.UpdateProviderInput{
		Name:          req.Name,
		Description:   req.Description,
		DescriptionEn: req.DescriptionEn,
		Protocol:      req.Protocol,
		AuthStyle:     req.AuthStyle,
		BaseURL:       req.BaseURL,
		DefaultModels: req.DefaultModels,
		Fields:        req.Fields,
		IconKey:       req.IconKey,
		Builtin:       req.Builtin,
		Attributes:    req.Attributes,
		LockedAPIKey:  req.LockedAPIKey,
	})
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, dto)
}

func (h *ProviderHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.service.Delete(id); err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "Provider 已删除")
}

// ── Probe ───────────────────────────────────────────────────────

func (h *ProviderHandler) Probe(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	type probeOverrideRequest struct {
		APIKey string `json:"apiKey"`
	}
	var overrideReq probeOverrideRequest
	// Body is optional; ignore bind errors when no body is sent.
	_ = c.ShouldBindJSON(&overrideReq)

	result, err := h.service.ProbeWithOverride(id, overrideReq.APIKey)
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, result)
}

type probeConfigRequest struct {
	BaseURL   string                  `json:"baseUrl" binding:"required"`
	APIKey    string                  `json:"apiKey"`
	Protocol  string                  `json:"protocol"`
	AuthStyle string                  `json:"authStyle"`
	Models    []provider.CatalogModel `json:"models"`
}

func (h *ProviderHandler) ProbeConfig(c *gin.Context) {
	var req probeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = string(provider.ProtocolAnthropic)
	}
	authStyle := req.AuthStyle
	if authStyle == "" {
		authStyle = string(provider.AuthStyleAPIKey)
	}

	result := h.service.ProbeConfig(req.BaseURL, req.APIKey, protocol, authStyle, req.Models)
	respondSuccess(c, result)
}

// ListRuntimeConfig serves every provider's runtime configuration (including
// plaintext API keys) to any authenticated user, so Zerone Desktop can make
// local model calls. The response is marked no-store and the audit log never
// contains plaintext keys.
func (h *ProviderHandler) ListRuntimeConfig(c *gin.Context) {
	configs, err := h.service.ListRuntimeConfigs()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Header("Cache-Control", "no-store")
	log.Printf("[AUDIT] provider runtime-config served | user_id=%s user_name=%s providers=%d remote_ip=%s time=%s",
		c.GetString("user_id"), c.GetString("user_name"), len(configs), c.ClientIP(),
		time.Now().UTC().Format(time.RFC3339))
	respondSuccess(c, configs)
}

// RevealAPIKey returns a provider's plaintext API key to an authorized admin.
func (h *ProviderHandler) RevealAPIKey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	apiKey, err := h.service.RevealAPIKey(id)
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("[AUDIT] provider API key revealed | user_id=%s user_name=%s provider_id=%d remote_ip=%s method=%s path=%s result=success time=%s",
		c.GetString("user_id"), c.GetString("user_name"), id, c.ClientIP(), c.Request.Method,
		c.Request.URL.Path, time.Now().UTC().Format(time.RFC3339))
	respondSuccess(c, gin.H{"apiKey": apiKey})
}

// AttrRules returns the provider-specific attribute rules used to
// dynamically render the provider form. When ?protocol= is given,
// only that protocol's rules are returned; otherwise the full map.
func (h *ProviderHandler) AttrRules(c *gin.Context) {
	protocol := c.Query("protocol")
	if protocol != "" {
		rules := provider.ProviderAttrRules[protocol]
		if rules == nil {
			rules = []provider.AttrRule{}
		}
		respondSuccess(c, map[string][]provider.AttrRule{protocol: rules})
		return
	}
	respondSuccess(c, provider.ProviderAttrRules)
}

// ── Per-model CRUD ──────────────────────────────────────────────

type createModelRequest struct {
	ModelID       string   `json:"modelId" binding:"required"`
	DisplayName   string   `json:"displayName"`
	ModelType     string   `json:"modelType" binding:"required"`
	ContextWindow int      `json:"contextWindow"`
	Efforts       []string `json:"efforts"`
}

// AddModel attaches a new model row to an existing provider and returns
// the updated provider DTO so the frontend gets the new state in one
// round-trip.
func (h *ProviderHandler) AddModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req createModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	dto, err := h.service.AddModel(id, &services.AddModelInput{
		ModelID:       req.ModelID,
		DisplayName:   req.DisplayName,
		ModelType:     req.ModelType,
		ContextWindow: req.ContextWindow,
		Efforts:       req.Efforts,
	})
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondCreated(c, dto)
}

type updateModelRequest struct {
	DisplayName   *string   `json:"displayName"`
	ModelType     *string   `json:"modelType"`
	ContextWindow *int      `json:"contextWindow"`
	Status        *string   `json:"status"`
	Efforts       *[]string `json:"efforts"`
}

// UpdateModel applies a partial update to a single model row identified
// by (providerID, selectionID) and returns the updated provider DTO.
func (h *ProviderHandler) UpdateModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	selectionID := c.Param("selectionId")
	var req updateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	dto, err := h.service.UpdateModel(id, selectionID, &services.UpdateModelInput{
		DisplayName:   req.DisplayName,
		ModelType:     req.ModelType,
		ContextWindow: req.ContextWindow,
		Status:        req.Status,
		Efforts:       req.Efforts,
	})
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, dto)
}

// DeleteModel removes a single model row identified by (providerID,
// selectionID).
func (h *ProviderHandler) DeleteModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	selectionID := c.Param("selectionId")
	if err := h.service.DeleteModel(id, selectionID); err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "Model 已删除")
}

// ── MultiRAG sync ───────────────────────────────────────────────

type syncToMultiRAGRequest struct {
	VerifyOnly bool     `json:"verifyOnly"`
	ModelIds   []string `json:"modelIds"`
}

// SyncToMultiRAG pushes a provider's configuration to the configured
// MultiRAG instance. The MultiRAG client is constructed at server startup
// from cfg.Knowledge.MultiragBaseURL / MultiragAPIKey and injected via
// NewProviderHandler; when absent, the endpoint returns 503.
//
// Body (optional): {"verifyOnly": bool, "modelIds": ["a","b"]} — modelIds
// restricts which models to sync; when empty, all models are synced.
func (h *ProviderHandler) SyncToMultiRAG(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req syncToMultiRAGRequest
	// Body is optional; ignore bind errors when no body is sent.
	_ = c.ShouldBindJSON(&req)

	result, err := h.service.SyncProviderToMultiRAG(c.Request.Context(), id, h.multiragClient, req.VerifyOnly, req.ModelIds)
	if err != nil {
		if err == provider.ErrProviderNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		if err == provider.ErrMultiRAGConfigMissing {
			respondError(c, http.StatusServiceUnavailable, "MultiRAG 未配置")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, result)
}
