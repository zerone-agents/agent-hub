package nativememory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"control-panel/internal/memory"

	"github.com/stretchr/testify/require"
)

const testBank = "native_bank"

var testToken = "memory-" + strings.Repeat("a", 64)

type fixture struct {
	mu                                                       sync.Mutex
	t                                                        *testing.T
	adapter                                                  *Adapter
	server                                                   *httptest.Server
	rows                                                     []map[string]any
	posts, puts, deletes, lists, details                     int
	pageTotal                                                bool
	postError, lostResponse, skipPost, skipDisable, putError bool
	lostPutResponse                                          bool
	configTypes                                              []string
	searchRows                                               []map[string]any
	searchQuery                                              url.Values
	listOverride                                             func(int, int) ([]map[string]any, int)
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, configTypes: []string{"raw"}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	var err error
	f.adapter, err = New(Config{BaseURL: f.server.URL, APIKey: "contract-test-key", BankID: testBank, PageSize: 2, MaxPages: 5})
	require.NoError(t, err)
	return f
}

func (f *fixture) row(id, quote string) map[string]any {
	s, err := f.adapter.scope(testBank, testToken)
	require.NoError(f.t, err)
	payload, err := encodeQuote(quote)
	require.NoError(f.t, err)
	number, err := strconv.ParseInt(id, 10, 64)
	require.NoError(f.t, err)
	return map[string]any{"id": testBank + "_" + id, "message_id": number, "source_id": 0, "message_type": "raw", "memory_id": testBank, "user_id": s.user, "agent_id": s.agent, "session_id": s.session, "status": true, "forget_at": nil, "valid_at": "2026-11-06 10:00:00", "invalid_at": nil, "content": "User Input: " + payload + "\nAgent Response: "}
}

func copyRow(row map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range row {
		result[k] = v
	}
	return result
}

