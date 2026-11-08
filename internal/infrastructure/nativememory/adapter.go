// Package nativememory uses a dedicated native MultiRAG Memory bank.
// DocumentID is a logical scope token, never a document.
// It does not implement forgetting or physical deletion.
package nativememory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"control-panel/internal/memory"
)

const schema = "hub-memory-fact-v1"

var bankPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var scopePattern = regexp.MustCompile(`^memory-[0-9a-f]{64}$`)
var numberPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

type Config struct {
	BaseURL    string
	APIKey     string
	BankID     string
	HTTPClient *http.Client
	// Bounds include the final empty page used to verify scan completion.
	PageSize int
	MaxPages int
}

type Adapter struct {
	base, key, bank string
	client          *http.Client
	pageSize, pages int
	mu              sync.Mutex
	uncertain       map[string]bool
}

var _ memory.Index = (*Adapter)(nil)

func New(cfg Config) (*Adapter, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(cfg.APIKey) == "" || strings.ContainsAny(cfg.APIKey, "\r\n") || !bankPattern.MatchString(cfg.BankID) {
		return nil, memory.ErrInvalid
	}
	if cfg.PageSize == 0 {
		cfg.PageSize = 100
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = 101
	}
	if cfg.PageSize < 1 || cfg.PageSize > 100 || cfg.MaxPages < 2 || cfg.MaxPages > 101 {
		return nil, memory.ErrInvalid
	}
	client := http.Client{Timeout: 15 * time.Second}
	if cfg.HTTPClient != nil {
		client = *cfg.HTTPClient
	}
	// No redirected request may forward the bank credential to another URL.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Adapter{base: strings.TrimRight(cfg.BaseURL, "/"), key: cfg.APIKey, bank: cfg.BankID, client: &client, pageSize: cfg.PageSize, pages: cfg.MaxPages, uncertain: map[string]bool{}}, nil
}

type nativeScope struct{ user, agent, session string }

func (a *Adapter) scope(bank, token string) (nativeScope, error) {
	if a == nil || a.client == nil || bank != a.bank || !scopePattern.MatchString(token) {
		return nativeScope{}, memory.ErrInvalid
	}
	hash := strings.TrimPrefix(token, "memory-")
	return nativeScope{"hub-user-" + hash, "hub-agent-" + hash, "hub-life-" + hash}, nil
}

func (a *Adapter) request(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, memory.ErrInvalid
		}
		payload = bytes.NewReader(encoded)
	}
	endpoint := a.base + "/api/v1" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, memory.ErrIndexUnknown
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, memory.ErrIndexUnknown
	}
	defer res.Body.Close()
	const maxResponse = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse || res.StatusCode != http.StatusOK {
		return nil, memory.ErrIndexUnknown
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil || envelope == nil {
		return nil, memory.ErrIndexUnknown
	}
	found := false
	for _, key := range []string{"code", "retcode"} {
		if value, ok := envelope[key]; ok {
			found = true
			if string(value) != "0" {
				return nil, memory.ErrIndexUnknown
			}
		}
	}
	if !found {
		return nil, memory.ErrIndexUnknown
	}
	return envelope["data"], nil
}

func (a *Adapter) checkBank(ctx context.Context) error {
	return a.checkBankMode(ctx, false)
}

