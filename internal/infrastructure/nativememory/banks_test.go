package nativememory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"control-panel/internal/memoryapp"
	"github.com/stretchr/testify/require"
)

var provisionSpec = memoryapp.BankSpec{Name: "hub-memory-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ChatModel: "356", EmbeddingModel: "337"}

type bankProvisionFixture struct {
	t                      *testing.T
	mu                     sync.Mutex
	server                 *httptest.Server
	client                 *BankProvisioner
	rows                   []provisioningBank
	posts, lists, configs  int
	snapshots              int
	skipWrite, lostPost    bool
	rejectPost, rejectList bool
	hideConfig             bool
	postReturnNoID         bool
	configOverride         func(provisioningBank) provisioningBank
	listOverride           func(int, int) ([]provisioningBank, int)
	rawList                map[string]any
}

func provisionRow(id string) provisioningBank {
	return provisioningBank{ID: id, Name: provisionSpec.Name, Owner: "native_owner", Types: []string{"raw", "semantic", "episodic", "procedural"}, Permissions: "me", Storage: "table", ChatModel: provisionSpec.ChatModel, EmbeddingModel: provisionSpec.EmbeddingModel}
}

func newBankProvisionFixture(t *testing.T) *bankProvisionFixture {
	f := &bankProvisionFixture{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	var err error
	f.client, err = NewBankProvisioner(BankProvisionerConfig{BaseURL: f.server.URL, APIKey: "provision-test-token", ExpectedOwnerID: "native_owner"})
	require.NoError(t, err)
	return f
}

func (f *bankProvisionFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Equal(f.t, "Bearer provision-test-token", r.Header.Get("Authorization"))
	require.Empty(f.t, r.Header.Get("Idempotency-Key"))
	respond := func(data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"retcode": 0, "retmsg": "success", "data": data})
	}
	switch {
	case r.URL.Path == "/api/v1/memories" && r.Method == http.MethodPost:
		f.posts++
		var body struct {
			Name      string   `json:"name"`
			Types     []string `json:"memory_type"`
			Chat      string   `json:"llm_id"`
			Embedding string   `json:"embd_id"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		require.NoError(f.t, decoder.Decode(&body))
		require.Equal(f.t, provisionSpec.Name, body.Name)
		require.Equal(f.t, []string{"raw", "semantic", "episodic", "procedural"}, body.Types)
		require.Equal(f.t, provisionSpec.ChatModel, body.Chat)
		require.Equal(f.t, provisionSpec.EmbeddingModel, body.Embedding)
		created := provisionRow("created_bank")
		if !f.skipWrite {
			f.rows = append(f.rows, created)
		}
		if f.lostPost {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(f.t, err)
			_ = conn.Close()
			return
		}
		if f.rejectPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"retcode": 500, "retmsg": "secret provider body should not escape", "data": false})
			return
		}
		if f.postReturnNoID {
			respond(nil)
			return
		}
		respond(map[string]any{"id": created.ID, "api_key": "secret provider credential", "name": "untrusted POST metadata"})
	case r.URL.Path == "/api/v1/memories" && r.Method == http.MethodGet:
		f.lists++
		require.Equal(f.t, provisionSpec.Name, r.URL.Query().Get("keywords"))
		require.Equal(f.t, "native_owner", r.URL.Query().Get("owner_ids"))
		require.Equal(f.t, "100", r.URL.Query().Get("page_size"))
		if f.rejectList {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 503, "message": "secret provider body should not escape"})
			return
		}
		if f.rawList != nil {
			respond(f.rawList)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 1 {
			f.snapshots++
		}
		rows, total := []provisioningBank{}, len(f.rows)
		if f.listOverride != nil {
			rows, total = f.listOverride(page, f.snapshots)
		} else {
			for i := (page - 1) * bankListPageSize; i < len(f.rows) && i < page*bankListPageSize; i++ {
				rows = append(rows, f.rows[i])
			}
		}
		respond(map[string]any{"memory_list": rows, "total_count": total})
	case strings.HasPrefix(r.URL.Path, "/api/v1/memories/") && strings.HasSuffix(r.URL.Path, "/config") && r.Method == http.MethodGet:
		f.configs++
		if f.hideConfig {
			_ = json.NewEncoder(w).Encode(map[string]any{"retcode": 404, "retmsg": "secret internal lookup detail", "data": false})
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/memories/"), "/config")
		for _, row := range f.rows {
			if row.ID == id {
				if f.configOverride != nil {
					row = f.configOverride(row)
				}
				respond(row)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"retcode": 404, "retmsg": "secret internal lookup detail", "data": false})
	default:
		f.t.Errorf("unexpected provisioning request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestBankProvisionerCreatesOnceAndIndependentlyReadsConfiguration(t *testing.T) {
	f := newBankProvisionFixture(t)
	bank, err := f.client.Create(context.Background(), provisionSpec)
	require.NoError(t, err)
	require.Equal(t, memoryapp.Bank{ID: "created_bank", Name: provisionSpec.Name, ChatModel: "356", EmbeddingModel: "337"}, bank)
	require.Equal(t, 1, f.posts)
	require.Equal(t, 1, f.configs)
	require.Zero(t, f.lists)
	repeated, err := f.client.Create(context.Background(), provisionSpec)
	require.NoError(t, err)
	require.Equal(t, bank, repeated)
	require.Equal(t, 1, f.posts)
	require.Equal(t, 4, f.lists, "each complete SQL snapshot traverses through an empty page")
	changed := provisionSpec
	changed.ChatModel = "another-reference"
	_, err = f.client.Create(context.Background(), changed)
	require.ErrorIs(t, err, memoryapp.ErrInvalid)
	require.Equal(t, 1, f.posts)
}

func TestBankProvisionerUnknownWritesOnlyRecoverThroughReads(t *testing.T) {
	for _, response := range []string{"lost", "business-error", "no-id"} {
		t.Run(response, func(t *testing.T) {
			f := newBankProvisionFixture(t)
			f.lostPost = response == "lost"
			f.rejectPost = response == "business-error"
			f.postReturnNoID = response == "no-id"
			bank, err := f.client.Create(context.Background(), provisionSpec)
			require.NoError(t, err)
			require.Equal(t, "created_bank", bank.ID)
			require.Equal(t, 1, f.posts)
			_, err = f.client.Create(context.Background(), provisionSpec)
			require.NoError(t, err)
			require.Equal(t, 1, f.posts)
		})
	}
	f := newBankProvisionFixture(t)
	f.skipWrite, f.lostPost = true, true
	_, err := f.client.Create(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnknown)
	_, err = f.client.Create(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnknown)
	_, err = f.client.Find(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrNotFound)
	require.Equal(t, 1, f.posts)
	// A fresh process must be instructed by Hub's durable state to Find, not
	// to call Create again. Its empty read cannot authorize another dispatch.
	restarted, err := NewBankProvisioner(BankProvisionerConfig{BaseURL: f.server.URL, APIKey: "provision-test-token", ExpectedOwnerID: "native_owner"})
	require.NoError(t, err)
	_, err = restarted.Find(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrNotFound)
	require.Equal(t, 1, f.posts)
}

func TestBankProvisionerRejectsNonmatchingIndependentReadback(t *testing.T) {
	for _, field := range []string{"identity", "renamed-collision", "owner", "shared", "storage", "missing-type", "duplicate-type", "canonical-model-different", "embedding"} {
		t.Run(field, func(t *testing.T) {
			f := newBankProvisionFixture(t)
			f.configOverride = func(row provisioningBank) provisioningBank {
				switch field {
				case "identity":
					row.ID = "other_bank"
				case "renamed-collision":
					row.Name += "(1)"
				case "owner":
					row.Owner = "other_owner"
				case "shared":
					row.Permissions = "team"
				case "storage":
					row.Storage = "graph"
				case "missing-type":
					row.Types = []string{"raw"}
				case "duplicate-type":
					row.Types = []string{"raw", "semantic", "semantic", "procedural"}
				case "canonical-model-different":
					row.ChatModel = "qwen-plus@another-provider"
				case "embedding":
					row.EmbeddingModel = "other"
				}
				return row
			}
			bank, err := f.client.Create(context.Background(), provisionSpec)
			require.ErrorIs(t, err, memoryapp.ErrUnknown)
			require.Empty(t, bank.ID)
			require.Equal(t, 1, f.posts)
		})
	}
}

func TestBankFindUsesExactNameAndCompleteBoundedStableSQLPagination(t *testing.T) {
	f := newBankProvisionFixture(t)
	for i := 0; i < 100; i++ {
		row := provisionRow(fmt.Sprintf("other_%d", i))
		row.Name += fmt.Sprintf("(%d)", i+1)
		f.rows = append(f.rows, row)
	}
	f.rows = append(f.rows, provisionRow("exact_bank"))
	bank, err := f.client.Find(context.Background(), provisionSpec)
	require.NoError(t, err)
	require.Equal(t, "exact_bank", bank.ID)
	require.Equal(t, 6, f.lists)
	require.Zero(t, f.posts)
	// Substring/suffix matches never establish ownership of the reserved name.
	f.rows = f.rows[:100]
	_, err = f.client.Find(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrNotFound)
}

func TestBankFindDoesNotCallAnInconsistentConfigReadAuthoritativeAbsence(t *testing.T) {
	f := newBankProvisionFixture(t)
	f.rows = []provisioningBank{provisionRow("listed_bank")}
	f.hideConfig = true
	_, err := f.client.Find(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnknown)
	require.NotErrorIs(t, err, memoryapp.ErrNotFound)
	require.NotContains(t, err.Error(), "secret")
	require.Zero(t, f.posts)
}

func TestBankFindRejectsAmbiguityTruncationAndChangingSnapshots(t *testing.T) {
	for _, scenario := range []string{"two-exact", "duplicate-id", "partial-count", "partial-nonterminal", "changing-total", "page-local-total", "changed-snapshot", "exceeded-bound", "missing-total", "null-list"} {
		t.Run(scenario, func(t *testing.T) {
			f := newBankProvisionFixture(t)
			one, two := provisionRow("one"), provisionRow("two")
			switch scenario {
			case "two-exact":
				f.rows = []provisioningBank{one, two}
			case "duplicate-id":
				f.rows = []provisioningBank{one, one}
			case "partial-count":
				f.listOverride = func(page, _ int) ([]provisioningBank, int) {
					if page == 1 {
						return []provisioningBank{one}, 2
					}
					return []provisioningBank{}, 2
				}
			case "partial-nonterminal":
				f.listOverride = func(page, _ int) ([]provisioningBank, int) {
					if page == 1 {
						return []provisioningBank{one}, 2
					}
					return []provisioningBank{two}, 2
				}
			case "changing-total":
				f.listOverride = func(page, _ int) ([]provisioningBank, int) {
					if page == 1 {
						return []provisioningBank{one}, 1
					}
					return []provisioningBank{}, 2
				}
			case "page-local-total":
				f.listOverride = func(page, _ int) ([]provisioningBank, int) {
					if page == 1 {
						return []provisioningBank{one}, 1
					}
					return []provisioningBank{}, 0
				}
			case "changed-snapshot":
				f.listOverride = func(page, snapshot int) ([]provisioningBank, int) {
					if page > 1 {
						return []provisioningBank{}, 1
					}
					if snapshot == 1 {
						return []provisioningBank{one}, 1
					}
					return []provisioningBank{two}, 1
				}
			case "exceeded-bound":
				f.listOverride = func(_, _ int) ([]provisioningBank, int) { return []provisioningBank{}, 10001 }
			case "missing-total":
				f.rawList = map[string]any{"memory_list": []provisioningBank{}}
			case "null-list":
				f.rawList = map[string]any{"memory_list": nil, "total_count": 0}
			}
			_, err := f.client.Find(context.Background(), provisionSpec)
			require.ErrorIs(t, err, memoryapp.ErrUnknown)
			require.Zero(t, f.posts)
			require.Zero(t, f.configs)
		})
	}
}

func TestBankProvisionerRejectsInvalidInputBeforeIOAndSanitizesFailures(t *testing.T) {
	for _, config := range []BankProvisionerConfig{{}, {BaseURL: "https://user:secret@example.test", APIKey: "key"}, {BaseURL: "https://example.test?private=query", APIKey: "key"}, {BaseURL: "https://example.test", APIKey: "key\nsecret"}, {BaseURL: "https://example.test", APIKey: "key", ExpectedOwnerID: "owner/other"}} {
		_, err := NewBankProvisioner(config)
		require.ErrorIs(t, err, memoryapp.ErrInvalid)
		require.NotContains(t, err.Error(), "secret")
	}
	f := newBankProvisionFixture(t)
	for _, spec := range []memoryapp.BankSpec{{}, {Name: " a ", ChatModel: "356", EmbeddingModel: "337"}, {Name: "a", ChatModel: "356\n", EmbeddingModel: "337"}, {Name: strings.Repeat("a", 129), ChatModel: "356", EmbeddingModel: "337"}, {Name: "valid", ChatModel: "", EmbeddingModel: "337"}} {
		_, err := f.client.Create(context.Background(), spec)
		require.ErrorIs(t, err, memoryapp.ErrInvalid)
		_, err = f.client.Find(context.Background(), spec)
		require.ErrorIs(t, err, memoryapp.ErrInvalid)
	}
	require.Zero(t, f.posts+f.lists+f.configs)
	f.skipWrite, f.rejectPost, f.rejectList = true, true, true
	_, err := f.client.Create(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnknown)
	require.NotContains(t, err.Error(), "secret")
	_, err = f.client.Find(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnavailable)
	require.NotContains(t, err.Error(), "secret")
}

func TestBankProvisionerDoesNotForwardCredentialsThroughRedirect(t *testing.T) {
	var forwarded atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forwarded.Add(1); w.WriteHeader(200) }))
	t.Cleanup(foreign.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	original := &http.Client{}
	client, err := NewBankProvisioner(BankProvisionerConfig{BaseURL: redirector.URL, APIKey: "private-provision-token", HTTPClient: original})
	require.NoError(t, err)
	require.Nil(t, original.CheckRedirect)
	_, err = client.Create(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnknown)
	_, err = client.Find(context.Background(), provisionSpec)
	require.ErrorIs(t, err, memoryapp.ErrUnavailable)
	require.Zero(t, forwarded.Load())
}

func TestBankProvisionerConcurrentCreateDispatchesAtMostOnce(t *testing.T) {
	f := newBankProvisionFixture(t)
	var group sync.WaitGroup
	errorsSeen := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := f.client.Create(context.Background(), provisionSpec)
			errorsSeen <- err
		}()
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		require.True(t, err == nil || errors.Is(err, memoryapp.ErrUnknown))
	}
	require.Equal(t, 1, f.posts)
	bank, err := f.client.Find(context.Background(), provisionSpec)
	require.NoError(t, err)
	require.Equal(t, "created_bank", bank.ID)
}
