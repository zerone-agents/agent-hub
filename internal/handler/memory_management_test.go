package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type memoryManagementNative struct {
	bankCalls, statusCalls int
	diagnosticCalls        int
	listCalls, recallCalls int
	scope, id              string
	query                  string
	searchOptions          memory.NativeSearchOptions
	emptyDiagnostic        bool
	enabled, expected      bool
	current                bool
	statusErr              error
}

func (f *memoryManagementNative) BankInfo(context.Context) (memory.NativeBankInfo, error) {
	f.bankCalls++
	capacity := int64(5242880)
	temperature := 0.0
	return memory.NativeBankInfo{Name: "Synthetic chat memory", MemoryTypes: []string{"raw", "semantic", "episodic", "procedural"}, ChatModel: "356", EmbeddingModel: "337", Permissions: "me", CapacityBytes: &capacity, Temperature: &temperature}, nil
}
func (f *memoryManagementNative) SetMessageStatus(_ context.Context, scope, id string, enabled, expected bool) (memory.NativeMessage, error) {
	f.statusCalls++
	f.scope, f.id, f.enabled, f.expected = scope, id, enabled, expected
	if f.statusErr != nil {
		return memory.NativeMessage{}, f.statusErr
	}
	if expected != f.current {
		return memory.NativeMessage{}, memory.ErrConflict
	}
	f.current = enabled
	return memory.NativeMessage{ID: id, SourceID: "0", Type: "raw", Content: "PRIVATE_SYNTHETIC_CHAT_BODY", Status: enabled, NativeStatus: enabled, StatusChangeAllowed: true}, nil
}
func (*memoryManagementNative) AddConversation(context.Context, string, memory.NativeTurn) (memory.NativeIngestResult, error) {
	return memory.NativeIngestResult{}, memory.ErrUnsupported
}
func (*memoryManagementNative) FindConversation(context.Context, string, memory.NativeTurn) (memory.NativeIngestResult, error) {
	return memory.NativeIngestResult{}, memory.ErrUnsupported
}
func (f *memoryManagementNative) ListMemories(context.Context, string) ([]memory.NativeMessage, error) {
	f.listCalls++
	return nil, memory.ErrUnsupported
}
func (f *memoryManagementNative) SearchMemories(context.Context, string, string, int) ([]memory.NativeMessage, error) {
	f.recallCalls++
	return nil, memory.ErrUnsupported
}
func (f *memoryManagementNative) SearchDiagnostic(_ context.Context, scope, query string, options memory.NativeSearchOptions) ([]memory.NativeMessage, error) {
	f.diagnosticCalls++
	f.scope, f.query, f.searchOptions = scope, query, options
	if f.emptyDiagnostic {
		return nil, nil
	}
	return []memory.NativeMessage{{ID: "7008", SourceID: "7001", Type: "semantic", Content: "PRIVATE_NATIVE_DIAGNOSTIC_HIT", Status: true, NativeStatus: true}}, nil
}

func memoryManagementFixture(t *testing.T) (*gin.Engine, *gorm.DB, memory.Scope, *memoryManagementNative, *memoryAdminAuditStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/management.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, memory.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("34", 32))
	require.NoError(t, err)
	native := &memoryManagementNative{current: true}
	svc, err := memory.NewService(db, memory.Config{Backend: memory.BackendNative, TenantID: "business-tenant", ApplicationID: "chat-app", PolicyVersion: "synthetic-v1", DatasetID: "PRIVATE_NATIVE_BANK_ID", SyntheticOnly: true, Wrapper: wrapper, Native: native})
	require.NoError(t, err)
	scope := memory.Scope{TenantID: "business-tenant", ApplicationID: "chat-app", UserID: "user-private-synthetic", LifecycleID: "life-private-synthetic"}
	_, err = svc.Activate(context.Background(), scope, "management-activate")
	require.NoError(t, err)
	foreign := adminMemoryService(t, "foreign-business", "foreign-app", "foreign-bank", memory.BackendNative)
	legacy := adminMemoryService(t, "legacy-business", "legacy-app", "legacy-dataset")
	store := &memoryAdminAuditStore{}
	h := NewMemoryAdminHandler([]MemoryBinding{
		{Client: config.MemoryClient{Token: "SERVICE_SECRET_MUST_NOT_LEAK", TenantID: scope.TenantID, ApplicationID: scope.ApplicationID, MemoryID: "PRIVATE_NATIVE_BANK_ID", Backend: memory.BackendNative, AutomaticExtraction: true, HubTenantID: "delegated-hub"}, Service: svc},
		{Client: config.MemoryClient{TenantID: "foreign-business", ApplicationID: "foreign-app", HubTenantID: "foreign-hub"}, Service: foreign},
		{Client: config.MemoryClient{TenantID: "legacy-business", ApplicationID: "legacy-app", HubTenantID: "delegated-hub"}, Service: legacy},
	}, services.NewAuditRecorder(store))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, c.GetHeader("X-Test-Hub-Tenant"))
		c.Set("roles", []string{c.GetHeader("X-Test-Role")})
		c.Set("user_id", "manager-1")
	})
	RegisterMemoryAdminRoutes(r.Group("/api/v1/admin"), r.Group("/api/v1/admin"), h)
	return r, db, scope, native, store
}