func (a *Adapter) bankConfig(ctx context.Context) (json.RawMessage, []string, error) {
	raw, err := a.request(ctx, http.MethodGet, "/memories/"+a.bank+"/config", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var config struct {
		ID          string   `json:"id"`
		TenantID    string   `json:"tenant_id"`
		MemoryType  []string `json:"memory_type"`
		Permissions string   `json:"permissions"`
	}
	if json.Unmarshal(raw, &config) != nil || config.ID != a.bank || !bankPattern.MatchString(config.TenantID) || !validMemoryTypes(config.MemoryType) || config.Permissions != "me" {
		return nil, nil, memory.ErrIndexUnknown
	}
	return raw, config.MemoryType, nil
}

func (a *Adapter) checkBankMode(ctx context.Context, allTypes bool) error {
	_, types, err := a.bankConfig(ctx)
	if err != nil {
		return err
	}
	if allTypes && len(types) != 4 {
		return memory.ErrIndexUnknown
	}
	return nil
}

// EnsureDocument validates a dedicated bank and returns the reversible,
// deterministic scope token. There is no upstream document to create/recover.
func (a *Adapter) EnsureDocument(ctx context.Context, bank, name string, _ bool) (string, error) {
	if _, err := a.scope(bank, name); err != nil {
		return "", err
	}
	if err := a.checkBank(ctx); err != nil {
		return "", err
	}
	return name, nil
}

type quoteEnvelope struct {
	Schema string `json:"schema"`
	Quote  string `json:"quote"`
}

func encodeQuote(content string) (string, error) {
	if !utf8.ValidString(content) || strings.TrimSpace(content) == "" || utf8.RuneCountInString(content) > 200 {
		return "", memory.ErrInvalid
	}
	raw, err := json.Marshal(quoteEnvelope{schema, content})
	return string(raw), err
}

func decodeQuote(content string) (string, error) {
	const prefix, suffix = "User Input: ", "\nAgent Response: "
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) {
		return "", memory.ErrIndexUnknown
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(content, prefix), suffix)
	var envelope quoteEnvelope
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || envelope.Schema != schema {
		return "", memory.ErrIndexUnknown
	}
	canonical, err := encodeQuote(envelope.Quote)
	if err != nil || canonical != payload {
		return "", memory.ErrIndexUnknown
	}
	return envelope.Quote, nil
}

type message struct {
	id, content, quote, forgottenAt string
	active                          bool
}

// Only canonical integer identities are accepted; float64 never touches IDs.
func messageID(raw json.RawMessage, allowZero bool) (string, bool) {
	value := string(raw)
	if strings.HasPrefix(value, `"`) {
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}
	}
	if allowZero && value == "0" {
		return value, true
	}
	if !numberPattern.MatchString(value) {
		return "", false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return value, err == nil && id > 0
}

func nativeBool(raw json.RawMessage) (bool, bool) {
	switch string(raw) {
	case "true", "1", `"1"`:
		return true, true
	case "false", "0", `"0"`:
		return false, true
	default:
		return false, false
	}
}

func (a *Adapter) parseMessage(raw json.RawMessage, scope nativeScope, requireContent bool) (message, error) {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return message{}, memory.ErrIndexUnknown
	}
	for field, expected := range map[string]string{"memory_id": a.bank, "user_id": scope.user, "agent_id": scope.agent, "session_id": scope.session, "message_type": "raw"} {
		var actual string
		if json.Unmarshal(value[field], &actual) != nil || actual != expected {
			return message{}, memory.ErrIndexUnknown
		}
	}
	id, ok := messageID(value["message_id"], false)
	source, sourceOK := messageID(value["source_id"], true)
	active, statusOK := nativeBool(value["status"])
	if !ok || !sourceOK || source != "0" || !statusOK {
		return message{}, memory.ErrIndexUnknown
	}
	if alias, present := value["id"]; present {
		var actual string
		if json.Unmarshal(alias, &actual) != nil || actual != a.bank+"_"+id {
			return message{}, memory.ErrIndexUnknown
		}
	}
	forgetRaw, present := value["forget_at"]
	if !present {
		return message{}, memory.ErrIndexUnknown
	}
	forgottenAt := ""
	if string(forgetRaw) != "null" {
		if json.Unmarshal(forgetRaw, &forgottenAt) != nil || forgottenAt == "-" {
			return message{}, memory.ErrIndexUnknown
		}
	}
	result := message{id: id, active: active, forgottenAt: forgottenAt}
	if rawContent, present := value["content"]; present {
		if json.Unmarshal(rawContent, &result.content) != nil {
			return message{}, memory.ErrIndexUnknown
		}
		var err error
		result.quote, err = decodeQuote(result.content)
		if err != nil {
			return message{}, err
		}
	} else if requireContent {
		return message{}, memory.ErrIndexUnknown
	}
	return result, nil
}

