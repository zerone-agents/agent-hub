package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPersonaAdminRunService(t *testing.T) (*services.RunService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}))
	started := time.Now().UTC()
	require.NoError(t, db.Create(&rundomain.Run{ID: "run-1", TenantID: "t1", Name: "case", Status: rundomain.StatusRunning, StartedAt: &started}).Error)
	return services.NewRunService(db), db
}

func setupPersonaAdmin(t *testing.T) (*PersonaAdminHandler, *services.BeliefService) {
	t.Helper()
	runService, _ := newPersonaAdminRunService(t)
	belief := services.NewBeliefService(runService)
	// Deliver the same fact to two agents, then contradict one of them.
	now := time.Now().UTC()
	require.NoError(t, belief.RecordDelivery("t1", "run-1", 11, "fact:scandal", now, "d-11"))
	require.NoError(t, belief.RecordDelivery("t1", "run-1", 22, "fact:scandal", now, "d-22"))
	require.NoError(t, belief.AddEvidence("t1", "run-1", 22, "fact:scandal", true, now, "c-22"))
	return NewPersonaAdminHandler(runService, belief), belief
}

func personaAdminContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", path, nil)
	tenant.SetTenantID(c, "t1")
	return c, rec
}

func TestPersonaAdminStateGroupsByPack(t *testing.T) {
	h, _ := setupPersonaAdmin(t)
	c, rec := personaAdminContext(t, "/api/v1/admin/runs/run-1/persona-state")
	c.Params = gin.Params{{Key: "runId", Value: "run-1"}}
	h.PersonaState(c)
	require.Equal(t, 200, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			RunID  string `json:"runId"`
			Belief []struct {
				Namespace string         `json:"namespace"`
				SubjectID string         `json:"subjectId"`
				Data      map[string]any `json:"data"`
			} `json:"belief"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, "run-1", body.Data.RunID)
	require.Len(t, body.Data.Belief, 2)
	for _, entry := range body.Data.Belief {
		require.Equal(t, services.BeliefNamespace, entry.Namespace)
		require.True(t, strings.HasPrefix(entry.SubjectID, "11/") || strings.HasPrefix(entry.SubjectID, "22/"))
	}
}

func TestPersonaAdminBeliefDisputes(t *testing.T) {
	h, _ := setupPersonaAdmin(t)
	c, rec := personaAdminContext(t, "/api/v1/admin/runs/run-1/belief-disputes")
	c.Params = gin.Params{{Key: "runId", Value: "run-1"}}
	h.BeliefDisputes(c)
	require.Equal(t, 200, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			FactRef string `json:"factRef"`
			Entries []struct {
				AgentID uint64 `json:"agentId"`
				Status  string `json:"status"`
			} `json:"entries"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data, 1)
	require.Equal(t, "fact:scandal", body.Data[0].FactRef)
	require.Len(t, body.Data[0].Entries, 2)
}

func TestPersonaAdminBeliefDisputesDisabledWithoutService(t *testing.T) {
	runService, _ := newPersonaAdminRunService(t)
	h := NewPersonaAdminHandler(runService, nil)
	c, rec := personaAdminContext(t, "/api/v1/admin/runs/run-1/belief-disputes")
	c.Params = gin.Params{{Key: "runId", Value: "run-1"}}
	h.BeliefDisputes(c)
	require.Equal(t, 501, rec.Code)
	require.Contains(t, rec.Body.String(), "能力尚未启用")
}