func TestMemoryManagementNativeConfigurationDelegationAndManagerGate(t *testing.T) {
	router, _, _, native, store := memoryManagementFixture(t)
	path := "/api/v1/admin/memory/native-config?tenantId=business-tenant&applicationId=chat-app"
	response := memoryAdminHTTP(t, router, "GET", path, "delegated-hub", "maintainer", nil)
	info := memoryAdminData[memory.NativeBankInfo](t, response)
	require.Equal(t, "Synthetic chat memory", info.Name)
	require.Len(t, info.MemoryTypes, 4)
	require.NotNil(t, info.Temperature)
	require.Zero(t, *info.Temperature)
	require.JSONEq(t, `{"success":true,"data":{"name":"Synthetic chat memory","memoryTypes":["raw","semantic","episodic","procedural"],"chatModel":"356","embeddingModel":"337","permissions":"me","capacityBytes":5242880,"temperature":0}}`, response.Body.String())
	require.Equal(t, 1, native.bankCalls)
	for _, tc := range []struct {
		path, hub, role string
		status          int
	}{
		{path, "delegated-hub", "member", 403},
		{path, "foreign-hub", "admin", 403},
		{path, "", "admin", 401},
		{"/api/v1/admin/memory/native-config?tenantId=foreign-business&applicationId=foreign-app", "delegated-hub", "admin", 403},
		{path + "&tenantId=business-tenant", "delegated-hub", "admin", 400},
		{path + "&memoryId=forged-bank", "delegated-hub", "admin", 400},
		{path + "&userId=unexpected-subject", "delegated-hub", "admin", 400},
	} {
		t.Run(tc.hub+"/"+tc.role+"/"+tc.path, func(t *testing.T) {
			result := memoryAdminHTTP(t, router, "GET", tc.path, tc.hub, tc.role, nil)
			require.Equal(t, tc.status, result.Code, result.Body.String())
			require.Equal(t, 1, native.bankCalls)
			require.Zero(t, native.statusCalls)
		})
	}
	require.NotContains(t, response.Body.String(), "PRIVATE_NATIVE_BANK_ID")
	require.NotContains(t, response.Body.String(), "SERVICE_SECRET")
	require.Equal(t, audit.Action("memory.admin.native.config.read"), store.rows[0].Action)
}

