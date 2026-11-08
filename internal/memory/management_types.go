package memory

import "context"

// NativeBankInfo is an allowlisted view of provider configuration. Capacity is
// a configured upper bound, not measured usage; model values can be references.
type NativeBankInfo struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	MemoryTypes      []string `json:"memoryTypes"`
	StorageType      string   `json:"storageType,omitempty"`
	ChatModel        string   `json:"chatModel,omitempty"`
	EmbeddingModel   string   `json:"embeddingModel,omitempty"`
	Permissions      string   `json:"permissions,omitempty"`
	CapacityBytes    *int64   `json:"capacityBytes,omitempty"`
	ForgettingPolicy string   `json:"forgettingPolicy,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	CreatedAt        string   `json:"createdAt,omitempty"`
}

// NativeManagement deliberately omits deletion and global bank mutation.
type NativeManagement interface {
	BankInfo(context.Context) (NativeBankInfo, error)
	SetMessageStatus(context.Context, string, string, bool, bool) (NativeMessage, error)
}

type NativeSearchOptions struct {
	Limit               int     `json:"limit"`
	SimilarityThreshold float64 `json:"similarityThreshold"`
	KeywordWeight       float64 `json:"keywordWeight"`
}

type NativeDiagnostics interface {
	SearchDiagnostic(context.Context, string, string, NativeSearchOptions) ([]NativeMessage, error)
}

type NativeSearchResult struct {
	Status   string              `json:"status"`
	Messages []NativeMessage     `json:"messages"`
	Options  NativeSearchOptions `json:"options"`
}
