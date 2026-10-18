package knowledge

import "context"

// EmbeddingEngine is optional so read-only/chat engines keep their contract.
type EmbeddingEngine interface {
	CheckEmbedding(context.Context, string, EmbeddingCheckRequest) (*EmbeddingCheckResult, error)
}

type EmbeddingCheckRequest struct {
	EmbeddingID string `json:"embd_id"`
	CheckNum    *int   `json:"check_num,omitempty"`
}

type EmbeddingCheckResult struct {
	Code       int      `json:"code"`
	Compatible bool     `json:"compatible"`
	Summary    Object   `json:"summary"`
	Results    []Object `json:"results"`
}

type EmbeddingIncompatibleError struct{ Result *EmbeddingCheckResult }

func (e *EmbeddingIncompatibleError) Error() string {
	return "嵌入模型与已有切片不兼容；配置未保存，请保留原模型或重新解析文档"
}

type PreviewEngine interface {
	PreviewDocument(context.Context, string, string) (*StreamResult, error)
}
