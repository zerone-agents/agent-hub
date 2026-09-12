package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeRunAgentMessageReader struct {
	tenantID, runID string
	limit           int
	rows            []*services.AgentMessageDTO
	err             error
}

func (f *fakeRunAgentMessageReader) ListRun(tenantID, runID string, limit int) ([]*services.AgentMessageDTO, error) {
	f.tenantID, f.runID, f.limit = tenantID, runID, limit
	return f.rows, f.err
}

func TestAgentMessageAdminListsAuditableChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeRunAgentMessageReader{rows: []*services.AgentMessageDTO{{
		ID: "m2", SourceAgent: "agent-b", TargetAgent: "agent-c", Action: "handoff", Status: "queued",
		ConversationID: "conv-1", RootMessageID: "m1", ParentMessageID: "m1", Hop: 2, MaxHops: 8,
		EventBudget: 64, EventCount: 2, TokenBudget: 65536, TokensUsed: 120,
	}}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	router.GET("/runs/:id/agent-messages", NewAgentMessageAdminHandler(fake).ListRun)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/run-1/agent-messages?limit=25", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "tenant-a", fake.tenantID)
	require.Equal(t, "run-1", fake.runID)
	require.Equal(t, 25, fake.limit)
	require.Contains(t, w.Body.String(), `"parentMessageId":"m1"`)
	require.Contains(t, w.Body.String(), `"eventBudget":64`)
}

func TestAgentMessageAdminReportsServiceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	router.GET("/runs/:id/agent-messages", NewAgentMessageAdminHandler(&fakeRunAgentMessageReader{err: errors.New("database unavailable")}).ListRun)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/run-1/agent-messages", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}
