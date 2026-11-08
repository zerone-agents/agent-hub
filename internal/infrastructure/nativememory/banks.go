package nativememory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"control-panel/internal/memoryapp"
)

const bankListPageSize, bankListMaxPages = 100, 101

type BankProvisionerConfig struct {
	BaseURL         string
	APIKey          string
	HTTPClient      *http.Client
	ExpectedOwnerID string
}

// BankProvisioner only creates native configuration and reconciles it through
// reads. It neither creates messages nor deletes an uncertain/orphaned bank.
// Hub must durably reserve the unique name before Create and never dispatch a
// second Create after a restart. The local map is only an additional guard.
type BankProvisioner struct {
	base, key, owner string
	client           *http.Client
	mu               sync.Mutex
	attempted        map[string]memoryapp.BankSpec
}

var _ memoryapp.BankProvisioner = (*BankProvisioner)(nil)

func NewBankProvisioner(cfg BankProvisionerConfig) (*BankProvisioner, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(cfg.APIKey) == "" || strings.ContainsAny(cfg.APIKey, "\r\n") || (cfg.ExpectedOwnerID != "" && !bankPattern.MatchString(cfg.ExpectedOwnerID)) {
		return nil, fmt.Errorf("%w: invalid native bank client configuration", memoryapp.ErrInvalid)
	}
	client := http.Client{Timeout: 15 * time.Second}
	if cfg.HTTPClient != nil {
		client = *cfg.HTTPClient
		if client.Timeout <= 0 {
			client.Timeout = 15 * time.Second
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &BankProvisioner{base: strings.TrimRight(cfg.BaseURL, "/"), key: cfg.APIKey, owner: cfg.ExpectedOwnerID, client: &client, attempted: map[string]memoryapp.BankSpec{}}, nil
}

func validBankText(text string) bool {
	return utf8.ValidString(text) && text != "" && strings.TrimSpace(text) == text && utf8.RuneCountInString(text) <= 128 && !strings.ContainsFunc(text, unicode.IsControl)
}

func validBankSpec(spec memoryapp.BankSpec) bool {
	return validBankText(spec.Name) && validBankText(spec.ChatModel) && validBankText(spec.EmbeddingModel)
}

func (p *BankProvisioner) request(ctx context.Context, method, path string, query url.Values, body any, missingIsNotFound bool) (json.RawMessage, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("%w: native bank client is not configured", memoryapp.ErrUnavailable)
	}
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid native bank request", memoryapp.ErrInvalid)
		}
		payload = bytes.NewReader(raw)
	}
	endpoint := p.base + "/api/v1" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("%w: native bank request could not be prepared", memoryapp.ErrUnavailable)
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Do not add an Idempotency-Key: native create has no such contract, and
	// Go's transport can treat that header as permission to replay a POST.
	response, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: native bank request unavailable", memoryapp.ErrUnavailable)
	}
	defer response.Body.Close()
	const maxResponse = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		return nil, fmt.Errorf("%w: native bank response was incomplete", memoryapp.ErrUnknown)
	}
	if response.StatusCode != http.StatusOK {
		if missingIsNotFound && response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: native bank is not visible", memoryapp.ErrNotFound)
		}
		return nil, fmt.Errorf("%w: native bank request was rejected", memoryapp.ErrUnavailable)
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil || envelope == nil {
		return nil, fmt.Errorf("%w: native bank envelope was invalid", memoryapp.ErrUnknown)
	}
	found := false
	for _, key := range []string{"code", "retcode"} {
		if value, present := envelope[key]; present {
			found = true
			if string(value) != "0" {
				if missingIsNotFound && string(value) == "404" {
					return nil, fmt.Errorf("%w: native bank is not visible", memoryapp.ErrNotFound)
				}
				return nil, fmt.Errorf("%w: native bank operation was rejected", memoryapp.ErrUnavailable)
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: native bank status was missing", memoryapp.ErrUnknown)
	}
	return envelope["data"], nil
}

type provisioningBank struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Owner          string   `json:"tenant_id"`
	Types          []string `json:"memory_type"`
	Permissions    string   `json:"permissions"`
	Storage        string   `json:"storage_type"`
	ChatModel      string   `json:"llm_id"`
	EmbeddingModel string   `json:"embd_id"`
}

