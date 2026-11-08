package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/config"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/handler"
	"control-panel/internal/memory"
	"control-panel/internal/memoryapp"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type applicationTestBanks struct {
	creates int
	unknown bool
}

func (b *applicationTestBanks) Create(_ context.Context, s memoryapp.BankSpec) (memoryapp.Bank, error) {
	b.creates++
	if b.unknown {
		return memoryapp.Bank{}, memoryapp.ErrUnknown
	}
	return memoryapp.Bank{ID: fmt.Sprintf("bank-%d", b.creates), Name: s.Name, ChatModel: s.ChatModel, EmbeddingModel: s.EmbeddingModel}, nil
}
func (b *applicationTestBanks) Find(context.Context, memoryapp.BankSpec) (memoryapp.Bank, error) {
	return memoryapp.Bank{}, memoryapp.ErrNotFound
}

func applicationTestSource(t *testing.T, banks memoryapp.BankProvisioner) *memoryApplicationSource {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/apps.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, memory.Migrate(db))
	require.NoError(t, memoryapp.Migrate(db))
	w, err := memory.NewSyntheticKeyWrapper(strings.Repeat("12", 32))
	require.NoError(t, err)
	registry, err := memoryapp.NewRegistry(db, w, banks)
	require.NoError(t, err)
	return &memoryApplicationSource{db: db, registry: registry, wrapper: w, cfg: config.MemoryConfig{MultiragURL: "http://127.0.0.1:1", MultiragKey: "sdk-only-test"}}
}
func applicationTestRouter(s *memoryApplicationSource, models ...string) *gin.Engine {
	chat, embedding := "chat", "embed"
	if len(models) == 2 {
		chat, embedding = models[0], models[1]
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	managers := []config.MemoryManager{{Token: strings.Repeat("m", 32), TenantID: "tenant-a", HubTenantID: "hub-a", ChatModel: chat, EmbeddingModel: embedding, MaxApplications: 32}, {Token: strings.Repeat("n", 32), TenantID: "tenant-b", HubTenantID: "hub-b", ChatModel: chat, EmbeddingModel: embedding, MaxApplications: 32}}
	handler.RegisterMemoryApplicationRoutes(r, handler.NewMemoryApplicationHandler(s.registry, managers, nil))
	business := handler.NewMemoryHandler(nil, nil)
	business.SetBindingSource(s)
	handler.RegisterMemoryRoutes(r, business)
	admin := handler.NewMemoryAdminHandler(nil, nil)
	admin.SetBindingSource(s)
	group := r.Group("/api/v1/admin", func(c *gin.Context) {
		tenant.SetTenantID(c, c.GetHeader("X-Test-Hub-Tenant"))
		c.Set("roles", []string{"admin"})
		c.Next()
	})
	handler.RegisterMemoryAdminRoutes(group, group, admin)
	return r
}
func applicationHTTP(t *testing.T, r http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Trace-Id", "application-test")
	req.Header.Set("X-Memory-User-Id", "shared-user")
	req.Header.Set("X-Memory-Lifecycle-Id", "shared-life")
	req.Header.Set("X-Test-Hub-Tenant", "hub-a")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func applicationData[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, 200, w.Code, w.Body.String())
	var e struct {
		Code int
		Data T
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &e))
	require.Equal(t, 200, e.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	return e.Data
}

func TestMemoryApplicationHTTPAndDynamicAdmission(t *testing.T) {
	banks := &applicationTestBanks{}
	s := applicationTestSource(t, banks)
	r := applicationTestRouter(s)
	manager := strings.Repeat("m", 32)
	in := memoryapp.CreateInput{OperationID: "create-a", ApplicationID: "assistant-a", Name: "Assistant A", PolicyVersion: "synthetic-v1"}
	a := applicationData[memoryapp.CreateResult](t, applicationHTTP(t, r, "POST", "/api/v1/memory/applications", manager, in))
	require.Equal(t, "active", a.Application.Status)
	// No new static binding and no process restart: both consumers see the app.
	caps := applicationData[map[string]any](t, applicationHTTP(t, r, "GET", "/api/v1/memory/capabilities", a.ApplicationToken, nil))
	require.Equal(t, true, caps["automaticExtractionSupported"])
	applicationData[memory.Operation](t, applicationHTTP(t, r, "POST", "/api/v1/memory/lifecycle/activate", a.ApplicationToken, map[string]any{"operationId": "activate"}))
	in.ApplicationID = "assistant-b"
	in.OperationID = "create-b"
	b := applicationData[memoryapp.CreateResult](t, applicationHTTP(t, r, "POST", "/api/v1/memory/applications", manager, in))
	applicationData[memory.Operation](t, applicationHTTP(t, r, "POST", "/api/v1/memory/lifecycle/activate", b.ApplicationToken, map[string]any{"operationId": "activate"}))
	bindings, err := s.BindingsForHubTenant(context.Background(), "hub-a")
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.NotEqual(t, bindings[0].Client.MemoryID, bindings[1].Client.MemoryID)
	for _, binding := range bindings {
		subjects, err := binding.Service.ListSubjects(context.Background())
		require.NoError(t, err)
		require.Len(t, subjects.Subjects, 1)
	}
	other, err := s.BindingsForHubTenant(context.Background(), "hub-b")
	require.NoError(t, err)
	require.Empty(t, other)
	admin := applicationHTTP(t, r, "GET", "/api/v1/admin/memory/applications", manager, nil)
	require.Equal(t, 200, admin.Code, admin.Body.String())
	require.Contains(t, admin.Body.String(), "assistant-a")
	for _, path := range []string{"/api/v1/memory/applications", "/api/v1/memory/applications/assistant-a"} {
		require.Equal(t, 401, applicationHTTP(t, r, "GET", path, a.ApplicationToken, nil).Code)
	}
	require.Equal(t, 401, applicationHTTP(t, r, "GET", "/api/v1/memory/capabilities", manager, nil).Code)
	require.Equal(t, 404, applicationHTTP(t, r, "GET", "/api/v1/memory/applications/assistant-a", strings.Repeat("n", 32), nil).Code)
	w := applicationHTTP(t, r, "GET", "/api/v1/memory/applications", manager, nil)
	require.NotContains(t, w.Body.String(), a.ApplicationToken)
	require.NotContains(t, w.Body.String(), "TokenDigest")
	require.NotContains(t, w.Body.String(), "tokenCipher")
	no := false
	disabled := applicationData[memoryapp.Application](t, applicationHTTP(t, r, "PATCH", "/api/v1/memory/applications/assistant-a", manager, memoryapp.UpdateInput{ExpectedVersion: a.Application.Version, Enabled: &no}))
	require.Equal(t, "disabled", disabled.Status)
	// A second registry/source simulates another instance, without cached admission.
	registry, err := memoryapp.NewRegistry(s.db, s.wrapper, banks)
	require.NoError(t, err)
	second := &memoryApplicationSource{db: s.db, registry: registry, wrapper: s.wrapper, cfg: s.cfg}
	require.Equal(t, 401, applicationHTTP(t, applicationTestRouter(second), "GET", "/api/v1/memory/capabilities", a.ApplicationToken, nil).Code)
	bindings, err = second.BindingsForHubTenant(context.Background(), "hub-a")
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	yes := true
	applicationData[memoryapp.Application](t, applicationHTTP(t, r, "PATCH", "/api/v1/memory/applications/assistant-a", manager, memoryapp.UpdateInput{ExpectedVersion: disabled.Version, Enabled: &yes}))
	require.Equal(t, 200, applicationHTTP(t, applicationTestRouter(second), "GET", "/api/v1/memory/capabilities", a.ApplicationToken, nil).Code)
	require.Equal(t, 2, banks.creates)
}

func TestMemoryApplicationHTTPStrictRequestsAndUnknown(t *testing.T) {
	banks := &applicationTestBanks{unknown: true}
	s := applicationTestSource(t, banks)
	r := applicationTestRouter(s)
	manager := strings.Repeat("m", 32)
	base := map[string]any{"operationId": "create", "applicationId": "app", "name": "App", "policyVersion": "synthetic-v1"}
	for _, key := range []string{"tenantId", "hubTenantId", "memoryId", "token", "chatModel", "embeddingModel", "baseURL"} {
		bad := map[string]any{}
		for k, v := range base {
			bad[k] = v
		}
		bad[key] = "forged"
		require.Equal(t, 400, applicationHTTP(t, r, "POST", "/api/v1/memory/applications", manager, bad).Code)
	}
	require.Zero(t, banks.creates)
	w := applicationHTTP(t, r, "POST", "/api/v1/memory/applications", manager, base)
	require.Equal(t, 202, w.Code, w.Body.String())
	var e struct{ Data memoryapp.CreateResult }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &e))
	require.Equal(t, "unknown", e.Data.Application.Status)
	require.Equal(t, 401, applicationHTTP(t, r, "GET", "/api/v1/memory/capabilities", e.Data.ApplicationToken, nil).Code)
	w = applicationHTTP(t, r, "POST", "/api/v1/memory/applications", manager, base)
	require.Equal(t, 202, w.Code)
	for i := 0; i < 2; i++ {
		require.Equal(t, 200, applicationHTTP(t, r, "GET", "/api/v1/memory/applications/app/provisioning", manager, nil).Code)
	}
	require.Equal(t, 1, banks.creates)
	for _, query := range []string{"?tenantId=tenant-b", "?limit=101", "?offset=-1", "?limit=10&limit=20"} {
		require.Equal(t, 400, applicationHTTP(t, r, "GET", "/api/v1/memory/applications"+query, manager, nil).Code)
	}
	// Missing trace is rejected before a provider mutation.
	req := httptest.NewRequest("POST", "/api/v1/memory/applications", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+manager)
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	require.Equal(t, 400, out.Code)
	require.Equal(t, 1, banks.creates)
}