func TestMemoryManagementNativeStatusFalsePreconditionAndEvidenceFreeAudit(t *testing.T) {
	router, _, scope, native, store := memoryManagementFixture(t)
	const id = "9223372036854775806"
	path := "/api/v1/admin/memory/native-messages/" + id
	result := memoryAdminHTTP(t, router, "PATCH", path, "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": false, "expectedEnabled": true})
	message := memoryAdminData[memory.NativeMessage](t, result)
	require.False(t, message.NativeStatus)
	require.False(t, message.Status)
	require.Equal(t, id, message.ID)
	require.Equal(t, id, native.id)
	require.False(t, native.enabled)
	require.True(t, native.expected)
	encoded, err := json.Marshal(scope)
	require.NoError(t, err)
	digest := sha256.Sum256(encoded)
	require.Equal(t, "memory-"+hex.EncodeToString(digest[:]), native.scope)
	message = memoryAdminData[memory.NativeMessage](t, memoryAdminHTTP(t, router, "PATCH", path, "delegated-hub", "maintainer", map[string]any{"scope": scope, "enabled": true, "expectedEnabled": false}))
	require.True(t, message.NativeStatus)
	require.True(t, native.enabled)
	require.False(t, native.expected)
	require.Equal(t, 409, memoryAdminHTTP(t, router, "PATCH", path, "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": false, "expectedEnabled": false}).Code)
	require.Equal(t, 3, native.statusCalls)
	require.True(t, native.current)
	memoryAdminData[memory.NativeMessage](t, memoryAdminHTTP(t, router, "PATCH", "/api/v1/admin/memory/native-messages/7002", "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": false, "expectedEnabled": true}))
	require.Equal(t, 4, native.statusCalls)
	require.Len(t, store.rows, 4)
	require.NotEqual(t, store.rows[0].TargetID, store.rows[3].TargetID)
	for i, row := range store.rows {
		require.Equal(t, audit.Action("memory.admin.native.status.update"), row.Action)
		require.Equal(t, "delegated-hub", row.TenantID)
		require.Len(t, row.TargetID, 64)
		require.NotContains(t, row.Detail, "PRIVATE_SYNTHETIC_CHAT_BODY")
		require.NotContains(t, row.Detail, "PRIVATE_NATIVE_BANK_ID")
		require.NotContains(t, row.Detail, "SERVICE_SECRET")
		require.NotContains(t, row.Detail, scope.UserID)
		require.NotContains(t, row.Detail, scope.LifecycleID)
		var detail map[string]any
		require.NoError(t, json.Unmarshal([]byte(row.Detail), &detail))
		expectedID := id
		if i == 3 {
			expectedID = "7002"
		}
		require.Equal(t, expectedID, detail["nativeMessageId"])
		// Pointer-valued audit metadata must preserve explicit false values.
		require.Equal(t, i == 1, detail["enabled"])
		require.Equal(t, i == 0 || i == 3, detail["expectedEnabled"])
		target := sha256.Sum256([]byte(hex.EncodeToString(digest[:]) + "\x00" + expectedID))
		require.Equal(t, hex.EncodeToString(target[:]), row.TargetID)
		if i == 2 {
			require.Equal(t, audit.StatusFailure, row.Status)
		} else {
			require.Equal(t, audit.StatusSuccess, row.Status)
		}
	}
}

func TestMemoryManagementNativeStatusStrictBodiesAndAuthorizationHaveNoCalls(t *testing.T) {
	router, _, scope, native, store := memoryManagementFixture(t)
	path := "/api/v1/admin/memory/native-messages/7001"
	body := func() map[string]any {
		return map[string]any{"scope": scope, "enabled": false, "expectedEnabled": true}
	}
	missingExpected := body()
	delete(missingExpected, "expectedEnabled")
	missingEnabled := body()
	delete(missingEnabled, "enabled")
	unknown := body()
	unknown["memoryId"] = "forged-bank"
	wrongScope := scope
	wrongScope.ApplicationID = "foreign-app"
	foreign := body()
	foreign["scope"] = wrongScope
	for _, tc := range []struct {
		name, hub, role string
		body            any
		status          int
	}{
		{"missing expected", "delegated-hub", "admin", missingExpected, 400},
		{"missing enabled", "delegated-hub", "admin", missingEnabled, 400},
		{"null expected", "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": false, "expectedEnabled": nil}, 400},
		{"string enabled", "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": "false", "expectedEnabled": true}, 400},
		{"unknown field", "delegated-hub", "admin", unknown, 400},
		{"nested scope field", "delegated-hub", "admin", map[string]any{"scope": map[string]any{"tenantId": scope.TenantID, "applicationId": scope.ApplicationID, "userId": scope.UserID, "accountLifecycleId": scope.LifecycleID, "bankId": "forged"}, "enabled": false, "expectedEnabled": true}, 400},
		{"foreign app", "delegated-hub", "admin", foreign, 403},
		{"foreign delegation", "foreign-hub", "admin", body(), 403},
		{"member", "delegated-hub", "member", body(), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := memoryAdminHTTP(t, router, "PATCH", path, tc.hub, tc.role, tc.body)
			require.Equal(t, tc.status, result.Code, result.Body.String())
			require.Zero(t, native.statusCalls)
			require.Zero(t, native.bankCalls)
		})
	}
	for _, id := range []string{"0", "01", "-1", "bad-id", "9223372036854775808", "PRIVATE_AUDIT_ID_TEXT"} {
		result := memoryAdminHTTP(t, router, "PATCH", "/api/v1/admin/memory/native-messages/"+id, "delegated-hub", "admin", body())
		require.Equal(t, 400, result.Code, result.Body.String())
		require.Zero(t, native.statusCalls)
		var detail map[string]any
		require.NoError(t, json.Unmarshal([]byte(store.rows[len(store.rows)-1].Detail), &detail))
		require.NotContains(t, detail, "nativeMessageId")
		require.NotContains(t, store.rows[len(store.rows)-1].Detail, "PRIVATE_AUDIT_ID_TEXT")
	}
}

