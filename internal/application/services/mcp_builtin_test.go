package services

import (
	"testing"
	"time"

	"control-panel/internal/domain/mcp"

	"github.com/stretchr/testify/assert"
)

func TestApplyBuiltinMetadataSetsFixedToolsAndSuccessfulStatus(t *testing.T) {
	probedAt := time.Now()
	existing := &mcp.McpServer{
		Name:         "knowledge",
		URL:          "https://custom.example.com/api/v1/knowledge/mcp",
		Headers:      "encrypted-custom-headers",
		IsBuiltin:    true,
		ToolsJSON:    "",
		ProbeStatus:  "failed",
		LastProbedAt: &probedAt,
	}
	definition := &mcp.McpServer{
		Name:        "knowledge",
		ToolsJSON:   mustMarshalMcpTools(builtinKnowledgeTools),
		ProbeStatus: "success",
	}

	changed := applyBuiltinMetadata(existing, definition)

	assert.True(t, changed)
	assert.Equal(t, "success", existing.ProbeStatus)
	assert.JSONEq(t, `[{"name":"knowledge_search","description":"检索 Agent 已绑定的知识库，为文档问答提供相关文本片段"}]`, existing.ToolsJSON)
	assert.Nil(t, existing.LastProbedAt)
	assert.Equal(t, "https://custom.example.com/api/v1/knowledge/mcp", existing.URL)
	assert.Equal(t, "encrypted-custom-headers", existing.Headers)
}

func TestApplyBuiltinMetadataIsIdempotent(t *testing.T) {
	toolsJSON := mustMarshalMcpTools(builtinKnowledgeTools)
	existing := &mcp.McpServer{
		Name:        "knowledge",
		IsBuiltin:   true,
		ToolsJSON:   toolsJSON,
		ProbeStatus: "success",
	}
	definition := &mcp.McpServer{
		Name:        "knowledge",
		ToolsJSON:   toolsJSON,
		ProbeStatus: "success",
	}

	assert.False(t, applyBuiltinMetadata(existing, definition))
}
