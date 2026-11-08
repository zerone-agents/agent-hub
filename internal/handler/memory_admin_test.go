package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/config"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memory"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type memoryAdminAuditStore struct{ rows []audit.Log }

func (s *memoryAdminAuditStore) Create(row *audit.Log) error {
	s.rows = append(s.rows, *row)
	return nil
}
func adminMemoryService(t *testing.T, businessTenant, app, dataset string, backends ...string) *memory.Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/admin-memory.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, memory.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("12", 32))
	require.NoError(t, err)
	backend := ""
	if len(backends) > 0 {
		backend = backends[0]
	}
	service, err := memory.NewService(db, memory.Config{Backend: backend, TenantID: businessTenant, ApplicationID: app, PolicyVersion: "synthetic-v1", DatasetID: dataset, SyntheticOnly: true, Wrapper: wrapper})
	require.NoError(t, err)
	return service
}
func memoryAdminFixture(t *testing.T) (*gin.Engine, *memory.Service, *memoryAdminAuditStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := adminMemoryService(t, "example-tenant", "memory-app", "private-dataset")
	foreign := adminMemoryService(t, "other-business", "foreign-app", "foreign-dataset")
	implicit := adminMemoryService(t, "implicit", "default-delegation", "implicit-dataset")
	store := &memoryAdminAuditStore{}
	handler := NewMemoryAdminHandler([]MemoryBinding{
		{Client: config.MemoryClient{Token: "never-return-this-service-secret", TenantID: "example-tenant", ApplicationID: "memory-app", PolicyVersion: "synthetic-v1", DatasetID: "private-dataset", HubTenantID: "default"}, Service: service},
		{Client: config.MemoryClient{TenantID: "other-business", ApplicationID: "foreign-app", PolicyVersion: "synthetic-v1", HubTenantID: "foreign-hub"}, Service: foreign},
		{Client: config.MemoryClient{TenantID: "implicit", ApplicationID: "default-delegation", PolicyVersion: "synthetic-v1"}, Service: implicit},
	}, services.NewAuditRecorder(store))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, c.GetHeader("X-Test-Hub-Tenant"))
		c.Set("roles", []string{c.GetHeader("X-Test-Role")})
		c.Set("user_id", "manager-1")
	})
	read := router.Group("/api/v1/admin")
	write := router.Group("/api/v1/admin")
	RegisterMemoryAdminRoutes(write, read, handler)
	return router, service, store
}
func memoryAdminHTTP(t *testing.T, router http.Handler, method, path, hub, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	request.Header.Set("X-Test-Hub-Tenant", hub)
	request.Header.Set("X-Test-Role", role)
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	return result
}
func memoryAdminData[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, 200, response.Code, response.Body.String())
	var out struct {
		Success bool `json:"success"`
		Data    T    `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &out))
	require.True(t, out.Success)
	return out.Data
}
func seedAdminFact(t *testing.T, service *memory.Service, sc memory.Scope, content string) memory.Fact {
	t.Helper()
	ctx := context.Background()
	_, err := service.Activate(ctx, sc, "activate")
	require.NoError(t, err)
	candidate, err := service.SubmitCandidate(ctx, sc, memory.CandidateInput{DataClass: "synthetic", Source: "trustedJava", OperationID: "candidate", TurnID: "actual-turn", SourceMessageID: "actual-message", UserText: content, Quote: content, Subject: "asQuoted", Temporal: "asQuoted", Category: "preference", ConsentRef: "synthetic-consent", PolicyVersion: "synthetic-v1"})
	require.NoError(t, err)
	task, err := service.ClaimTask(ctx, sc, "claim")
	require.NoError(t, err)
	facts, err := service.Publish(ctx, sc, memory.PublishInput{OperationID: "publish", TaskID: task.ID, Items: []memory.PublishItem{{CandidateID: candidate.ID, ExpectedVersion: 1, Content: content}}})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	return facts[0]
}
func adminScopeQuery(sc memory.Scope) string {
	values := url.Values{"tenantId": {sc.TenantID}, "applicationId": {sc.ApplicationID}, "userId": {sc.UserID}, "accountLifecycleId": {sc.LifecycleID}}
	return values.Encode()
}
func TestMemoryAdminDelegationAndManagerReadGate(t *testing.T) {
	router, service, _ := memoryAdminFixture(t)
	sc := memory.Scope{TenantID: "example-tenant", ApplicationID: "memory-app", UserID: "business-alice", LifecycleID: "life-a"}
	seedAdminFact(t, service, sc, "我喜欢散步。")
	response := memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/applications", "default", "admin", nil)
	apps := memoryAdminData[[]memoryAdminApplication](t, response)
	require.Len(t, apps, 1)
	require.Equal(t, "example-tenant", apps[0].TenantID)
	require.Equal(t, "synthetic", apps[0].DataClass)
	require.NotContains(t, response.Body.String(), "private-dataset")
	require.NotContains(t, response.Body.String(), "service-secret")
	require.NotContains(t, response.Body.String(), "foreign-app")
	apps = memoryAdminData[[]memoryAdminApplication](t, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/applications", "implicit", "maintainer", nil))
	require.Len(t, apps, 1)
	require.Equal(t, "default-delegation", apps[0].ApplicationID)
	require.Equal(t, 403, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/applications", "default", "member", nil).Code)
	require.Equal(t, 403, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/subjects?tenantId=other-business&applicationId=foreign-app", "default", "admin", nil).Code)
	require.Equal(t, 403, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/facts?"+adminScopeQuery(sc), "foreign-hub", "admin", nil).Code)
	subjects := memoryAdminData[memory.SubjectList](t, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/subjects?tenantId=example-tenant&applicationId=memory-app", "default", "admin", nil))
	require.Len(t, subjects.Subjects, 1)
	require.Equal(t, sc, subjects.Subjects[0].Scope)
	facts := memoryAdminData[[]memory.Fact](t, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/facts?"+adminScopeQuery(sc), "default", "maintainer", nil))
	require.Len(t, facts, 1)
}
func TestMemoryAdminCRUDCASUnknownFieldsAndEvidenceFreeAudit(t *testing.T) {
	router, service, store := memoryAdminFixture(t)
	sc := memory.Scope{TenantID: "example-tenant", ApplicationID: "memory-app", UserID: "business-alice", LifecycleID: "life-a"}
	fact := seedAdminFact(t, service, sc, "PRIVATE_SYNTHETIC_ORIGINAL")
	path := "/api/v1/admin/memory/facts/" + fact.ID
	body := map[string]any{"scope": sc, "operationId": "admin-edit", "expectedVersion": 1, "content": "PRIVATE_SYNTHETIC_EDIT"}
	updated := memoryAdminData[memory.Fact](t, memoryAdminHTTP(t, router, "PATCH", path, "default", "admin", body))
	require.Equal(t, uint64(2), updated.Version)
	require.Equal(t, "PRIVATE_SYNTHETIC_EDIT", updated.Content)
	require.Equal(t, 409, memoryAdminHTTP(t, router, "PATCH", path, "default", "admin", map[string]any{"scope": sc, "operationId": "stale", "expectedVersion": 1, "content": "wrong"}).Code)
	withUnknown := map[string]any{"scope": sc, "operationId": "bad", "expectedVersion": 2, "content": "wrong", "datasetId": "forged"}
	require.Equal(t, 400, memoryAdminHTTP(t, router, "PATCH", path, "default", "admin", withUnknown).Code)
	nested := map[string]any{"tenantId": "example-tenant", "applicationId": "memory-app", "userId": "business-alice", "accountLifecycleId": "life-a", "documentId": "forged"}
	require.Equal(t, 400, memoryAdminHTTP(t, router, "DELETE", path, "default", "admin", map[string]any{"scope": nested, "operationId": "bad-nested", "expectedVersion": 2}).Code)
	forged := sc
	forged.ApplicationID = "foreign-app"
	require.Equal(t, 403, memoryAdminHTTP(t, router, "DELETE", path, "default", "admin", map[string]any{"scope": forged, "operationId": "foreign", "expectedVersion": 2}).Code)
	receipt := memoryAdminData[memory.Operation](t, memoryAdminHTTP(t, router, "DELETE", path, "default", "admin", map[string]any{"scope": sc, "operationId": "delete", "expectedVersion": 2}))
	require.True(t, receipt.StoppedSupply)
	require.Equal(t, "retrievalReplica", receipt.CleanupScope)
	facts := memoryAdminData[[]memory.Fact](t, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/facts?"+adminScopeQuery(sc), "default", "admin", nil))
	require.Empty(t, facts)
	receipt = memoryAdminData[memory.Operation](t, memoryAdminHTTP(t, router, "GET", "/api/v1/admin/memory/operations/delete?"+adminScopeQuery(sc), "default", "admin", nil))
	require.Equal(t, "delete", receipt.ID)
	memoryAdminData[memory.Operation](t, memoryAdminHTTP(t, router, "POST", "/api/v1/admin/memory/consents/revoke", "default", "admin", map[string]any{"scope": sc, "operationId": "revoke", "consentRef": "synthetic-consent"}))
	contextResult := memoryAdminData[memory.Context](t, memoryAdminHTTP(t, router, "POST", "/api/v1/admin/memory/context", "default", "admin", map[string]any{"scope": sc, "query": "读回"}))
	require.Empty(t, contextResult.Facts)
	require.NotEmpty(t, store.rows)
	for _, entry := range store.rows {
		require.NotContains(t, entry.Detail, "PRIVATE_SYNTHETIC")
		require.NotContains(t, entry.Detail, "business-alice")
		require.NotContains(t, entry.Detail, "service-secret")
		require.Equal(t, "default", entry.TenantID)
		if entry.TargetID != "" {
			require.Len(t, entry.TargetID, 64)
		}
	}
}
