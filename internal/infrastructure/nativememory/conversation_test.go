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

type conversationFixture struct {
	t                       *testing.T
	mu                      sync.Mutex
	adapter                 *Adapter
	rows                    []map[string]any
	posts                   int
	puts                    int
	requests                int
	postBodies              []map[string]string
	putIDs                  []string
	progress                *float64
	lostResponse, skipWrite bool
	lostPutResponse         bool
	skipStatusWrite         bool
	mutateAfterPut          func(map[string]any)
	configOverride          map[string]any
	searchRows              []map[string]any
	searchQuery             url.Values
}

func conversationTestFixture(t *testing.T) *conversationFixture {
	f := &conversationFixture{t: t}
	s := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(s.Close)
	var err error
	f.adapter, err = New(Config{BaseURL: s.URL, APIKey: "contract-test-key", BankID: testBank, PageSize: 2, MaxPages: 5})
	require.NoError(t, err)
	return f
}

func (f *conversationFixture) row(id, source, kind, session, content string) map[string]any {
	scope, err := f.adapter.scope(testBank, testToken)
	require.NoError(f.t, err)
	return map[string]any{"id": testBank + "_" + id, "message_id": json.Number(id), "source_id": json.Number(source), "message_type": kind, "memory_id": testBank, "user_id": scope.user, "agent_id": scope.agent, "session_id": session, "status": true, "forget_at": nil, "valid_at": "2026-11-06 10:00:00", "invalid_at": nil, "content": content}
}