func (a *Adapter) readMessage(ctx context.Context, scope nativeScope, id string) (message, error) {
	raw, err := a.request(ctx, http.MethodGet, "/messages/"+a.bank+":"+id+"/content", nil, nil)
	if err != nil {
		return message{}, err
	}
	result, err := a.parseMessage(raw, scope, true)
	if err != nil || result.id != id {
		return message{}, memory.ErrIndexUnknown
	}
	return result, nil
}

// scan traverses to an empty page. ES reports a global total; current Milvus
// reports the page count. We accept either coherent documented shape, never
// use the first total as a reason to stop, and reject duplicates/truncation.
func (a *Adapter) scan(ctx context.Context, scope nativeScope) (map[string]message, error) {
	seen := map[string]message{}
	globalTotal, globalMode, pageMode := -1, true, true
	partialPage := false
	for page := 1; page <= a.pages; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(a.pageSize)}, "agent_id": {scope.agent}, "keywords": {scope.session}}
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
		if json.Unmarshal(raw, &listing) != nil || listing.Messages == nil || listing.Messages.List == nil || listing.Messages.Total == nil || len(listing.Messages.List) > a.pageSize {
			return nil, memory.ErrIndexUnknown
		}
		rows, total := listing.Messages.List, *listing.Messages.Total
		if total < len(rows) || total > a.pageSize*(a.pages-1) || partialPage && len(rows) != 0 {
			return nil, memory.ErrIndexUnknown
		}
		partialPage = len(rows) > 0 && len(rows) < a.pageSize
		if globalTotal < 0 {
			globalTotal = total
		}
		globalMode = globalMode && total == globalTotal
		pageMode = pageMode && total == len(rows)
		if !globalMode && !pageMode {
			return nil, memory.ErrIndexUnknown
		}
		for _, row := range rows {
			listed, err := a.parseMessage(row, scope, false)
			if err != nil {
				return nil, err
			}
			if _, duplicate := seen[listed.id]; duplicate {
				return nil, memory.ErrIndexUnknown
			}
			var extracts struct {
				Extract []json.RawMessage `json:"extract"`
			}
			if json.Unmarshal(row, &extracts) != nil || extracts.Extract == nil || len(extracts.Extract) != 0 {
				return nil, memory.ErrIndexUnknown
			}
			full, err := a.readMessage(ctx, scope, listed.id)
			if err != nil || full.active != listed.active || full.forgottenAt != listed.forgottenAt || (listed.content != "" && listed.content != full.content) {
				return nil, memory.ErrIndexUnknown
			}
			seen[full.id] = full
		}
		if len(rows) == 0 {
			if globalMode && len(seen) == globalTotal || pageMode {
				return seen, nil
			}
			return nil, memory.ErrIndexUnknown
		}
	}
	return nil, memory.ErrIndexUnknown
}

func (a *Adapter) find(ctx context.Context, scope nativeScope, content string) (string, error) {
	first, err := a.scan(ctx, scope)
	if err != nil {
		return "", err
	}
	second, err := a.scan(ctx, scope)
	if err != nil || !reflect.DeepEqual(first, second) {
		return "", memory.ErrIndexUnknown
	}
	found := ""
	for _, row := range second {
		if row.quote != content {
			continue
		}
		if !row.active || row.forgottenAt != "" || found != "" {
			return "", memory.ErrIndexUnknown
		}
		found = row.id
	}
	return found, nil
}

// Find is read-only. Absence in a complete snapshot never authorizes replay of
// an earlier request whose remote completion is unknown.
func (a *Adapter) Find(ctx context.Context, bank, token, content string) (string, error) {
	scope, err := a.scope(bank, token)
	if err != nil {
		return "", err
	}
	if _, err = encodeQuote(content); err != nil {
		return "", err
	}
	if err = a.checkBank(ctx); err != nil {
		return "", err
	}
	return a.find(ctx, scope, content)
}

