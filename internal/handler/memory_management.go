package handler

import (
	"control-panel/internal/memory"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *MemoryAdminHandler) forgetNativeMessage(c *gin.Context) {
	var in struct {
		Scope memory.Scope `json:"scope"`
	}
	if !memoryAdminBody(c, &in) || !memoryAdminSubjectScope(c, in.Scope) {
		return
	}
	svc, ok := h.selectService(c, in.Scope)
	if !ok {
		return
	}
	c.Set("memory.admin.action", "memory.admin.native.forget")
	id := c.Param("messageId")
	if parsed, err := strconv.ParseInt(id, 10, 64); err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == id {
		c.Set("memory.admin.native.messageId", id)
		sum := sha256.Sum256([]byte(c.GetString("memory.admin.target") + "\x00" + id))
		c.Set("memory.admin.target", hex.EncodeToString(sum[:]))
	}
	out, err := svc.ForgetNativeMessage(c.Request.Context(), in.Scope, id)
	memoryAdminResult(c, out, err)
}

func (h *MemoryAdminHandler) nativeConfiguration(c *gin.Context) {
	scope, ok := memoryAdminQuery(c, false)
	if !ok {
		return
	}
	svc, ok := h.selectService(c, scope)
	if !ok {
		return
	}
	c.Set("memory.admin.action", "memory.admin.native.config.read")
	out, err := svc.NativeConfiguration(c.Request.Context())
	memoryAdminResult(c, out, err)
}

func (h *MemoryAdminHandler) setNativeStatus(c *gin.Context) {
	var in struct {
		Scope           memory.Scope `json:"scope"`
		Enabled         *bool        `json:"enabled"`
		ExpectedEnabled *bool        `json:"expectedEnabled"`
	}
	if !memoryAdminBody(c, &in) || !memoryAdminSubjectScope(c, in.Scope) {
		return
	}
	if in.Enabled == nil || in.ExpectedEnabled == nil {
		respondError(c, 400, "memory_invalid_request", "需要明确的启用状态与当前状态")
		return
	}
	svc, ok := h.selectService(c, in.Scope)
	if !ok {
		return
	}
	c.Set("memory.admin.action", "memory.admin.native.status.update")
	messageID := c.Param("messageId")
	if id, err := strconv.ParseInt(messageID, 10, 64); err == nil && id > 0 && strconv.FormatInt(id, 10) == messageID {
		c.Set("memory.admin.native.messageId", messageID)
		sum := sha256.Sum256([]byte(c.GetString("memory.admin.target") + "\x00" + messageID))
		c.Set("memory.admin.target", hex.EncodeToString(sum[:]))
	}
	c.Set("memory.admin.native.enabled", *in.Enabled)
	c.Set("memory.admin.native.expectedEnabled", *in.ExpectedEnabled)
	out, err := svc.SetNativeMessageStatus(c.Request.Context(), in.Scope, messageID, *in.Enabled, *in.ExpectedEnabled)
	memoryAdminResult(c, out, err)
}

func (h *MemoryAdminHandler) nativeSearch(c *gin.Context) {
	var in struct {
		Scope               memory.Scope `json:"scope"`
		Query               string       `json:"query"`
		Limit               *int         `json:"limit,omitempty"`
		SimilarityThreshold *float64     `json:"similarityThreshold,omitempty"`
		KeywordWeight       *float64     `json:"keywordWeight,omitempty"`
	}
	if !memoryAdminBody(c, &in) || !memoryAdminSubjectScope(c, in.Scope) {
		return
	}
	svc, ok := h.selectService(c, in.Scope)
	if !ok {
		return
	}
	options := memory.NativeSearchOptions{Limit: 5, SimilarityThreshold: 0.2, KeywordWeight: 0.7}
	if in.Limit != nil {
		options.Limit = *in.Limit
	}
	if in.SimilarityThreshold != nil {
		options.SimilarityThreshold = *in.SimilarityThreshold
	}
	if in.KeywordWeight != nil {
		options.KeywordWeight = *in.KeywordWeight
	}
	c.Set("memory.admin.action", "memory.admin.native.search")
	out, err := svc.DiagnoseNativeSearch(c.Request.Context(), in.Scope, in.Query, options)
	memoryAdminResult(c, out, err)
}