func TestMemoryManagementNativeStatusInactiveScopeDoesNotReachProvider(t *testing.T) {
	router, db, scope, native, _ := memoryManagementFixture(t)
	require.NoError(t, db.Table("memory_lifecycles").Where("state = ?", "active").Update("state", "frozen").Error)
	result := memoryAdminHTTP(t, router, "PATCH", "/api/v1/admin/memory/native-messages/7001", "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": false, "expectedEnabled": true})
	require.Equal(t, 403, result.Code, result.Body.String())
	require.Zero(t, native.statusCalls)
	require.Zero(t, native.bankCalls)
}

func TestMemoryManagementLegacyBackendUnsupportedAndUnknownOutcome(t *testing.T) {
	router, _, scope, native, store := memoryManagementFixture(t)
	legacyScope := memory.Scope{TenantID: "legacy-business", ApplicationID: "legacy-app", UserID: scope.UserID, LifecycleID: scope.LifecycleID}
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/admin/memory/native-config?tenantId=legacy-business&applicationId=legacy-app", nil},
		{"PATCH", "/api/v1/admin/memory/native-messages/7001", map[string]any{"scope": legacyScope, "enabled": false, "expectedEnabled": true}},
		{"POST", "/api/v1/admin/memory/native-search", map[string]any{"scope": legacyScope, "query": "synthetic diagnostic"}},
	} {
		result := memoryAdminHTTP(t, router, tc.method, tc.path, "delegated-hub", "admin", tc.body)
		require.Equal(t, 501, result.Code, result.Body.String())
		require.Contains(t, result.Body.String(), "MEMORY_OPERATION_UNSUPPORTED")
	}
	require.Zero(t, native.bankCalls)
	require.Zero(t, native.statusCalls)
	require.Zero(t, native.diagnosticCalls)
	native.statusErr = fmt.Errorf("%w: PRIVATE_PROVIDER_SECRET", memory.ErrIndexUnknown)
	result := memoryAdminHTTP(t, router, "PATCH", "/api/v1/admin/memory/native-messages/7001", "delegated-hub", "admin", map[string]any{"scope": scope, "enabled": false, "expectedEnabled": true})
	require.Equal(t, 503, result.Code, result.Body.String())
	require.Contains(t, result.Body.String(), "memory_outcome_unknown")
	require.NotContains(t, result.Body.String(), "PRIVATE_PROVIDER_SECRET")
	require.Equal(t, 1, native.statusCalls)
	require.True(t, native.current)
	require.Equal(t, audit.StatusFailure, store.rows[len(store.rows)-1].Status)
}

func TestMemoryManagementNativeSearchDefaultsExplicitZeroAndNativeHitsOnly(t *testing.T) {
	router, _, scope, native, store := memoryManagementFixture(t)
	const path = "/api/v1/admin/memory/native-search"
	const query = "PRIVATE_SYNTHETIC_DIAGNOSTIC_QUESTION"
	for _, tc := range []struct {
		name    string
		fields  map[string]any
		options memory.NativeSearchOptions
	}{
		{"defaults", nil, memory.NativeSearchOptions{Limit: 5, SimilarityThreshold: 0.2, KeywordWeight: 0.7}},
		{"explicit zero", map[string]any{"limit": 1, "similarityThreshold": 0, "keywordWeight": 0}, memory.NativeSearchOptions{Limit: 1, SimilarityThreshold: 0, KeywordWeight: 0}},
		{"inclusive upper bounds", map[string]any{"limit": 20, "similarityThreshold": 1, "keywordWeight": 1}, memory.NativeSearchOptions{Limit: 20, SimilarityThreshold: 1, KeywordWeight: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"scope": scope, "query": query}
			for key, value := range tc.fields {
				body[key] = value
			}
			response := memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "maintainer", body)
			result := memoryAdminData[memory.NativeSearchResult](t, response)
			require.Equal(t, "ready", result.Status)
			require.Equal(t, tc.options, result.Options)
			require.Equal(t, tc.options, native.searchOptions)
			require.Equal(t, query, native.query)
			encoded, err := json.Marshal(scope)
			require.NoError(t, err)
			digest := sha256.Sum256(encoded)
			require.Equal(t, "memory-"+hex.EncodeToString(digest[:]), native.scope)
			require.Len(t, result.Messages, 1)
			require.Equal(t, "7008", result.Messages[0].ID)
			require.Equal(t, "semantic", result.Messages[0].Type)
			// This endpoint exposes the provider result, without chat's recent-RAW
			// supplementation. Parameter transport is not a ranking-quality claim.
			require.Zero(t, native.listCalls)
			require.Zero(t, native.recallCalls)
			require.Zero(t, native.statusCalls)
			require.Zero(t, native.bankCalls)
			if tc.name == "explicit zero" {
				require.Contains(t, response.Body.String(), `"similarityThreshold":0`)
				require.Contains(t, response.Body.String(), `"keywordWeight":0`)
			}
		})
	}
	require.Equal(t, 3, native.diagnosticCalls)
	native.emptyDiagnostic = true
	empty := memoryAdminData[memory.NativeSearchResult](t, memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "admin", map[string]any{"scope": scope, "query": query}))
	require.Equal(t, "empty", empty.Status)
	require.NotNil(t, empty.Messages)
	require.Empty(t, empty.Messages)
	for _, row := range store.rows {
		require.Equal(t, audit.Action("memory.admin.native.search"), row.Action)
		require.NotContains(t, row.Detail, query)
		require.NotContains(t, row.Detail, "PRIVATE_NATIVE_DIAGNOSTIC_HIT")
		require.NotContains(t, row.Detail, "SERVICE_SECRET")
		require.NotContains(t, row.Detail, scope.UserID)
		require.NotContains(t, row.Detail, scope.LifecycleID)
	}
}