func (f *conversationFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	require.Equal(f.t, "Bearer contract-test-key", r.Header.Get("Authorization"))
	respond := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}) }
	scope, _ := f.adapter.scope(testBank, testToken)
	switch {
	case r.URL.Path == "/api/v1/memories/"+testBank+"/config":
		if f.configOverride != nil {
			respond(f.configOverride)
			return
		}
		respond(map[string]any{"id": testBank, "tenant_id": "native_owner", "permissions": "me", "memory_type": []string{"raw", "semantic", "episodic", "procedural"}})
	case r.URL.Path == "/api/v1/memories/"+testBank:
		require.Equal(f.t, scope.agent, r.URL.Query().Get("agent_id"))
		raws := []map[string]any{}
		for _, row := range f.rows {
			if row["message_type"] == "raw" && (r.URL.Query().Get("keywords") == "" || row["session_id"] == r.URL.Query().Get("keywords")) {
				raws = append(raws, row)
			}
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		listed := []map[string]any{}
		for i := (page - 1) * size; i < len(raws) && i < page*size; i++ {
			row := copyRow(raws[i])
			delete(row, "content")
			children := []map[string]any{}
			for _, child := range f.rows {
				if fmt.Sprint(child["source_id"]) == fmt.Sprint(row["message_id"]) {
					children = append(children, copyRow(child))
				}
			}
			row["extract"] = children
			row["task"] = map[string]any{}
			if f.progress != nil {
				row["task"] = map[string]any{"progress": *f.progress}
			}
			listed = append(listed, row)
		}
		respond(map[string]any{"messages": map[string]any{"message_list": listed, "total_count": len(listed)}})
	case r.URL.Path == "/api/v1/messages" && r.Method == http.MethodPost:
		f.posts++
		var body map[string]string
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		f.postBodies = append(f.postBodies, body)
		require.Equal(f.t, scope.user, body["user_id"])
		require.Equal(f.t, scope.agent, body["agent_id"])
		if !f.skipWrite {
			f.rows = append(f.rows, f.row(strconv.FormatInt(9007199254740992+int64(f.posts), 10), "0", "raw", body["session_id"], "User Input: "+body["user_input"]+"\nAgent Response: "+body["agent_response"]))
		}
		if f.lostResponse {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(f.t, err)
			_ = conn.Close()
			return
		}
		respond(nil)
	case r.URL.Path == "/api/v1/messages/search":
		f.searchQuery = r.URL.Query()
		require.Equal(f.t, testBank, r.URL.Query().Get("memory_id"))
		require.Equal(f.t, scope.user, r.URL.Query().Get("user_id"))
		require.Equal(f.t, scope.agent, r.URL.Query().Get("agent_id"))
		require.Empty(f.t, r.URL.Query().Get("session_id"))
		rows := f.searchRows
		if rows == nil {
			rows = f.rows
		}
		respond(rows)
	case strings.HasPrefix(r.URL.Path, "/api/v1/messages/"+testBank+":") && strings.HasSuffix(r.URL.Path, "/content"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/messages/"+testBank+":"), "/content")
		for _, row := range f.rows {
			if fmt.Sprint(row["message_id"]) == id {
				respond(row)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	case strings.HasPrefix(r.URL.Path, "/api/v1/messages/"+testBank+":") && r.Method == http.MethodPut:
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/messages/"+testBank+":")
		var body map[string]bool
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(f.t, body, 1)
		_, present := body["status"]
		require.True(f.t, present)
		for _, row := range f.rows {
			if fmt.Sprint(row["message_id"]) != id {
				continue
			}
			f.puts++
			f.putIDs = append(f.putIDs, id)
			if !f.skipStatusWrite {
				row["status"] = body["status"]
			}
			if f.mutateAfterPut != nil {
				f.mutateAfterPut(row)
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
		w.WriteHeader(http.StatusNotFound)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(405)
	}
}

func TestConversationUsesOriginalTurnAndOperationSession(t *testing.T) {
	f := conversationTestFixture(t)
	turn := memory.NativeTurn{OperationID: "op-1", SessionID: "runtime-session", TurnID: "turn-1", UserInput: "我喜欢普洱茶。", AgentResponse: "好的，聊聊你喜欢的茶。"}
	first, err := f.adapter.AddConversation(context.Background(), testToken, turn)
	require.NoError(t, err)
	require.Equal(t, "9007199254740993", first.SourceID)
	require.Equal(t, "unknown", first.Status)
	require.Equal(t, turn.UserInput, f.postBodies[0]["user_input"])
	require.Equal(t, turn.AgentResponse, f.postBodies[0]["agent_response"])
	scope, _ := f.adapter.scope(testBank, testToken)
	require.Equal(t, conversationSession(scope, turn.OperationID), first.SourceSessionID)
	_, err = f.adapter.AddConversation(context.Background(), testToken, turn)
	require.NoError(t, err)
	require.Equal(t, 1, f.posts)
	turn.UserInput = "同键换内容"
	_, err = f.adapter.AddConversation(context.Background(), testToken, turn)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	require.Equal(t, 1, f.posts)
	turn.OperationID = "op-2"
	second, err := f.adapter.AddConversation(context.Background(), testToken, turn)
	require.NoError(t, err)
	require.NotEqual(t, first.SourceSessionID, second.SourceSessionID)
	require.Equal(t, 2, f.posts)
}

func TestConversationLostResponseReadRecoveryNoReplay(t *testing.T) {
	f := conversationTestFixture(t)
	f.lostResponse = true
	turn := memory.NativeTurn{OperationID: "lost", UserInput: "真实输入", AgentResponse: "真实回复"}
	result, err := f.adapter.AddConversation(context.Background(), testToken, turn)
	require.NoError(t, err)
	require.NotEmpty(t, result.SourceID)
	_, err = f.adapter.FindConversation(context.Background(), testToken, turn)
	require.NoError(t, err)
	require.Equal(t, 1, f.posts)
	f.skipWrite = true
	turn.OperationID = "uncertain"
	_, err = f.adapter.AddConversation(context.Background(), testToken, turn)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	_, err = f.adapter.AddConversation(context.Background(), testToken, turn)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	require.Equal(t, 2, f.posts)
	result, err = f.adapter.FindConversation(context.Background(), testToken, turn)
	require.NoError(t, err)
	require.Equal(t, "unknown", result.Status)
	require.Empty(t, result.SourceID)
	require.Equal(t, 2, f.posts)
}

func TestAllNativeTypesKeepProvenanceAndExtractionState(t *testing.T) {
	f := conversationTestFixture(t)
	scope, _ := f.adapter.scope(testBank, testToken)
	session := conversationSession(scope, "typed")
	f.rows = append(f.rows, f.row("9007199254740993", "0", "raw", session, "User Input: 原始对话\nAgent Response: 实际回复"))
	for i, kind := range []string{"semantic", "episodic", "procedural"} {
		f.rows = append(f.rows, f.row(strconv.Itoa(200+i), "9007199254740993", kind, session, kind+" generated content"))
	}
	zero := 0.0
	f.progress = &zero
	rows, err := f.adapter.ListMemories(context.Background(), testToken)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	require.Equal(t, "derivedAvailable", rows[0].ExtractionStatus)
	for _, row := range rows[1:] {
		require.Equal(t, "9007199254740993", row.SourceID)
		require.Equal(t, session, row.SourceSessionID)
		require.Contains(t, []string{"semantic", "episodic", "procedural"}, row.Type)
	}
	rows, err = f.adapter.SearchMemories(context.Background(), testToken, "相关问题", 5)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	done := 1.0
	f.progress = &done
	rows, err = f.adapter.ListMemories(context.Background(), testToken)
	require.NoError(t, err)
	require.Equal(t, "completed", rows[0].ExtractionStatus)
	f.rows = f.rows[:1]
	f.progress = &zero
	rows, err = f.adapter.ListMemories(context.Background(), testToken)
	require.NoError(t, err)
	require.Equal(t, "received", rows[0].ExtractionStatus)
}

func TestTypedSearchRejectsScopeForgeryAndPreservesEventTimeSemantics(t *testing.T) {
	for _, mutation := range []string{"foreign-user", "foreign-session", "wrong-source-session", "expired", "future", "invalid-time", "disabled-parent"} {
		t.Run(mutation, func(t *testing.T) {
			f := conversationTestFixture(t)
			scope, _ := f.adapter.scope(testBank, testToken)
			session := conversationSession(scope, "typed")
			parent := f.row("100", "0", "raw", session, "original")
			child := f.row("101", "100", "semantic", session, "derived")
			f.rows = []map[string]any{parent, child}
			f.searchRows = []map[string]any{child}
			switch mutation {
			case "foreign-user":
				child["user_id"] = "other"
			case "foreign-session":
				child["session_id"] = "other"
			case "wrong-source-session":
				parent["session_id"] = conversationSession(scope, "different")
			case "expired":
				child["message_type"] = "episodic"
				child["invalid_at"] = "2000-01-01 00:00:00"
			case "future":
				child["valid_at"] = "2999-01-01 00:00:00"
			case "invalid-time":
				child["valid_at"] = "not a date"
			case "disabled-parent":
				parent["status"] = false
			}
			rows, err := f.adapter.SearchMemories(context.Background(), testToken, "query", 5)
			if mutation == "disabled-parent" {
				require.NoError(t, err)
				require.Empty(t, rows)
			} else if mutation == "expired" || mutation == "future" || mutation == "invalid-time" {
				require.NoError(t, err)
				require.Len(t, rows, 1)
			} else {
				require.ErrorIs(t, err, memory.ErrIndexUnknown)
			}
		})
	}
}
