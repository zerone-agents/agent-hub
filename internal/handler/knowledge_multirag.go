package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"control-panel/internal/domain/provider"

	"github.com/gin-gonic/gin"
)

// MultiRAGModel is a flattened entry from MultiRAG's /v1/llm/my_llms response.
// The nested `{factory: {llm: [models]}}` shape is projected onto this flat
// struct so the frontend can render a single list without second-level
// grouping.
type MultiRAGModel struct {
	Reference    string   `json:"reference,omitempty"`
	InstanceName string   `json:"instanceName,omitempty"`
	ModelTypes   []string `json:"modelTypes,omitempty"`
	Name         string   `json:"name"`
	Factory      string   `json:"factory"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	FullID       string   `json:"fullId"`
	IsTools      *bool    `json:"isTools,omitempty"`
}

// ListMultiRAGModels flattens MultiRAG's my_llms response, filters by `type`,
// and returns the list as `{success: true, data: []MultiRAGModel}`.
//
// Query param `type` is required. Knowledge forms use "embedding", "ocr"
// and "image2text". Other MultiRAG types pass through.
//
// Returns:
//   - 400 when `type` query param is missing.
//   - 503 when MultiRAG is unconfigured (source is nil) OR the upstream call
//     fails OR the response cannot be parsed.
func (h *KnowledgeHandler) ListMultiRAGModels(c *gin.Context) {
	typeFilter := strings.TrimSpace(c.Query("type"))
	if typeFilter == "" {
		respondError(c, http.StatusBadRequest, ErrCodeTypeQueryRequired, "type query param required (embedding|ocr|...)")
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
			"error":   "加载 MultiRAG 模型失败，请重试",
		})
		return
	}

	out, err := projectMultiRAGModels(raw, typeFilter)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "MultiRAG 模型列表响应无效，请重试"})
		return
	}
	respondSuccess(c, out)
}

// All consumers (including MCP rerank) use this projection. A model may support
// several capabilities while the legacy `type` carries only the first one.
func projectMultiRAGModels(raw json.RawMessage, typeFilter string) ([]MultiRAGModel, error) {
	var perFactory map[string]json.RawMessage
	if err := json.Unmarshal(raw, &perFactory); err != nil || perFactory == nil {
		return nil, fmt.Errorf("invalid MultiRAG inventory")
	}

	out := make([]MultiRAGModel, 0, 32)
	seen := map[string]bool{}
	for factory, rawGroup := range perFactory {
		var group struct {
			LLM []struct {
				ID           json.RawMessage `json:"id"`
				Reference    json.RawMessage `json:"reference"`
				InstanceName string          `json:"instance_name"`
				ModelTypes   json.RawMessage `json:"model_types"`
				Type         string          `json:"type"`
				Name         string          `json:"name"`
				Status       string          `json:"status"`
				IsTools      *bool           `json:"is_tools"`
			} `json:"llm"`
		}
		if strings.TrimSpace(factory) == "" || json.Unmarshal(rawGroup, &group) != nil || group.LLM == nil {
			return nil, fmt.Errorf("invalid MultiRAG inventory group")
		}
		for _, m := range group.LLM {
			if strings.TrimSpace(m.Type) == "" || strings.TrimSpace(m.Name) == "" {
				return nil, fmt.Errorf("invalid MultiRAG model")
			}
			fullID := m.Name + "@" + factory
			reference := ""
			if len(m.Reference) > 0 {
				if json.Unmarshal(m.Reference, &reference) != nil {
					return nil, fmt.Errorf("invalid MultiRAG reference")
				}
				var id json.Number
				if !provider.ValidModelReference(reference) || json.Unmarshal(m.ID, &id) != nil || id.String() != reference {
					return nil, fmt.Errorf("invalid MultiRAG reference")
				}
				fullID = reference
			}
			capabilities := []string{strings.ToLower(m.Type)}
			if len(m.ModelTypes) > 0 {
				if json.Unmarshal(m.ModelTypes, &capabilities) != nil {
					return nil, fmt.Errorf("invalid MultiRAG capabilities")
				}
				if len(capabilities) == 0 || len(capabilities) > 7 {
					return nil, fmt.Errorf("invalid MultiRAG capabilities")
				}
				checked := map[string]bool{}
				for _, capability := range capabilities {
					if !slices.Contains([]string{"chat", "embedding", "rerank", "image2text", "ocr", "tts", "speech2text"}, capability) || checked[capability] {
						return nil, fmt.Errorf("invalid MultiRAG capabilities")
					}
					checked[capability] = true
				}
			}
			if !slices.Contains(capabilities, strings.ToLower(typeFilter)) {
				continue
			}
			identity := fullID + "\x00" + strings.ToLower(typeFilter)
			if seen[identity] {
				return nil, fmt.Errorf("duplicate MultiRAG model identity")
			}
			seen[identity] = true
			projectedType := m.Type
			var modelTypes []string
			if len(m.ModelTypes) > 0 {
				projectedType = strings.ToLower(typeFilter)
				modelTypes = capabilities
			}
			out = append(out, MultiRAGModel{
				Reference:    reference,
				InstanceName: m.InstanceName,
				ModelTypes:   modelTypes,
				Name:         m.Name,
				Factory:      factory,
				Type:         projectedType,
				Status:       m.Status,
				FullID:       fullID,
				IsTools:      m.IsTools,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Factory != out[j].Factory {
			return out[i].Factory < out[j].Factory
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].FullID < out[j].FullID
	})

	return out, nil
}
