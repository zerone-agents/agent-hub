package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// DocumentIngestRequest is strict: no implicit document selection, no coerced
// booleans, and apply_kb is valid only for run=1. Run 0 resets, 1 starts and 2
// cancels. Delete clears old chunks; acknowledgement never proves completion.
type DocumentIngestRequest struct {
	DocIDs  []string `json:"doc_ids"`
	Run     string   `json:"run"`
	Delete  bool     `json:"delete"`
	ApplyKB bool     `json:"apply_kb"`
}

type DocumentIngestEngine interface {
	IngestDocuments(ctx context.Context, req DocumentIngestRequest) error
}

func (r *DocumentIngestRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		DocIDs  json.RawMessage `json:"doc_ids"`
		Run     json.RawMessage `json:"run"`
		Delete  json.RawMessage `json:"delete"`
		ApplyKB json.RawMessage `json:"apply_kb"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return NewBadRequestError("摄取请求字段无效")
	}
	if len(raw.DocIDs) == 0 || raw.DocIDs[0] != '[' || json.Unmarshal(raw.DocIDs, &r.DocIDs) != nil {
		return NewBadRequestError("doc_ids 必须是非空字符串数组")
	}
	switch string(raw.Run) {
	case "0", `"0"`:
		r.Run = "0"
	case "1", `"1"`:
		r.Run = "1"
	case "2", `"2"`:
		r.Run = "2"
	default:
		return NewBadRequestError("run 必须是 0、1 或 2")
	}
	for _, field := range []struct {
		name string
		raw  json.RawMessage
		out  *bool
	}{{"delete", raw.Delete, &r.Delete}, {"apply_kb", raw.ApplyKB, &r.ApplyKB}} {
		*field.out = false
		if len(field.raw) == 0 {
			continue
		}
		if string(field.raw) != "true" && string(field.raw) != "false" {
			return NewBadRequestError(field.name + " 必须是布尔值")
		}
		*field.out = string(field.raw) == "true"
	}
	return r.Validate()
}

func (r DocumentIngestRequest) Validate() error {
	if len(r.DocIDs) == 0 {
		return NewBadRequestError("doc_ids 不能为空")
	}
	for _, id := range r.DocIDs {
		if strings.TrimSpace(id) == "" {
			return NewBadRequestError("doc_ids 必须是非空字符串数组")
		}
	}
	if r.Run != "0" && r.Run != "1" && r.Run != "2" {
		return NewBadRequestError("run 必须是 0、1 或 2")
	}
	if r.ApplyKB && r.Run != "1" {
		return NewBadRequestError("apply_kb 仅可在 run=1 时使用")
	}
	return nil
}
