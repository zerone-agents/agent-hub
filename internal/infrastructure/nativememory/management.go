package nativememory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"control-panel/internal/memory"
)

var _ memory.NativeManagement = (*Adapter)(nil)
var _ memory.NativeDiagnostics = (*Adapter)(nil)

// BankInfo copies only safe, typed metadata from the configured private bank.
// Prompt bodies, avatar, owner/tenant IDs, credentials, and unknown fields never
// enter the public DTO. Capacity is a configured limit, not measured usage.
func (a *Adapter) BankInfo(ctx context.Context) (memory.NativeBankInfo, error) {
	raw, types, err := a.bankConfig(ctx)
	if err != nil {
		return memory.NativeBankInfo{}, err
	}
	var config struct {
		Name             string          `json:"name"`
		Description      string          `json:"description"`
		StorageType      string          `json:"storage_type"`
		ChatModel        string          `json:"llm_id"`
		EmbeddingModel   string          `json:"embd_id"`
		Permissions      string          `json:"permissions"`
		CapacityBytes    *int64          `json:"memory_size"`
		ForgettingPolicy string          `json:"forgetting_policy"`
		Temperature      *float64        `json:"temperature"`
		CreatedAt        json.RawMessage `json:"create_time"`
	}
	if json.Unmarshal(raw, &config) != nil || !boundedMetadata(config.Name, 128, false) || !boundedMetadata(config.Description, 4096, true) || !boundedMetadata(config.ChatModel, 128, false) || !boundedMetadata(config.EmbeddingModel, 128, false) || (config.StorageType != "table" && config.StorageType != "graph") || (config.ForgettingPolicy != "" && config.ForgettingPolicy != "FIFO" && config.ForgettingPolicy != "LRU") || (config.CapacityBytes != nil && *config.CapacityBytes <= 0) || (config.Temperature != nil && (*config.Temperature < 0 || *config.Temperature > 1)) {
		return memory.NativeBankInfo{}, memory.ErrIndexUnknown
	}
	created := ""
	if len(config.CreatedAt) > 0 && string(config.CreatedAt) != "null" {
		var millis int64
		if json.Unmarshal(config.CreatedAt, &millis) != nil || millis < 0 {
			return memory.NativeBankInfo{}, memory.ErrIndexUnknown
		}
		created = time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
	}
	return memory.NativeBankInfo{Name: config.Name, Description: config.Description, MemoryTypes: types, StorageType: config.StorageType, ChatModel: config.ChatModel, EmbeddingModel: config.EmbeddingModel, Permissions: config.Permissions, CapacityBytes: config.CapacityBytes, ForgettingPolicy: config.ForgettingPolicy, Temperature: config.Temperature, CreatedAt: created}, nil
}

func boundedMetadata(value string, max int, empty bool) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && (empty || strings.TrimSpace(value) != "") && !strings.ContainsAny(value, "\x00\r")
}

func (a *Adapter) managedRecord(ctx context.Context, scope nativeScope, id string) (nativeRecord, error) {
	row, err := a.readTyped(ctx, scope, id)
	if err != nil {
		return nativeRecord{}, err
	}
	if row.Type == "raw" {
		return row, nil
	}
	parent, err := a.readTyped(ctx, scope, row.SourceID)
	if err != nil {
		return nativeRecord{}, err
	}
	if parent.Type != "raw" || parent.SourceSessionID != row.SourceSessionID {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	return withParentStatus(row, parent), nil
}

func unchangedMessage(before, after nativeRecord) bool {
	return before.ID == after.ID && before.SourceID == after.SourceID && before.SourceSessionID == after.SourceSessionID && before.Type == after.Type && before.Content == after.Content && before.ValidAt == after.ValidAt && before.InvalidAt == after.InvalidAt && before.CreatedAt == after.CreatedAt && before.forgotten == after.forgotten
}

// SetMessageStatus changes one native switch. Parent-derived supply semantics
// are evaluated on read; this method never mutates children, forget_at or text.
// The expected switch is an observed-state precondition, not native atomic CAS.
func (a *Adapter) SetMessageStatus(ctx context.Context, token, id string, enabled, expectedEnabled bool) (memory.NativeMessage, error) {
	scope, err := a.scope(a.bank, token)
	if err != nil {
		return memory.NativeMessage{}, err
	}
	if parsed, ok := messageID(json.RawMessage(id), false); !ok || parsed != id {
		return memory.NativeMessage{}, memory.ErrInvalid
	}
	if err = a.checkBankMode(ctx, true); err != nil {
		return memory.NativeMessage{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	before, err := a.managedRecord(ctx, scope, id)
	if err != nil {
		return memory.NativeMessage{}, err
	}
	if !before.StatusChangeAllowed {
		return memory.NativeMessage{}, memory.ErrUnsupported
	}
	if before.NativeStatus != expectedEnabled {
		return memory.NativeMessage{}, memory.ErrConflict
	}
	if before.NativeStatus == enabled {
		return before.NativeMessage, nil
	}
	if enabled && before.ParentStatus != nil && !*before.ParentStatus {
		return memory.NativeMessage{}, memory.ErrConflict
	}
	// A lost/error response can have committed. Confirm through a fresh scoped
	// content read, never by repeating the mutation or trusting data:true alone.
	_, _ = a.request(ctx, http.MethodPut, "/messages/"+a.bank+":"+id, nil, map[string]bool{"status": enabled})
	after, err := a.managedRecord(ctx, scope, id)
	if err != nil || after.NativeStatus != enabled || !after.StatusChangeAllowed || !unchangedMessage(before, after) {
		return memory.NativeMessage{}, memory.ErrIndexUnknown
	}
	return after.NativeMessage, nil
}
