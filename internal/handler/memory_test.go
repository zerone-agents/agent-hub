package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/config"
	"control-panel/internal/memory"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func memoryTestRouter(t *testing.T) (*gin.Engine, *memory.Service, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, memory.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("12", 32))
	require.NoError(t, err)
	svc, err := memory.NewService(db, memory.Config{TenantID: "example-tenant", ApplicationID: "memory-app", PolicyVersion: "synthetic-v1", DatasetID: "private", SyntheticOnly: true, Wrapper: wrapper})
	require.NoError(t, err)
	r := gin.New()
	RegisterMemoryRoutes(r, NewMemoryHandler([]MemoryBinding{{Client: config.MemoryClient{Token: strings.Repeat("k", 32), TenantID: "example-tenant", ApplicationID: "memory-app"}, Service: svc}}, nil))
	return r, svc, db
}

func memoryHTTP(t *testing.T, r http.Handler, method, path, user, life string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("k", 32))
	req.Header.Set("X-Trace-Id", "test-trace")
	req.Header.Set("X-Memory-User-Id", user)
	req.Header.Set("X-Memory-Lifecycle-Id", life)
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	require.Equal(t, "no-store", out.Header().Get("Cache-Control"))
	return out
}

func memoryData[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, 200, w.Code, w.Body.String())
	var envelope struct {
		Code    int
		Data    T
		TraceID string `json:"traceId"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, 200, envelope.Code)
	require.Equal(t, "test-trace", envelope.TraceID)
	return envelope.Data
}

func TestMemoryHTTPClosedLoopAndLifecycleIsolation(t *testing.T) {
	r, svc, _ := memoryTestRouter(t)
	sc := memory.Scope{TenantID: "example-tenant", ApplicationID: "memory-app", UserID: "user-a", LifecycleID: "life-a"}
	activate := func(user, life, op string) {
		memoryData[memory.Operation](t, memoryHTTP(t, r, "POST", "/api/v1/memory/lifecycle/activate", user, life, map[string]any{"operationId": op}))
	}
	activate("user-a", "life-a", "activate-a")
	activate("user-b", "life-b", "activate-b")
	in := memory.CandidateInput{OperationID: "candidate-a", Source: "trustedJava", DataClass: "synthetic", TurnID: "turn-a", SourceMessageID: "message-a", UserText: "我朋友以前怕失业，现在已经不担心了。", Quote: "我朋友以前怕失业，现在已经不担心了。", Subject: "asQuoted", Temporal: "asQuoted", Category: "situation", ConsentRef: "synthetic-consent", PolicyVersion: "synthetic-v1"}
	candidate := memoryData[memory.Candidate](t, memoryHTTP(t, r, "POST", "/api/v1/memory/candidates", "user-a", "life-a", in))
	task := memoryData[memory.Task](t, memoryHTTP(t, r, "POST", "/api/v1/memory/tasks/claim", "user-a", "life-a", map[string]any{"operationId": "claim-a"}))
	require.Len(t, task.Candidates, 1)
	facts := memoryData[[]memory.Fact](t, memoryHTTP(t, r, "POST", "/api/v1/memory/tasks/publish", "user-a", "life-a", memory.PublishInput{OperationID: "publish-a", TaskID: task.ID, Items: []memory.PublishItem{{CandidateID: candidate.ID, ExpectedVersion: candidate.Version, Content: in.Quote}}}))
	require.Len(t, facts, 1)
	fact := facts[0]
	require.Equal(t, "asQuoted", fact.Subject)
	require.Equal(t, in.Quote, fact.Content)
	other := memoryHTTP(t, r, "GET", "/api/v1/memory/facts/"+fact.ID, "user-b", "life-b", nil)
	require.Equal(t, 404, other.Code)
	badVersion := memoryHTTP(t, r, "PATCH", "/api/v1/memory/facts/"+fact.ID, "user-a", "life-a", map[string]any{"operationId": "bad-update", "expectedVersion": fact.Version + 5, "content": "合成用户更新。"})
	require.Equal(t, 409, badVersion.Code)
	contextBefore, err := svc.Recall(context.Background(), sc, "工作")
	require.NoError(t, err)
	deleted := memoryData[memory.Operation](t, memoryHTTP(t, r, "DELETE", "/api/v1/memory/facts/"+fact.ID, "user-a", "life-a", map[string]any{"operationId": "delete-a", "expectedVersion": fact.Version}))
	require.True(t, deleted.StoppedSupply)
	require.False(t, deleted.PhysicalCleanup)
	valid := memoryData[map[string]bool](t, memoryHTTP(t, r, "POST", "/api/v1/memory/context/validate", "user-a", "life-a", map[string]any{"contextVersion": contextBefore.Version}))
	require.False(t, valid["valid"])
	deactivated := memoryData[memory.Operation](t, memoryHTTP(t, r, "POST", "/api/v1/memory/lifecycle/deactivate", "user-a", "life-a", map[string]any{"operationId": "deactivate-a"}))
	require.True(t, deactivated.StoppedSupply)
	activate("user-a", "life-new", "activate-new")
	old := memoryHTTP(t, r, "POST", "/api/v1/memory/candidates", "user-a", "life-a", in)
	require.Equal(t, 403, old.Code)
	newFacts := memoryData[[]memory.Fact](t, memoryHTTP(t, r, "GET", "/api/v1/memory/facts", "user-a", "life-new", nil))
	require.Empty(t, newFacts)
}

func TestMemoryHTTPRejectsUntrustedFieldsAndCredentials(t *testing.T) {
	r, _, _ := memoryTestRouter(t)
	unauthorized := httptest.NewRecorder()
	r.ServeHTTP(unauthorized, httptest.NewRequest("GET", "/api/v1/memory/capabilities", nil))
	require.Equal(t, 401, unauthorized.Code)
	for _, body := range []any{
		map[string]any{"operationId": "op", "tenantId": "other"},
		map[string]any{"operationId": "op", "datasetId": "secret"},
		map[string]any{"operationId": "op", "search_id": "saved"},
	} {
		w := memoryHTTP(t, r, "POST", "/api/v1/memory/lifecycle/activate", "user", "life", body)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	w := memoryHTTP(t, r, "POST", "/api/v1/memory/lifecycle/activate", "user", "life", map[string]any{"operationId": "activate"})
	require.Equal(t, 200, w.Code)
	// Error responses carry only stable domain errors, never the supplied text.
	secret := strings.Repeat("private-original-text", 1000)
	w = memoryHTTP(t, r, "POST", "/api/v1/memory/candidates", "user", "life", map[string]any{"operationId": "bad", "source": "model", "quote": secret})
	require.Equal(t, 400, w.Code)
	require.NotContains(t, w.Body.String(), "private-original-text")
}

func TestMemoryHTTPRejectConsumesCandidateAndRetiresSnapshot(t *testing.T) {
	r, _, _ := memoryTestRouter(t)
	memoryData[memory.Operation](t, memoryHTTP(t, r, "POST", "/api/v1/memory/lifecycle/activate", "user", "life", map[string]any{"operationId": "activate"}))
	in := memory.CandidateInput{OperationID: "submit", Source: "trustedJava", DataClass: "synthetic", TurnID: "turn", SourceMessageID: "message", UserText: "合成用户喜欢散步。", Quote: "合成用户喜欢散步。", Subject: "asQuoted", Temporal: "asQuoted", Category: "preference", ConsentRef: "synthetic-consent", PolicyVersion: "synthetic-v1"}
	candidate := memoryData[memory.Candidate](t, memoryHTTP(t, r, "POST", "/api/v1/memory/candidates", "user", "life", in))
	task := memoryData[memory.Task](t, memoryHTTP(t, r, "POST", "/api/v1/memory/tasks/claim", "user", "life", map[string]any{"operationId": "claim"}))
	rejected := memoryData[memory.Candidate](t, memoryHTTP(t, r, "POST", "/api/v1/memory/candidates/"+candidate.ID+"/reject", "user", "life", map[string]any{"operationId": "reject", "expectedVersion": candidate.Version, "reason": "notDurable"}))
	require.Equal(t, "rejected", rejected.Status)
	w := memoryHTTP(t, r, "POST", "/api/v1/memory/tasks/publish", "user", "life", memory.PublishInput{OperationID: "stale-publish", TaskID: task.ID, Items: []memory.PublishItem{{CandidateID: candidate.ID, ExpectedVersion: candidate.Version, Content: in.Quote}}})
	require.Equal(t, 409, w.Code)
	next := memoryData[memory.Task](t, memoryHTTP(t, r, "POST", "/api/v1/memory/tasks/claim", "user", "life", map[string]any{"operationId": "claim-new"}))
	require.Empty(t, next.Candidates)
}
