package handler

import (
	"control-panel/internal/memory"
	"github.com/gin-gonic/gin"
)

func (h *MemoryHandler) ingestConversation(c *gin.Context) {
	var in memory.ConversationInput
	if !memoryBody(c, &in) {
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.IngestConversation(c.Request.Context(), sc, in)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) conversationStatus(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "状态查询不接受参数")
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.ConversationStatus(c.Request.Context(), sc, c.Param("operationId"))
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) nativeMessages(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		memoryFailure(c, 400, "MEMORY_INVALID_REQUEST", "本版原生记忆列表不接受筛选参数")
		return
	}
	svc, sc := memoryRequest(c)
	out, err := svc.ListNativeMemories(c.Request.Context(), sc)
	memorySuccess(c, out, err)
}
func (h *MemoryHandler) nativeContext(c *gin.Context) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}
	if !memoryBody(c, &in) {
		return
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	svc, sc := memoryRequest(c)
	out, err := svc.NativeRecall(c.Request.Context(), sc, in.Query, in.Limit)
	memorySuccess(c, out, err)
}

func (h *MemoryAdminHandler) nativeMessages(c *gin.Context) {
	scope, ok := memoryAdminQuery(c, true)
	if !ok {
		return
	}
	svc, ok := h.selectService(c, scope)
	if !ok {
		return
	}
	c.Set("memory.admin.action", "memory.admin.native.read")
	out, err := svc.ListNativeMemories(c.Request.Context(), scope)
	memoryAdminResult(c, out, err)
}
func (h *MemoryAdminHandler) nativeContext(c *gin.Context) {
	var in struct {
		Scope memory.Scope `json:"scope"`
		Query string       `json:"query"`
		Limit int          `json:"limit,omitempty"`
	}
	if !memoryAdminBody(c, &in) || !memoryAdminSubjectScope(c, in.Scope) {
		return
	}
	svc, ok := h.selectService(c, in.Scope)
	if !ok {
		return
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	c.Set("memory.admin.action", "memory.admin.native.context")
	out, err := svc.NativeRecall(c.Request.Context(), in.Scope, in.Query, in.Limit)
	memoryAdminResult(c, out, err)
}
