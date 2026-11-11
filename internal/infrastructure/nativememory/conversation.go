package nativememory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"control-panel/internal/memory"
)

var _ memory.NativeConversations = (*Adapter)(nil)

func validMemoryTypes(types []string) bool {
	seen := map[string]bool{}
	for _, kind := range types {
		if seen[kind] || (kind != "raw" && kind != "semantic" && kind != "episodic" && kind != "procedural") {
			return false
		}
		seen[kind] = true
	}
	return seen["raw"]
}

// Each ingestion operation gets a unique native session. This preserves repeat
// utterances in different turns without altering the actual dialogue text.
func conversationSession(scope nativeScope, operation string) string {
	sum := sha256.Sum256([]byte(operation))
	return scope.session + "-" + hex.EncodeToString(sum[:])
}

func withinSession(scope nativeScope, session string) bool {
	if session == scope.session { // Existing quote-ledger records.
		return true
	}
	if !strings.HasPrefix(session, scope.session+"-") {
		return false
	}
	suffix := strings.TrimPrefix(session, scope.session+"-")
	decoded, err := hex.DecodeString(suffix)
	return err == nil && len(decoded) == 32 && strings.ToLower(suffix) == suffix
}

type nativeRecord struct {
	memory.NativeMessage
	forgotten string
}

func optionalString(value map[string]json.RawMessage, field string) (string, bool) {
	raw, present := value[field]
	if !present || string(raw) == "null" {
		return "", true
	}
	var text string
	err := json.Unmarshal(raw, &text)
	return text, err == nil
}