func TestMemoryManagementNativeSearchInvalidRequestsNeverReachProvider(t *testing.T) {
	router, _, scope, native, _ := memoryManagementFixture(t)
	const path = "/api/v1/admin/memory/native-search"
	for _, tc := range []struct {
		name   string
		fields map[string]any
	}{
		{"empty query", map[string]any{"query": ""}},
		{"blank query", map[string]any{"query": " \t "}},
		{"long query", map[string]any{"query": strings.Repeat("问", 2001)}},
		{"limit zero", map[string]any{"limit": 0}},
		{"limit negative", map[string]any{"limit": -1}},
		{"limit too large", map[string]any{"limit": 21}},
		{"threshold negative", map[string]any{"similarityThreshold": -0.01}},
		{"threshold too large", map[string]any{"similarityThreshold": 1.01}},
		{"weight negative", map[string]any{"keywordWeight": -0.01}},
		{"weight too large", map[string]any{"keywordWeight": 1.01}},
		{"string threshold", map[string]any{"similarityThreshold": "0"}},
		{"unknown field", map[string]any{"rawSupplement": true}},
		{"nested unknown field", map[string]any{"scope": map[string]any{"tenantId": scope.TenantID, "applicationId": scope.ApplicationID, "userId": scope.UserID, "accountLifecycleId": scope.LifecycleID, "bankId": "foreign"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"scope": scope, "query": "valid synthetic question"}
			for key, value := range tc.fields {
				body[key] = value
			}
			response := memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "admin", body)
			require.Equal(t, 400, response.Code, response.Body.String())
			require.Zero(t, native.diagnosticCalls)
		})
	}
	for _, tc := range []struct {
		name, hub, role string
		scope           memory.Scope
		status          int
	}{
		{"member", "delegated-hub", "member", scope, 403},
		{"foreign delegation", "foreign-hub", "admin", scope, 403},
		{"foreign application", "delegated-hub", "admin", memory.Scope{TenantID: scope.TenantID, ApplicationID: "foreign-app", UserID: scope.UserID, LifecycleID: scope.LifecycleID}, 403},
		{"foreign subject", "delegated-hub", "admin", memory.Scope{TenantID: scope.TenantID, ApplicationID: scope.ApplicationID, UserID: "other-user", LifecycleID: scope.LifecycleID}, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := memoryAdminHTTP(t, router, "POST", path, tc.hub, tc.role, map[string]any{"scope": tc.scope, "query": "valid synthetic question"})
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.Zero(t, native.diagnosticCalls)
		})
	}
	require.Zero(t, native.listCalls)
	require.Zero(t, native.recallCalls)
	require.Zero(t, native.statusCalls)
	require.Zero(t, native.bankCalls)
}

func TestMemoryManagementNativeSearchInactiveScopeNeverReachesProvider(t *testing.T) {
	router, db, scope, native, _ := memoryManagementFixture(t)
	require.NoError(t, db.Table("memory_lifecycles").Where("state = ?", "active").Update("state", "frozen").Error)
	response := memoryAdminHTTP(t, router, "POST", "/api/v1/admin/memory/native-search", "delegated-hub", "admin", map[string]any{"scope": scope, "query": "synthetic diagnostic"})
	require.Equal(t, 403, response.Code, response.Body.String())
	require.Zero(t, native.diagnosticCalls)
	require.Zero(t, native.listCalls)
	require.Zero(t, native.recallCalls)
}
