package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// MultiRAGModel is a flattened entry from MultiRAG's /v1/llm/my_llms response.
// The nested `{factory: {llm: [models]}}` shape is projected onto this flat
// struct so the frontend can render a single list without second-level
// grouping.
type MultiRAGModel struct {
	Name    string `json:"name"`
	Factory string `json:"factory"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	FullID  string `json:"fullId"`
}

// ListMultiRAGModels flattens MultiRAG's my_llms response, filters by `type`,
// and returns the list as `{success: true, data: []MultiRAGModel}`.
//
// Query param `type` is required. Common values: "embedding", "ocr". Other
// MultiRAG types (e.g. "chat", "audio", "image") pass through but are not
// used by the current knowledge form.
//
// Returns:
//   - 400 when `type` query param is missing.
//   - 503 when MultiRAG is unconfigured (source is nil) OR the upstream call
//     fails OR the response cannot be parsed.
func (h *KnowledgeHandler) ListMultiRAGModels(c *gin.Context) {
	typeFilter := c.Query("type")
	if typeFilter == "" {
		respondError(c, http.StatusBadRequest, "type query param required (embedding|ocr|...)")
		return
	}

	if h.multiragMyLLMs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "MultiRAG 未配置",
		})
		return
	}

	raw, err := h.multiragMyLLMs.ListMyLLMs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// MultiRAG shape: { "<Factory>": { "llm": [ {type, name, status, ...} ] } }
	var perFactory map[string]struct {
		LLM []struct {
			Type   string `json:"type"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"llm"`
	}
	if err := json.Unmarshal(raw, &perFactory); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "解析 MultiRAG 响应失败: " + err.Error(),
		})
		return
	}

	out := make([]MultiRAGModel, 0, 32)
	for factory, group := range perFactory {
		for _, m := range group.LLM {
			if !strings.EqualFold(m.Type, typeFilter) {
				continue
			}
			out = append(out, MultiRAGModel{
				Name:    m.Name,
				Factory: factory,
				Type:    m.Type,
				Status:  m.Status,
				FullID:  m.Name + "@" + factory,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Factory != out[j].Factory {
			return out[i].Factory < out[j].Factory
		}
		return out[i].Name < out[j].Name
	})

	respondSuccess(c, out)
}
