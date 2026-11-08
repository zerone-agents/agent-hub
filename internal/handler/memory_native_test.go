package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"control-panel/internal/config"
	"control-panel/internal/memory"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryNativeCapabilitiesAndUnsupportedHTTPActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := adminMemoryService(t, "example-tenant", "native-memory-app", "native-bank", memory.BackendNative)
	binding := MemoryBinding{Client: config.MemoryClient{Token: strings.Repeat("k", 32), TenantID: "example-tenant", ApplicationID: "native-memory-app", PolicyVersion: "synthetic-v1", HubTenantID: "default"}, Service: service}
	router := gin.New()
	RegisterMemoryRoutes(router, NewMemoryHandler([]MemoryBinding{binding}, nil))
	sc := memory.Scope{TenantID: "example-tenant", ApplicationID: "native-memory-app", UserID: "alice", LifecycleID: "native-life"}
	fact := seedAdminFact(t, service, sc, "我喜欢乌龙茶。")
	caps := memoryData[memory.BackendCapabilities](t, memoryHTTP(t, router, "GET", "/api/v1/memory/capabilities", "alice", "native-life", nil))
	require.Equal(t, memory.BackendNative, caps.Backend)
	require.True(t, caps.RawOnly)
	require.True(t, caps.CorrectionSupported)
	require.False(t, caps.ForgetSupported)
	require.False(t, caps.PhysicalDeleteSupported)
	require.False(t, caps.ConsentRevokeSupported)
	require.False(t, caps.DeactivateSupported)
	cases := []struct {
		method, path string
		body         any
		operationID  string
	}{
		{"DELETE", "/api/v1/memory/facts/" + fact.ID, map[string]any{"operationId": "deny-delete", "expectedVersion": 1}, "deny-delete"},
		{"POST", "/api/v1/memory/consents/revoke", map[string]any{"operationId": "deny-revoke", "consentRef": "synthetic-consent"}, "deny-revoke"},
		{"POST", "/api/v1/memory/lifecycle/deactivate", map[string]any{"operationId": "deny-deactivate"}, "deny-deactivate"},
	}
	for _, test := range cases {
		response := memoryHTTP(t, router, test.method, test.path, "alice", "native-life", test.body)
		require.Equal(t, 501, response.Code, response.Body.String())
		var body struct {
			Error struct {
				Reason string `json:"reason"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.Equal(t, "MEMORY_OPERATION_UNSUPPORTED", body.Error.Reason)
		require.NotContains(t, response.Body.String(), "physicalCleanup")
		_, err := service.GetOperation(context.Background(), sc, test.operationID)
		require.ErrorIs(t, err, memory.ErrNotFound)
	}
	facts := memoryData[[]memory.Fact](t, memoryHTTP(t, router, "GET", "/api/v1/memory/facts", "alice", "native-life", nil))
	require.Len(t, facts, 1)
	require.Equal(t, uint64(1), facts[0].Version)
	// The manager API advertises the actual service policy and returns the same
	// stable unsupported reason rather than translating a no-op into success.
	admin := gin.New()
	admin.Use(func(c *gin.Context) { c.Set("tenant_id", "default"); c.Set("roles", []string{"admin"}) })
	RegisterMemoryAdminRoutes(admin.Group("/api/v1/admin"), admin.Group("/api/v1/admin"), NewMemoryAdminHandler([]MemoryBinding{binding}, nil))
	apps := memoryAdminData[[]memoryAdminApplication](t, memoryAdminHTTP(t, admin, "GET", "/api/v1/admin/memory/applications", "default", "admin", nil))
	require.Len(t, apps, 1)
	require.Equal(t, caps, apps[0].BackendCapabilities)
	for _, test := range []struct {
		method, path string
		body         any
	}{
		{"DELETE", "/api/v1/admin/memory/facts/" + fact.ID, map[string]any{"scope": sc, "operationId": "admin-deny-delete", "expectedVersion": 1}},
		{"POST", "/api/v1/admin/memory/consents/revoke", map[string]any{"scope": sc, "operationId": "admin-deny-revoke", "consentRef": "synthetic-consent"}},
	} {
		response := memoryAdminHTTP(t, admin, test.method, test.path, "default", "admin", test.body)
		require.Equal(t, 501, response.Code, response.Body.String())
		var body struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.Equal(t, "MEMORY_OPERATION_UNSUPPORTED", body.Code)
		require.NotContains(t, response.Body.String(), "physicalCleanup")
	}
}