func (f *fixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer contract-test-key" {
		f.t.Error("missing native credential")
		w.WriteHeader(401)
		return
	}
	respond := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}) }
	scope, _ := f.adapter.scope(testBank, testToken)
	switch {
	case r.Method == http.MethodDelete:
		f.deletes++
		f.t.Error("native adapter must not call DELETE/forget")
		w.WriteHeader(405)
	case r.URL.Path == "/api/v1/memories/"+testBank+"/config":
		respond(map[string]any{"id": testBank, "tenant_id": "native_owner", "memory_type": f.configTypes, "permissions": "me"})
	case r.URL.Path == "/api/v1/memories/"+testBank:
		f.lists++
		require.Equal(f.t, scope.agent, r.URL.Query().Get("agent_id"))
		require.Equal(f.t, scope.session, r.URL.Query().Get("keywords"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		rows, total := []map[string]any{}, len(f.rows)
		if f.listOverride != nil {
			rows, total = f.listOverride(page, size)
		} else {
			for i := (page - 1) * size; i < len(f.rows) && i < page*size; i++ {
				rows = append(rows, f.rows[i])
			}
			if f.pageTotal {
				total = len(rows)
			}
		}
		metadata := []map[string]any{}
		for _, row := range rows {
			listed := copyRow(row)
			delete(listed, "content")
			listed["extract"] = []any{}
			metadata = append(metadata, listed)
		}
		respond(map[string]any{"messages": map[string]any{"message_list": metadata, "total_count": total}, "storage_type": "table"})
	case r.URL.Path == "/api/v1/messages/search":
		f.searchQuery = r.URL.Query()
		if f.searchRows != nil {
			respond(f.searchRows)
		} else {
			rows := []map[string]any{}
			for _, row := range f.rows {
				if row["status"] == true && row["forget_at"] == nil {
					rows = append(rows, row)
				}
			}
			respond(rows)
		}
	case r.URL.Path == "/api/v1/messages" && r.Method == http.MethodPost:
		f.posts++
		var body map[string]string
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(f.t, map[string]string{"memory_id": testBank, "user_id": scope.user, "agent_id": scope.agent, "session_id": scope.session, "user_input": body["user_input"], "agent_response": ""}, body)
		if !f.skipPost {
			row := f.row(strconv.Itoa(100+f.posts), "new quote")
			row["content"] = "User Input: " + body["user_input"] + "\nAgent Response: "
			f.rows = append(f.rows, row)
		}
		if f.lostResponse {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(f.t, err)
			_ = conn.Close()
			return
		}
		if f.postError {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "data": false, "message": "private original quote"})
			return
		}
		respond(nil)
	case strings.HasPrefix(r.URL.Path, "/api/v1/messages/"+testBank+":"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/messages/"+testBank+":")
		content := strings.HasSuffix(id, "/content")
		id = strings.TrimSuffix(id, "/content")
		for _, row := range f.rows {
			if fmt.Sprint(row["message_id"]) != id {
				continue
			}
			if content && r.Method == http.MethodGet {
				f.details++
				respond(row)
				return
			}
			if !content && r.Method == http.MethodPut {
				f.puts++
				var body map[string]bool
				require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(f.t, map[string]bool{"status": false}, body)
				if f.putError {
					_ = json.NewEncoder(w).Encode(map[string]any{"retcode": 500, "data": true})
					return
				}
				if !f.skipDisable {
					row["status"] = false
				}
				if f.lostPutResponse {
					conn, _, err := w.(http.Hijacker).Hijack()
					require.NoError(f.t, err)
					_ = conn.Close()
					return
				}
				respond(true)
				return
			}
		}
		w.WriteHeader(404)
	default:
		f.t.Errorf("unexpected native endpoint %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}
}

func TestLogicalScopeNoDocumentCreation(t *testing.T) {
	f := newFixture(t)
	for _, allow := range []bool{false, true} {
		token, err := f.adapter.EnsureDocument(context.Background(), testBank, testToken, allow)
		require.NoError(t, err)
		require.Equal(t, testToken, token)
	}
	require.Zero(t, f.posts)
	err := f.adapter.DropDocument(context.Background(), testBank, testToken)
	require.ErrorIs(t, err, memory.ErrUnsupported)
	require.Zero(t, f.deletes)
	f.configTypes = []string{"raw", "semantic"}
	_, err = f.adapter.EnsureDocument(context.Background(), testBank, testToken, true)
	require.NoError(t, err)
	f.configTypes = []string{"raw", "invented"}
	_, err = f.adapter.EnsureDocument(context.Background(), testBank, testToken, true)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
}

func TestScopeAndConstructorRejectBeforeIO(t *testing.T) {
	for _, cfg := range []Config{{}, {BaseURL: "https://user:pass@example.test", APIKey: "key", BankID: testBank}, {BaseURL: "https://example.test?query=1", APIKey: "key", BankID: testBank}, {BaseURL: "https://example.test", APIKey: "key\nsecret", BankID: testBank}} {
		_, err := New(cfg)
		require.ErrorIs(t, err, memory.ErrInvalid)
	}
	f := newFixture(t)
	for _, token := range []string{"", "memory-", "memory-" + strings.Repeat("g", 64), testToken + `" OR true`} {
		_, err := f.adapter.Find(context.Background(), testBank, token, "quote")
		require.ErrorIs(t, err, memory.ErrInvalid)
	}
	_, err := f.adapter.Search(context.Background(), "other_bank", testToken, "query", 5)
	require.ErrorIs(t, err, memory.ErrInvalid)
	_, err = f.adapter.Search(context.Background(), testBank, testToken, "", 5)
	require.ErrorIs(t, err, memory.ErrInvalid)
	require.Zero(t, f.lists)
}

func TestQuoteEnvelopeStrictlyReversible(t *testing.T) {
	for _, quote := range []string{"用户原话🙂\nAgent Response: 引号\" <tag> & \\", strings.Repeat("🙂", 200)} {
		payload, err := encodeQuote(quote)
		require.NoError(t, err)
		decoded, err := decodeQuote("User Input: " + payload + "\nAgent Response: ")
		require.NoError(t, err)
		require.Equal(t, quote, decoded)
	}
	for _, raw := range []string{`User Input: {"schema":"hub-memory-fact-v1","quote":"x","extra":true}` + "\nAgent Response: ", `User Input: {"schema":"hub-memory-fact-v1","quote":"old","quote":"x"}` + "\nAgent Response: ", "User Input: noncanonical\nAgent Response: ", "ordinary quote"} {
		_, err := decodeQuote(raw)
		require.ErrorIs(t, err, memory.ErrIndexUnknown)
	}
	_, err := encodeQuote(strings.Repeat("🙂", 201))
	require.ErrorIs(t, err, memory.ErrInvalid)
}

func TestPutReadbackAndExactDedup(t *testing.T) {
	f := newFixture(t)
	quote := "已审核的原话🙂"
	id, err := f.adapter.Put(context.Background(), testBank, testToken, quote)
	require.NoError(t, err)
	require.Equal(t, "101", id)
	id, err = f.adapter.Put(context.Background(), testBank, testToken, quote)
	require.NoError(t, err)
	require.Equal(t, "101", id)
	require.Equal(t, 1, f.posts)
	require.GreaterOrEqual(t, f.details, 4)
}

func TestCompletePaginationGlobalAndMilvusPageCounts(t *testing.T) {
	for _, pageTotal := range []bool{false, true} {
		t.Run(fmt.Sprint(pageTotal), func(t *testing.T) {
			f := newFixture(t)
			f.pageTotal = pageTotal
			f.rows = []map[string]any{f.row("1", "first"), f.row("2", "second"), f.row("3", "last")}
			id, err := f.adapter.Find(context.Background(), testBank, testToken, "last")
			require.NoError(t, err)
			require.Equal(t, "3", id)
			require.Equal(t, 6, f.lists)
			require.Equal(t, 6, f.details)
		})
	}
}

func TestIncompleteChangingOrDuplicatePaginationRejected(t *testing.T) {
	for _, kind := range []string{"duplicate", "total-change", "oversize-total", "never-empty", "changing-content"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.rows = []map[string]any{f.row("1", "quote")}
			f.listOverride = func(page, size int) ([]map[string]any, int) {
				switch kind {
				case "duplicate":
					return []map[string]any{f.rows[0]}, 1
				case "total-change":
					if page == 1 {
						return f.rows, 2
					}
					return []map[string]any{}, 3
				case "oversize-total":
					return f.rows, 100000
				case "never-empty":
					row := f.row(strconv.Itoa(page), "quote")
					f.rows = append(f.rows, row)
					return []map[string]any{row}, 1
				case "changing-content":
					if f.lists > 2 {
						f.rows[0] = f.row("1", "changed")
					}
					if page == 1 {
						return f.rows, 1
					}
					return []map[string]any{}, 1
				}
				return nil, 0
			}
			_, err := f.adapter.Find(context.Background(), testBank, testToken, "quote")
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
		})
	}
}