func (p *BankProvisioner) readBank(ctx context.Context, id string, spec memoryapp.BankSpec) (memoryapp.Bank, error) {
	if !bankPattern.MatchString(id) {
		return memoryapp.Bank{}, fmt.Errorf("%w: native bank identity was invalid", memoryapp.ErrUnknown)
	}
	raw, err := p.request(ctx, http.MethodGet, "/memories/"+id+"/config", nil, nil, true)
	if err != nil {
		return memoryapp.Bank{}, err
	}
	var bank provisioningBank
	if json.Unmarshal(raw, &bank) != nil || bank.ID != id || bank.Name != spec.Name || !bankPattern.MatchString(bank.Owner) || (p.owner != "" && bank.Owner != p.owner) || bank.Permissions != "me" || bank.Storage != "table" || len(bank.Types) != 4 || !validMemoryTypes(bank.Types) || bank.ChatModel != spec.ChatModel || bank.EmbeddingModel != spec.EmbeddingModel {
		return memoryapp.Bank{}, fmt.Errorf("%w: native bank readback did not match its reserved specification", memoryapp.ErrUnknown)
	}
	return memoryapp.Bank{ID: bank.ID, Name: bank.Name, ChatModel: bank.ChatModel, EmbeddingModel: bank.EmbeddingModel}, nil
}

// Create consumes this process's dispatch guard before a single POST. After any
// uncertain outcome only read-only Find is used, even on a repeated invocation.
func (p *BankProvisioner) Create(ctx context.Context, spec memoryapp.BankSpec) (memoryapp.Bank, error) {
	if !validBankSpec(spec) {
		return memoryapp.Bank{}, fmt.Errorf("%w: invalid native bank specification", memoryapp.ErrInvalid)
	}
	if p == nil || p.client == nil {
		return memoryapp.Bank{}, fmt.Errorf("%w: native bank client is not configured", memoryapp.ErrUnavailable)
	}
	p.mu.Lock()
	previous, attempted := p.attempted[spec.Name]
	if attempted && previous != spec {
		p.mu.Unlock()
		return memoryapp.Bank{}, fmt.Errorf("%w: reserved native bank specification changed", memoryapp.ErrInvalid)
	}
	if !attempted {
		p.attempted[spec.Name] = spec
	}
	p.mu.Unlock()
	if attempted {
		return p.recover(ctx, spec)
	}
	raw, err := p.request(ctx, http.MethodPost, "/memories", nil, map[string]any{"name": spec.Name, "memory_type": []string{"raw", "semantic", "episodic", "procedural"}, "llm_id": spec.ChatModel, "embd_id": spec.EmbeddingModel}, false)
	if err == nil {
		var created struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &created) == nil && bankPattern.MatchString(created.ID) {
			bank, readErr := p.readBank(ctx, created.ID, spec)
			if readErr == nil {
				return bank, nil
			}
			return memoryapp.Bank{}, fmt.Errorf("%w: created native bank could not be independently verified", memoryapp.ErrUnknown)
		}
	}
	return p.recover(ctx, spec)
}

func (p *BankProvisioner) recover(ctx context.Context, spec memoryapp.BankSpec) (memoryapp.Bank, error) {
	bank, err := p.Find(ctx, spec)
	if err != nil {
		return memoryapp.Bank{}, fmt.Errorf("%w: native bank creation requires read-only reconciliation", memoryapp.ErrUnknown)
	}
	return bank, nil
}

