package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/tenant"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type agentRelationHTTPEnvelope struct {
	Success bool                        `json:"success"`
	Data    []services.AgentRelationDTO `json:"data"`
	Error   string                      `json:"error"`
}

func setupAgentRelationHTTPTest(t *testing.T) (*gin.Engine, agent.AgentConfig, agent.AgentConfig) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}, &agentrelation.AgentRelationEvent{}))

	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })

	source := agent.AgentConfig{Name: "chief-of-staff", TenantID: "org-a"}
	target := agent.AgentConfig{Name: "legal-counsel", TenantID: "org-a"}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&target).Error)

	h := NewAgentRelationHandler(services.NewAgentRelationService())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "org-a")
		c.Next()
	})
	router.GET("/api/v1/admin/agent-relations", h.List)
	router.GET("/api/v1/admin/agent-relations/:id/events", h.ListEvents)
	router.POST("/api/v1/admin/agent-relations", h.Create)
	router.POST("/api/v1/admin/agent-relations/:id/events", h.RecordEvent)
	router.PATCH("/api/v1/admin/agent-relations/:id", h.Update)
	router.DELETE("/api/v1/admin/agent-relations/:id", h.Delete)
	return router, source, target
}

func TestAgentRelationHTTPRecordsAndListsDynamicEvents(t *testing.T) {
	router, source, target := setupAgentRelationHTTPTest(t)
	createBody := fmt.Sprintf(`{
		"sourceAgentId": %d,
		"targetAgentId": %d,
		"scope": "speeding-hq",
		"relationType": "peer",
		"stance": "friendly",
		"allowedActions": ["inform"]
	}`, source.ID, target.ID)
	createdResponse := agentRelationHTTPRequest(t, router, http.MethodPost, "/api/v1/admin/agent-relations", createBody)
	require.Equal(t, http.StatusCreated, createdResponse.Code)
	var created agentRelationHTTPEnvelope
	require.NoError(t, json.Unmarshal(createdResponse.Body.Bytes(), &created))
	require.Len(t, created.Data, 1)

	eventPath := fmt.Sprintf("/api/v1/admin/agent-relations/%d/events", created.Data[0].ID)
	recordedResponse := agentRelationHTTPRequest(t, router, http.MethodPost, eventPath, `{
		"eventType": "promise_broken",
		"severity": 1,
		"reason": "答应提供证据但没有交付",
		"visibility": "participants",
		"idempotencyKey": "meeting-7-promise"
	}`)
	require.Equal(t, http.StatusCreated, recordedResponse.Code)
	var recorded struct {
		Success bool `json:"success"`
		Data    struct {
			Relation services.AgentRelationDTO      `json:"relation"`
			Event    services.AgentRelationEventDTO `json:"event"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recordedResponse.Body.Bytes(), &recorded))
	require.True(t, recorded.Success)
	require.Equal(t, 20, recorded.Data.Relation.RelationshipScore)
	require.Equal(t, "neutral", recorded.Data.Relation.Stance)
	require.Equal(t, -20, recorded.Data.Event.Delta)

	listedResponse := agentRelationHTTPRequest(t, router, http.MethodGet, eventPath, "")
	require.Equal(t, http.StatusOK, listedResponse.Code)
	var listed struct {
		Success bool                             `json:"success"`
		Data    []services.AgentRelationEventDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listedResponse.Body.Bytes(), &listed))
	require.Len(t, listed.Data, 1)
	require.Equal(t, "meeting-7-promise", listed.Data[0].IdempotencyKey)
}

func agentRelationHTTPRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestAgentRelationHTTPCRUDAndBidirectionalContract(t *testing.T) {
	router, source, target := setupAgentRelationHTTPTest(t)
	createBody := fmt.Sprintf(`{
		"sourceAgentId": %d,
		"targetAgentId": %d,
		"scope": "speeding-hq",
		"relationType": "peer",
		"stance": "friendly",
		"allowedActions": ["inform", "consult", "handoff"],
		"contextPolicy": "shared_thread",
		"deliveryPolicy": "async",
		"bidirectional": true
	}`, source.ID, target.ID)

	createResponse := agentRelationHTTPRequest(t, router, http.MethodPost, "/api/v1/admin/agent-relations", createBody)
	require.Equal(t, http.StatusCreated, createResponse.Code)
	var created agentRelationHTTPEnvelope
	require.NoError(t, json.Unmarshal(createResponse.Body.Bytes(), &created))
	require.True(t, created.Success)
	require.Len(t, created.Data, 2)
	require.True(t, created.Data[0].Enabled, "omitting enabled must default the relation to active")
	require.Equal(t, source.Name, created.Data[0].SourceAgentName)
	require.Equal(t, target.Name, created.Data[1].SourceAgentName)

	listResponse := agentRelationHTTPRequest(t, router, http.MethodGet, "/api/v1/admin/agent-relations", "")
	require.Equal(t, http.StatusOK, listResponse.Code)
	var listed agentRelationHTTPEnvelope
	require.NoError(t, json.Unmarshal(listResponse.Body.Bytes(), &listed))
	require.Len(t, listed.Data, 2)

	updateResponse := agentRelationHTTPRequest(t, router, http.MethodPatch,
		fmt.Sprintf("/api/v1/admin/agent-relations/%d", created.Data[0].ID),
		`{"stance":"competitive","allowedActions":["review","challenge"]}`)
	require.Equal(t, http.StatusOK, updateResponse.Code)
	var updated struct {
		Success bool                      `json:"success"`
		Data    services.AgentRelationDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(updateResponse.Body.Bytes(), &updated))
	require.Equal(t, "competitive", updated.Data.Stance)
	require.Equal(t, []string{"review", "challenge"}, updated.Data.AllowedActions)

	deleteResponse := agentRelationHTTPRequest(t, router, http.MethodDelete,
		fmt.Sprintf("/api/v1/admin/agent-relations/%d", created.Data[0].ID), "")
	require.Equal(t, http.StatusOK, deleteResponse.Code)

	finalListResponse := agentRelationHTTPRequest(t, router, http.MethodGet, "/api/v1/admin/agent-relations", "")
	require.Equal(t, http.StatusOK, finalListResponse.Code)
	var finalList agentRelationHTTPEnvelope
	require.NoError(t, json.Unmarshal(finalListResponse.Body.Bytes(), &finalList))
	require.Len(t, finalList.Data, 1, "deleting A -> B must leave B -> A intact")
	require.Equal(t, target.ID, finalList.Data[0].SourceAgentID)
	require.Equal(t, source.ID, finalList.Data[0].TargetAgentID)
}

func TestAgentRelationHTTPReturnsConflictForDuplicateEdge(t *testing.T) {
	router, source, target := setupAgentRelationHTTPTest(t)
	body := fmt.Sprintf(`{
		"sourceAgentId": %d,
		"targetAgentId": %d,
		"scope": "speeding-hq",
		"relationType": "advisor",
		"allowedActions": ["consult"]
	}`, source.ID, target.ID)

	first := agentRelationHTTPRequest(t, router, http.MethodPost, "/api/v1/admin/agent-relations", body)
	require.Equal(t, http.StatusCreated, first.Code)
	duplicate := agentRelationHTTPRequest(t, router, http.MethodPost, "/api/v1/admin/agent-relations", body)
	require.Equal(t, http.StatusConflict, duplicate.Code)

	var response agentRelationHTTPEnvelope
	require.NoError(t, json.Unmarshal(duplicate.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.NotEmpty(t, response.Error)
}

func TestAgentRelationHTTPRejectsSelfRelation(t *testing.T) {
	router, source, _ := setupAgentRelationHTTPTest(t)
	body := fmt.Sprintf(`{
		"sourceAgentId": %d,
		"targetAgentId": %d,
		"relationType": "peer",
		"allowedActions": ["inform"]
	}`, source.ID, source.ID)

	response := agentRelationHTTPRequest(t, router, http.MethodPost, "/api/v1/admin/agent-relations", body)
	require.Equal(t, http.StatusBadRequest, response.Code)
}