func TestUnknownPostNeverBlindlyReplayed(t *testing.T) {
	for _, persist := range []bool{false, true} {
		t.Run(fmt.Sprint(persist), func(t *testing.T) {
			f := newFixture(t)
			f.lostResponse = true
			f.skipPost = !persist
			_, err := f.adapter.Put(context.Background(), testBank, testToken, "private original quote")
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
			require.NotContains(t, err.Error(), "private original quote")
			id, err := f.adapter.Find(context.Background(), testBank, testToken, "private original quote")
			require.NoError(t, err)
			if persist {
				require.Equal(t, "101", id)
			} else {
				require.Empty(t, id)
			}
			_, err = f.adapter.Put(context.Background(), testBank, testToken, "private original quote")
			if persist {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, memory.ErrIndexUnknown)
			}
			require.Equal(t, 1, f.posts)
		})
	}
}

func TestBusinessFailureAndAcceptedButInvisibleWriteAreUnknown(t *testing.T) {
	for _, business := range []bool{false, true} {
		t.Run(fmt.Sprint(business), func(t *testing.T) {
			f := newFixture(t)
			f.skipPost = true
			f.postError = business
			_, err := f.adapter.Put(context.Background(), testBank, testToken, "private original quote")
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
			require.NotContains(t, err.Error(), "private original quote")
			_, err = f.adapter.Put(context.Background(), testBank, testToken, "private original quote")
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
			require.Equal(t, 1, f.posts)
		})
	}
}

func TestFindRejectsInactiveForgottenCrossScopeOrNoncanonicalRows(t *testing.T) {
	for _, field := range []string{"disabled", "forgotten", "missing-status", "missing-forget", "bank", "user", "agent", "session", "type", "source", "alias", "fractional-id", "content"} {
		t.Run(field, func(t *testing.T) {
			f := newFixture(t)
			row := f.row("1", "quote")
			switch field {
			case "disabled":
				row["status"] = false
			case "forgotten":
				row["forget_at"] = "2026-11-06 11:00:00"
			case "missing-status":
				delete(row, "status")
			case "missing-forget":
				delete(row, "forget_at")
			case "bank":
				row["memory_id"] = "other_bank"
			case "user":
				row["user_id"] = "other_user"
			case "agent":
				row["agent_id"] = "other_agent"
			case "session":
				row["session_id"] = "other_session"
			case "type":
				row["message_type"] = "semantic"
			case "source":
				row["source_id"] = 99
			case "alias":
				row["id"] = "other_id"
			case "fractional-id":
				row["message_id"] = 1.5
			case "content":
				row["content"] = "not a canonical approved fact"
			}
			f.rows = []map[string]any{row}
			_, err := f.adapter.Find(context.Background(), testBank, testToken, "quote")
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
			require.Zero(t, f.posts)
		})
	}
}