func (a *Adapter) Put(ctx context.Context, bank, token, content string) (string, error) {
	scope, err := a.scope(bank, token)
	if err != nil {
		return "", err
	}
	payload, err := encodeQuote(content)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err = a.checkBank(ctx); err != nil {
		return "", err
	}
	id, err := a.find(ctx, scope, content)
	if err != nil || id != "" {
		return id, err
	}
	requestKey := token + "\x00" + payload
	if a.uncertain[requestKey] {
		return "", memory.ErrIndexUnknown
	}
	// Set before dispatch: a timeout/cancellation cannot prove the server stopped.
	a.uncertain[requestKey] = true
	_, err = a.request(ctx, http.MethodPost, "/messages", nil, map[string]string{"memory_id": bank, "user_id": scope.user, "agent_id": scope.agent, "session_id": scope.session, "user_input": payload, "agent_response": ""})
	if err != nil {
		return "", memory.ErrIndexUnknown
	}
	id, err = a.find(ctx, scope, content)
	if err != nil || id == "" {
		return "", memory.ErrIndexUnknown
	}
	delete(a.uncertain, requestKey)
	return id, nil
}

// Remove only confirms status:false for a correction. It is not physical
// deletion and must never back a user deletion/consent/account-cleanup receipt.
func (a *Adapter) Remove(ctx context.Context, bank, token, id string) error {
	scope, err := a.scope(bank, token)
	if err != nil {
		return err
	}
	if parsed, ok := messageID(json.RawMessage(id), false); !ok || parsed != id {
		return memory.ErrInvalid
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err = a.checkBank(ctx); err != nil {
		return err
	}
	before, err := a.readMessage(ctx, scope, id)
	if err != nil || before.forgottenAt != "" {
		return memory.ErrIndexUnknown
	}
	if !before.active {
		return nil
	}
	// Even an unconfirmed PUT can have committed. Read its effect rather than
	// repeating a mutation or treating a boolean response as proof of visibility.
	_, _ = a.request(ctx, http.MethodPut, "/messages/"+bank+":"+id, nil, map[string]bool{"status": false})
	after, err := a.readMessage(ctx, scope, id)
	if err != nil || after.active || after.forgottenAt != "" || after.content != before.content {
		return memory.ErrIndexUnknown
	}
	return nil
}

func (a *Adapter) DropDocument(context.Context, string, string) error {
	return memory.ErrUnsupported
}

func (a *Adapter) Search(ctx context.Context, bank, token, query string, limit int) ([]memory.Hit, error) {
	scope, err := a.scope(bank, token)
	if err != nil || strings.TrimSpace(query) == "" || limit < 1 || limit > 100 || !utf8.ValidString(query) {
		return nil, memory.ErrInvalid
	}
	if err = a.checkBank(ctx); err != nil {
		return nil, err
	}
	params := url.Values{"memory_id": {bank}, "user_id": {scope.user}, "agent_id": {scope.agent}, "session_id": {scope.session}, "query": {query}, "top_n": {strconv.Itoa(limit)}, "similarity_threshold": {"0.2"}, "keywords_similarity_weight": {"0.7"}}
	raw, err := a.request(ctx, http.MethodGet, "/messages/search", params, nil)
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil || len(rows) > limit {
		return nil, memory.ErrIndexUnknown
	}
	hits := make([]memory.Hit, 0, len(rows))
	seen, quotes := map[string]bool{}, map[string]bool{}
	for _, rawRow := range rows {
		row, err := a.parseMessage(rawRow, scope, true)
		if err != nil || !row.active || row.forgottenAt != "" || seen[row.id] || quotes[row.quote] {
			return nil, memory.ErrIndexUnknown
		}
		full, err := a.readMessage(ctx, scope, row.id)
		if err != nil || !full.active || full.forgottenAt != "" || full.content != row.content {
			return nil, memory.ErrIndexUnknown
		}
		seen[row.id], quotes[row.quote] = true, true
		// The public native message DTO omits similarity scores. Preserve its
		// ranking order; zero is an unavailable score, not a fabricated metric.
		hits = append(hits, memory.Hit{ChunkID: row.id, DocumentID: token, Content: row.quote})
	}
	return hits, nil
}