// scan uses the SQL bank-list contract: total_count is global, unlike native
// message-store totals. A final empty page proves bounded traversal completion.
func (p *BankProvisioner) scan(ctx context.Context, spec memoryapp.BankSpec) (map[string]provisioningBank, error) {
	seen := map[string]provisioningBank{}
	total, partial := -1, false
	for page := 1; page <= bankListMaxPages; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(bankListPageSize)}, "keywords": {spec.Name}}
		if p.owner != "" {
			query.Set("owner_ids", p.owner)
		}
		raw, err := p.request(ctx, http.MethodGet, "/memories", query, nil, false)
		if err != nil {
			return nil, err
		}
		var list struct {
			Banks []provisioningBank `json:"memory_list"`
			Total *int               `json:"total_count"`
		}
		if json.Unmarshal(raw, &list) != nil || list.Banks == nil || list.Total == nil || len(list.Banks) > bankListPageSize || *list.Total < len(list.Banks) || *list.Total > bankListPageSize*(bankListMaxPages-1) || partial && len(list.Banks) > 0 {
			return nil, fmt.Errorf("%w: native bank pagination was incomplete", memoryapp.ErrUnknown)
		}
		if total < 0 {
			total = *list.Total
		}
		if total != *list.Total {
			return nil, fmt.Errorf("%w: native bank count changed during lookup", memoryapp.ErrUnknown)
		}
		if len(list.Banks) == 0 {
			if len(seen) != total {
				return nil, fmt.Errorf("%w: native bank traversal did not match its count", memoryapp.ErrUnknown)
			}
			return seen, nil
		}
		partial = len(list.Banks) < bankListPageSize
		for _, bank := range list.Banks {
			if !bankPattern.MatchString(bank.ID) || !validBankText(bank.Name) {
				return nil, fmt.Errorf("%w: native bank list identity was invalid", memoryapp.ErrUnknown)
			}
			if _, duplicate := seen[bank.ID]; duplicate {
				return nil, fmt.Errorf("%w: native bank list repeated an identity", memoryapp.ErrUnknown)
			}
			seen[bank.ID] = bank
		}
	}
	return nil, fmt.Errorf("%w: native bank lookup exceeded its scan bound", memoryapp.ErrUnknown)
}

// Find never writes. Missing is an observed read result, never permission to
// replay a POST consumed by durable Hub state or by this process's guard.
func (p *BankProvisioner) Find(ctx context.Context, spec memoryapp.BankSpec) (memoryapp.Bank, error) {
	if !validBankSpec(spec) {
		return memoryapp.Bank{}, fmt.Errorf("%w: invalid native bank specification", memoryapp.ErrInvalid)
	}
	if p == nil || p.client == nil {
		return memoryapp.Bank{}, fmt.Errorf("%w: native bank client is not configured", memoryapp.ErrUnavailable)
	}
	first, err := p.scan(ctx, spec)
	if err != nil {
		return memoryapp.Bank{}, err
	}
	second, err := p.scan(ctx, spec)
	if err != nil {
		return memoryapp.Bank{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return memoryapp.Bank{}, fmt.Errorf("%w: native bank lookup did not stabilize", memoryapp.ErrUnknown)
	}
	id := ""
	for _, bank := range second {
		if bank.Name != spec.Name {
			continue
		}
		if id != "" {
			return memoryapp.Bank{}, fmt.Errorf("%w: native bank name matched multiple identities", memoryapp.ErrUnknown)
		}
		id = bank.ID
	}
	if id == "" {
		return memoryapp.Bank{}, fmt.Errorf("%w: reserved native bank is not visible", memoryapp.ErrNotFound)
	}
	bank, err := p.readBank(ctx, id, spec)
	if errors.Is(err, memoryapp.ErrNotFound) {
		return memoryapp.Bank{}, fmt.Errorf("%w: listed native bank could not be independently found", memoryapp.ErrUnknown)
	}
	return bank, err
}