func TestRemoveOnlyConfirmsStatusFalseReadback(t *testing.T) {
	f := newFixture(t)
	f.rows = []map[string]any{f.row("1", "quote")}
	require.NoError(t, f.adapter.Remove(context.Background(), testBank, testToken, "1"))
	require.Equal(t, false, f.rows[0]["status"])
	require.Nil(t, f.rows[0]["forget_at"])
	require.NoError(t, f.adapter.Remove(context.Background(), testBank, testToken, "1"))
	require.Equal(t, 1, f.puts)
	require.Zero(t, f.deletes)
	_, err := f.adapter.Find(context.Background(), testBank, testToken, "quote")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
}

func TestRemoveBusinessFailureAndUnchangedReadbackRejected(t *testing.T) {
	for _, business := range []bool{false, true} {
		t.Run(fmt.Sprint(business), func(t *testing.T) {
			f := newFixture(t)
			f.rows = []map[string]any{f.row("1", "quote")}
			f.skipDisable = true
			f.putError = business
			require.ErrorIs(t, f.adapter.Remove(context.Background(), testBank, testToken, "1"), memory.ErrIndexUnknown)
			require.Equal(t, true, f.rows[0]["status"])
			require.Zero(t, f.deletes)
		})
	}
}

func TestRemoveLostResponseRecoveredOnlyByReadback(t *testing.T) {
	f := newFixture(t)
	f.rows = []map[string]any{f.row("1", "quote")}
	f.lostPutResponse = true
	require.NoError(t, f.adapter.Remove(context.Background(), testBank, testToken, "1"))
	require.Equal(t, 1, f.puts)
	require.Equal(t, 2, f.details)
	require.Zero(t, f.deletes)
}

func TestSearchThreeScopesAndExactQuoteReadback(t *testing.T) {
	f := newFixture(t)
	f.rows = []map[string]any{f.row("1", "quoted fact")}
	hits, err := f.adapter.Search(context.Background(), testBank, testToken, "question", 5)
	require.NoError(t, err)
	require.Equal(t, []memory.Hit{{ChunkID: "1", DocumentID: testToken, Content: "quoted fact", Score: 0}}, hits)
	scope, _ := f.adapter.scope(testBank, testToken)
	require.Equal(t, url.Values{"memory_id": {testBank}, "user_id": {scope.user}, "agent_id": {scope.agent}, "session_id": {scope.session}, "query": {"question"}, "top_n": {"5"}, "similarity_threshold": {"0.2"}, "keywords_similarity_weight": {"0.7"}}, f.searchQuery)
}

func TestSearchOutOfScopeDisabledOrStaleContentRejected(t *testing.T) {
	for _, kind := range []string{"user", "bank", "disabled", "forgotten", "body", "duplicate", "type"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.rows = []map[string]any{f.row("1", "quoted fact")}
			row := copyRow(f.rows[0])
			f.searchRows = []map[string]any{row}
			switch kind {
			case "user":
				row["user_id"] = "other"
			case "bank":
				row["memory_id"] = "other"
			case "disabled":
				row["status"] = false
			case "forgotten":
				row["forget_at"] = "2026-11-06"
			case "body":
				row["content"] = f.row("1", "different fact")["content"]
			case "duplicate":
				f.searchRows = append(f.searchRows, row)
			case "type":
				row["message_type"] = "semantic"
			}
			_, err := f.adapter.Search(context.Background(), testBank, testToken, "question", 5)
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
		})
	}
}

func TestMessageIDDoesNotLoseIntegerPrecision(t *testing.T) {
	for _, raw := range []string{"9007199254740993", `"9007199254740993"`} {
		id, ok := messageID(json.RawMessage(raw), false)
		require.True(t, ok)
		require.Equal(t, "9007199254740993", id)
	}
	for _, raw := range []string{"", "1.0", "1e3", "-1", "0", "true", `"01"`, "9223372036854775808"} {
		_, ok := messageID(json.RawMessage(raw), false)
		require.False(t, ok)
	}
}
