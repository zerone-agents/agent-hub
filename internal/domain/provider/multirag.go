package provider

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrMultiRAGSyncNotImplemented is returned by the default BaseProvider
// implementation of SyncToMultiRAG. Each concrete provider overrides it.
var ErrMultiRAGSyncNotImplemented = errors.New("SyncToMultiRAG not implemented for this provider")

// ErrMultiRAGConfigMissing is returned when the MultiRAG client hasn't been
// configured (missing base URL or API key).
var ErrMultiRAGConfigMissing = errors.New("MultiRAG client not configured (missing base URL or API key)")

// MultiRAGClient abstracts the HTTP transport to a MultiRAG instance.
// Implementations live in the infrastructure layer.
type MultiRAGClient interface {
	// AddLLM corresponds to MultiRAG POST /v1/llm/add_llm (all providers).
	AddLLM(ctx context.Context, payload AddLLMRequest) (*MultiRAGResponse, error)
}

// MultiRAGMyLLMsSource is the read-only source of MultiRAG's currently
// configured models. It abstracts the HTTP client so handlers can be unit
// tested without touching the network. Defined here (rather than in the
// handler package) to avoid an import cycle: the infrastructure client
// implements it, and the handler consumes it.
type MultiRAGMyLLMsSource interface {
	// ListMyLLMs returns the raw JSON `data` field of MultiRAG's
	// /v1/llm/my_llms?include_details=true response (without the outer
	// {retcode, retmsg, data} envelope). The shape is:
	//   { "<Factory>": { "llm": [ {type, name, status, ...} ] } }
	ListMyLLMs(ctx context.Context) (json.RawMessage, error)
}

// AddLLMRequest is the payload for provider sync. MultiRAG's
// /v1/llm/add_llm accepts these fields plus provider-specific extras
// (e.g. bedrock_ak, spark_api_password). Extras are carried via the
// Extras map and merged into the outgoing JSON at marshal time.
type AddLLMRequest struct {
	LLMFactory string          `json:"llm_factory"`
	LLMName    string          `json:"llm_name"`
	MdlType    string          `json:"mdl_type"`
	APIKey     json.RawMessage `json:"api_key,omitempty"` // string OR nested object
	APIBase    string          `json:"api_base,omitempty"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
	Verify     bool            `json:"verify"`
	// Extras carries C 类 provider-specific fields (tencent_cloud_sid,
	// bedrock_ak, endpoint_id, etc.). These are merged into the top-level
	// JSON object at marshal time, NOT nested under api_key.
	Extras map[string]any `json:"-"`
}

// MarshalJSON merges AddLLMRequest.Extras into the top-level JSON object.
func (r AddLLMRequest) MarshalJSON() ([]byte, error) {
	type alias AddLLMRequest // prevent recursion
	b, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	if len(r.Extras) == 0 {
		return b, nil
	}
	extras, err := json.Marshal(r.Extras)
	if err != nil {
		return nil, err
	}
	// Merge extras into the marshaled object.
	out := make(map[string]json.RawMessage)
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(extras, &out); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// MultiRAGResponse is the normalized response from any MultiRAG endpoint.
// MultiRAG returns the result in several inconsistent shapes; the client
// implementation is responsible for normalizing into this struct.
type MultiRAGResponse struct {
	// HTTPStatusCode is the raw HTTP response code.
	HTTPStatusCode int
	// Success is true when MultiRAG indicates the operation succeeded.
	// For envelope responses: retcode == 0. For bare responses: true.
	Success bool
	// Message is a human-readable status or error message from MultiRAG.
	Message string
	// Raw is the raw response body for debugging.
	Raw json.RawMessage
}

// SyncOptions controls SyncToMultiRAG behavior.
type SyncOptions struct {
	// VerifyOnly corresponds to MultiRAG's verify=true mode: validate
	// credentials/configuration without persisting. False means persist.
	VerifyOnly bool
}

// SyncResult reports the outcome of one provider's sync.
type SyncResult struct {
	// FactoryName is the MultiRAG factory the provider synced to.
	FactoryName string `json:"factoryName"`
	// Endpoint is "add_llm".
	Endpoint string `json:"endpoint"`
	// CallCount is the number of MultiRAG calls made (1 for C 类,
	// N for per-model sync where N = len(DefaultModels)).
	CallCount int `json:"callCount"`
	// Success is true iff every call succeeded.
	Success bool `json:"success"`
	// PerCall carries one entry per MultiRAG call.
	PerCall []SyncCallResult `json:"perCall"`
}

// SyncCallResult reports the outcome of a single MultiRAG call.
type SyncCallResult struct {
	// ModelName is the model involved in this call. Empty when the call
	// failed before a model could be associated with it.
	ModelName string `json:"modelName"`
	// HTTPStatus from MultiRAG.
	HTTPStatus int `json:"httpStatus"`
	// OK is true iff this individual call succeeded.
	OK bool `json:"ok"`
	// Error carries a human-readable error when OK is false.
	Error string `json:"error"`
	// Raw is the raw response body for debugging.
	Raw json.RawMessage `json:"raw"`
}

// MapModelTypeToMultiRAG converts our internal model_type values
// (llm | ocr | embedding | vlm) to MultiRAG's mdl_type vocabulary
// (chat | ocr | embedding | image2text). Unknown values map to "" (caller
// should validate before calling AddLLM).
func MapModelTypeToMultiRAG(t string) string {
	switch t {
	case string(TypeLLM):
		return "chat"
	case string(TypeOCR):
		return "ocr"
	case string(TypeEmbedding):
		return "embedding"
	case string(TypeVLM):
		return "image2text"
	}
	return ""
}