func (a *Adapter) typedMessage(raw json.RawMessage, scope nativeScope, requireContent bool) (nativeRecord, error) {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	for field, expected := range map[string]string{"memory_id": a.bank, "user_id": scope.user, "agent_id": scope.agent} {
		var actual string
		if json.Unmarshal(value[field], &actual) != nil || actual != expected {
			return nativeRecord{}, memory.ErrIndexUnknown
		}
	}
	var result nativeRecord
	if json.Unmarshal(value["session_id"], &result.SourceSessionID) != nil || !withinSession(scope, result.SourceSessionID) || json.Unmarshal(value["message_type"], &result.Type) != nil {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	var ok, sourceOK, statusOK bool
	result.ID, ok = messageID(value["message_id"], false)
	result.SourceID, sourceOK = messageID(value["source_id"], true)
	result.Status, statusOK = nativeBool(value["status"])
	result.NativeStatus = result.Status
	if !ok || !sourceOK || !statusOK || (result.Type == "raw") != (result.SourceID == "0") || (result.Type != "raw" && result.Type != "semantic" && result.Type != "episodic" && result.Type != "procedural") {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	if alias, present := value["id"]; present {
		var actual string
		if json.Unmarshal(alias, &actual) != nil || actual != a.bank+"_"+result.ID {
			return nativeRecord{}, memory.ErrIndexUnknown
		}
	}
	if _, present := value["forget_at"]; !present {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	if result.forgotten, ok = optionalString(value, "forget_at"); !ok || result.forgotten == "-" {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	if result.ValidAt, ok = optionalString(value, "valid_at"); !ok {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	if result.InvalidAt, ok = optionalString(value, "invalid_at"); !ok {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	if result.CreatedAt, ok = optionalString(value, "created_at"); !ok {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	if content, present := value["content"]; present {
		if json.Unmarshal(content, &result.Content) != nil || !utf8.ValidString(result.Content) {
			return nativeRecord{}, memory.ErrIndexUnknown
		}
	} else if requireContent {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	// Forgotten records are retained for diagnostics but never marked usable.
	if result.forgotten != "" {
		result.Status = false
	}
	result.StatusChangeAllowed = result.forgotten == ""
	result.ForgottenAt = result.forgotten
	return result, nil
}

func withParentStatus(child, parent nativeRecord) nativeRecord {
	active := parent.Status
	child.ParentStatus = &active
	child.ParentForgottenAt = parent.ForgottenAt
	child.Status = child.Status && active
	child.StatusChangeAllowed = child.StatusChangeAllowed && parent.forgotten == ""
	return child
}

func (a *Adapter) readTyped(ctx context.Context, scope nativeScope, id string) (nativeRecord, error) {
	raw, err := a.request(ctx, http.MethodGet, "/messages/"+a.bank+":"+id+"/content", nil, nil)
	if err != nil {
		return nativeRecord{}, err
	}
	row, err := a.typedMessage(raw, scope, true)
	if err != nil || row.ID != id {
		return nativeRecord{}, memory.ErrIndexUnknown
	}
	return row, nil
}

// The task attached by the native list endpoint is the extraction authority.
// Seeing only RAW is never sufficient to declare extraction completed.
func extractionTask(raw json.RawMessage, hasDerived bool) (string, float64) {
	var task struct {
		Progress *float64 `json:"progress"`
	}
	if json.Unmarshal(raw, &task) == nil && task.Progress != nil {
		p := *task.Progress
		switch {
		case p < 0:
			return "failed", p
		case p >= 1:
			return "completed", p
		case hasDerived:
			return "derivedAvailable", p
		case p > 0:
			return "extracting", p
		default:
			return "received", p
		}
	}
	if hasDerived {
		return "derivedAvailable", 0
	}
	return "unknown", 0
}

// scanTyped reads raw parents and their typed children. Native list has a fixed
// 512-child bound, so a saturated child page is rejected instead of presenting
// a truncated set as a complete result.
func (a *Adapter) scanTyped(ctx context.Context, scope nativeScope, exactSession string) ([]nativeRecord, error) {
	result := []nativeRecord{}
	seen := map[string]bool{}
	globalTotal, rawCount, globalMode, pageMode := -1, 0, true, true
	partial := false
	for page := 1; page <= a.pages; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(a.pageSize)}, "agent_id": {scope.agent}}
		if exactSession != "" {
			query.Set("keywords", exactSession)
		}
		raw, err := a.request(ctx, http.MethodGet, "/memories/"+a.bank, query, nil)
		if err != nil {
			return nil, err
		}
		var listing struct {
			Messages *struct {
				List  []json.RawMessage `json:"message_list"`
				Total *int              `json:"total_count"`
			} `json:"messages"`
		}
		if json.Unmarshal(raw, &listing) != nil || listing.Messages == nil || listing.Messages.List == nil || listing.Messages.Total == nil {
			return nil, memory.ErrIndexUnknown
		}
		rows, total := listing.Messages.List, *listing.Messages.Total
		if len(rows) > a.pageSize || total < len(rows) || total > a.pageSize*(a.pages-1) || partial && len(rows) != 0 {
			return nil, memory.ErrIndexUnknown
		}
		partial = len(rows) > 0 && len(rows) < a.pageSize
		if globalTotal < 0 {
			globalTotal = total
		}
		globalMode, pageMode = globalMode && total == globalTotal, pageMode && total == len(rows)
		if !globalMode && !pageMode {
			return nil, memory.ErrIndexUnknown
		}
		if len(rows) == 0 {
			if !((globalMode && rawCount == globalTotal) || pageMode) {
				return nil, memory.ErrIndexUnknown
			}
			return result, nil
		}
		childrenOnPage := 0
		for _, rawRow := range rows {
			listed, err := a.typedMessage(rawRow, scope, false)
			if err != nil || listed.Type != "raw" || seen[listed.ID] || exactSession != "" && listed.SourceSessionID != exactSession {
				return nil, memory.ErrIndexUnknown
			}
			full, err := a.readTyped(ctx, scope, listed.ID)
			if err != nil || full.Status != listed.Status || full.forgotten != listed.forgotten || full.SourceSessionID != listed.SourceSessionID || full.Type != "raw" {
				return nil, memory.ErrIndexUnknown
			}
			var attached struct {
				Extract []json.RawMessage `json:"extract"`
				Task    json.RawMessage   `json:"task"`
			}
			if json.Unmarshal(rawRow, &attached) != nil || attached.Extract == nil {
				return nil, memory.ErrIndexUnknown
			}
			childrenOnPage += len(attached.Extract)
			full.ExtractionStatus, full.ExtractionProgress = extractionTask(attached.Task, len(attached.Extract) > 0)
			if full.ExtractionStatus == "failed" {
				full.ExtractionError = "NATIVE_EXTRACTION_FAILED"
			}
			seen[full.ID], rawCount = true, rawCount+1
			result = append(result, full)
			for _, rawChild := range attached.Extract {
				child, err := a.typedMessage(rawChild, scope, true)
				if err != nil || child.Type == "raw" || child.SourceID != full.ID || child.SourceSessionID != full.SourceSessionID || seen[child.ID] {
					return nil, memory.ErrIndexUnknown
				}
				verified, err := a.readTyped(ctx, scope, child.ID)
				if err != nil || verified.NativeMessage != child.NativeMessage || verified.forgotten != child.forgotten {
					return nil, memory.ErrIndexUnknown
				}
				// A disabled/forgotten parent cannot leave derived content usable.
				verified = withParentStatus(verified, full)
				seen[child.ID] = true
				result = append(result, verified)
			}
		}
		if childrenOnPage >= 512 {
			return nil, memory.ErrIndexUnknown
		}
	}
	return nil, memory.ErrIndexUnknown
}

func publicMessages(rows []nativeRecord) []memory.NativeMessage {
	result := make([]memory.NativeMessage, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.NativeMessage)
	}
	return result
}

func conversationResult(session, content string, rows []nativeRecord) (memory.NativeIngestResult, error) {
	result := memory.NativeIngestResult{Status: "unknown", SourceSessionID: session, Messages: []memory.NativeMessage{}}
	parents := 0
	for _, row := range rows {
		if row.Type != "raw" {
			continue
		}
		parents++
		if row.Content != content || !row.Status {
			return result, memory.ErrIndexUnknown
		}
		result.SourceID, result.Status = row.ID, row.ExtractionStatus
	}
	if parents != 1 {
		return result, memory.ErrIndexUnknown
	}
	result.Messages = publicMessages(rows)
	return result, nil
}

func validTurn(turn memory.NativeTurn) bool {
	return strings.TrimSpace(turn.OperationID) != "" && len(turn.OperationID) <= 256 && strings.TrimSpace(turn.UserInput) != "" && utf8.ValidString(turn.UserInput) && utf8.ValidString(turn.AgentResponse) && utf8.RuneCountInString(turn.UserInput) <= 6000 && utf8.RuneCountInString(turn.AgentResponse) <= 12000
}

// FindConversation performs read-only recovery even when no source ID was
// received. Absence never authorizes a replay of a previously dispatched POST.
func (a *Adapter) FindConversation(ctx context.Context, token string, turn memory.NativeTurn) (memory.NativeIngestResult, error) {
	result := memory.NativeIngestResult{Status: "unknown", Messages: []memory.NativeMessage{}}
	scope, err := a.scope(a.bank, token)
	if err != nil || !validTurn(turn) {
		return result, memory.ErrInvalid
	}
	result.SourceSessionID = conversationSession(scope, turn.OperationID)
	if err = a.checkBankMode(ctx, true); err != nil {
		return result, err
	}
	rows, err := a.scanTyped(ctx, scope, result.SourceSessionID)
	if err != nil || len(rows) == 0 {
		return result, err
	}
	return conversationResult(result.SourceSessionID, "User Input: "+turn.UserInput+"\nAgent Response: "+turn.AgentResponse, rows)
}

func (a *Adapter) ListMemories(ctx context.Context, token string) ([]memory.NativeMessage, error) {
	scope, err := a.scope(a.bank, token)
	if err != nil {
		return nil, err
	}
	if err = a.checkBankMode(ctx, true); err != nil {
		return nil, err
	}
	rows, err := a.scanTyped(ctx, scope, "")
	return publicMessages(rows), err
}

func (a *Adapter) AddConversation(ctx context.Context, token string, turn memory.NativeTurn) (memory.NativeIngestResult, error) {
	result := memory.NativeIngestResult{Status: "unknown", Messages: []memory.NativeMessage{}}
	scope, err := a.scope(a.bank, token)
	if err != nil || !validTurn(turn) {
		return result, memory.ErrInvalid
	}
	result.SourceSessionID = conversationSession(scope, turn.OperationID)
	if err = a.checkBankMode(ctx, true); err != nil {
		return result, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	content := "User Input: " + turn.UserInput + "\nAgent Response: " + turn.AgentResponse
	read := func() ([]nativeRecord, error) { return a.scanTyped(ctx, scope, result.SourceSessionID) }
	rows, err := read()
	if err != nil {
		return result, err
	}
	finish := func(rows []nativeRecord) (memory.NativeIngestResult, error) {
		return conversationResult(result.SourceSessionID, content, rows)
	}
	if len(rows) > 0 {
		return finish(rows)
	}
	requestKey := "conversation\x00" + result.SourceSessionID
	if a.uncertain[requestKey] {
		return result, memory.ErrIndexUnknown
	}
	a.uncertain[requestKey] = true
	_, _ = a.request(ctx, http.MethodPost, "/messages", nil, map[string]string{"memory_id": a.bank, "user_id": scope.user, "agent_id": scope.agent, "session_id": result.SourceSessionID, "user_input": turn.UserInput, "agent_response": turn.AgentResponse})
	// Read after any dispatch, including a lost/error response. This never
	// authorizes another POST when the remote outcome cannot be determined.
	rows, readErr := read()
	if readErr != nil || len(rows) == 0 {
		return result, memory.ErrIndexUnknown
	}
	resolved, finishErr := finish(rows)
	if finishErr == nil {
		delete(a.uncertain, requestKey)
	}
	return resolved, finishErr
}

// Native validity dates describe type-specific time (including an episodic
// event's beginning/end), rather than a universal retention or TTL interval.
func usable(row nativeRecord) bool {
	return row.Status && row.forgotten == ""
}

func (a *Adapter) SearchMemories(ctx context.Context, token, query string, limit int) ([]memory.NativeMessage, error) {
	return a.searchMemories(ctx, token, query, memory.NativeSearchOptions{Limit: limit, SimilarityThreshold: 0.2, KeywordWeight: 0.7}, 100)
}

// SearchDiagnostic returns only verified native hits. It never supplements the
// provider results with recent RAW conversations or changes normal chat defaults.
func (a *Adapter) SearchDiagnostic(ctx context.Context, token, query string, options memory.NativeSearchOptions) ([]memory.NativeMessage, error) {
	return a.searchMemories(ctx, token, query, options, 20)
}

func (a *Adapter) searchMemories(ctx context.Context, token, query string, options memory.NativeSearchOptions, maxLimit int) ([]memory.NativeMessage, error) {
	scope, err := a.scope(a.bank, token)
	if err != nil || strings.TrimSpace(query) == "" || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 2000 || options.Limit < 1 || options.Limit > maxLimit || !(options.SimilarityThreshold >= 0 && options.SimilarityThreshold <= 1) || !(options.KeywordWeight >= 0 && options.KeywordWeight <= 1) {
		return nil, memory.ErrInvalid
	}
	if err = a.checkBankMode(ctx, true); err != nil {
		return nil, err
	}
	params := url.Values{"memory_id": {a.bank}, "user_id": {scope.user}, "agent_id": {scope.agent}, "query": {query}, "top_n": {strconv.Itoa(options.Limit)}, "similarity_threshold": {strconv.FormatFloat(options.SimilarityThreshold, 'f', -1, 64)}, "keywords_similarity_weight": {strconv.FormatFloat(options.KeywordWeight, 'f', -1, 64)}}
	raw, err := a.request(ctx, http.MethodGet, "/messages/search", params, nil)
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil || len(rows) > options.Limit {
		return nil, memory.ErrIndexUnknown
	}
	result := []memory.NativeMessage{}
	seen := map[string]bool{}
	parents := map[string]nativeRecord{}
	for _, rawRow := range rows {
		listed, err := a.typedMessage(rawRow, scope, true)
		if err != nil || seen[listed.ID] {
			return nil, memory.ErrIndexUnknown
		}
		full, err := a.readTyped(ctx, scope, listed.ID)
		if err != nil || !reflect.DeepEqual(full.NativeMessage, listed.NativeMessage) || full.forgotten != listed.forgotten {
			return nil, memory.ErrIndexUnknown
		}
		seen[full.ID] = true
		if !usable(full) {
			continue
		}
		if full.Type != "raw" {
			parent, present := parents[full.SourceID]
			if !present {
				parent, err = a.readTyped(ctx, scope, full.SourceID)
				if err != nil {
					return nil, err
				}
				parents[full.SourceID] = parent
			}
			if parent.Type != "raw" || parent.SourceSessionID != full.SourceSessionID {
				return nil, memory.ErrIndexUnknown
			}
			if !usable(parent) {
				continue
			}
			full = withParentStatus(full, parent)
		}
		result = append(result, full.NativeMessage)
	}
	// Keep the provider's ranking; native DTOs do not expose similarity scores.
	return result, nil
}
