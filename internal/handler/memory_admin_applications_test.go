package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memory"
	"control-panel/internal/memoryapp"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type consoleMemoryBanks struct {
	creates, finds int
	unknown        bool
	last           memoryapp.BankSpec
}

func (b *consoleMemoryBanks) Create(_ context.Context, spec memoryapp.BankSpec) (memoryapp.Bank, error) {
	b.creates++
	b.last = spec
	if b.unknown {
		return memoryapp.Bank{}, memoryapp.ErrUnknown
	}
	return memoryapp.Bank{ID: fmt.Sprintf("bank-%d", b.creates), Name: spec.Name, ChatModel: spec.ChatModel, EmbeddingModel: spec.EmbeddingModel}, nil
}
func (b *consoleMemoryBanks) Find(_ context.Context, spec memoryapp.BankSpec) (memoryapp.Bank, error) {
	b.finds++
	if b.unknown {
		return memoryapp.Bank{}, memoryapp.ErrNotFound
	}
	return memoryapp.Bank{ID: "reconciled-bank", Name: spec.Name, ChatModel: spec.ChatModel, EmbeddingModel: spec.EmbeddingModel}, nil
}

func consoleMemoryFixture(t *testing.T) (*gin.Engine, *consoleMemoryBanks, *memoryapp.Registry, *MemoryAdminHandler) {
	t.Helper()
	_, _, db := memoryTestRouter(t)
	require.NoError(t, memoryapp.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("12", 32))
	require.NoError(t, err)
	banks := &consoleMemoryBanks{}
	registry, err := memoryapp.NewRegistry(db, wrapper, banks)
	require.NoError(t, err)
	h := NewMemoryAdminHandler(nil, nil)
	h.SetApplicationManagement(registry, &stubMultiRAGMyLLMs{data: []byte(`{"Models":{"llm":[{"id":356,"reference":"356","name":"chat-one","type":"chat","status":"1"},{"id":357,"reference":"357","name":"chat-two","type":"chat","status":"1"},{"id":337,"reference":"337","name":"embed","type":"embedding","status":"1"},{"id":999,"reference":"999","name":"disabled","type":"chat","status":"0"}]}}`)})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, c.GetHeader("X-Test-Hub-Tenant"))
		c.Set("roles", []string{c.GetHeader("X-Test-Role")})
	})
	RegisterMemoryAdminRoutes(r.Group("/api/v1/admin"), r.Group("/api/v1/admin"), h)
	return r, banks, registry, h
}

func consoleMemoryInput() memoryAdminCreateApplication {
	return memoryAdminCreateApplication{OperationID: "console-create", ApplicationID: "console-app", Name: "合成测试应用", ChatModel: "356", EmbeddingModel: "337"}
}

func TestConsoleMemoryCreationNeedsNoManagerConfiguration(t *testing.T) {
	r, banks, registry, h := consoleMemoryFixture(t)
	store := &memoryAdminAuditStore{}
	h.audit = services.NewAuditRecorder(store)
	in := consoleMemoryInput()
	for _, role := range []string{"member", "guest", ""} {
		w := memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "default", role, in)
		require.Equal(t, 403, w.Code)
	}
	require.Equal(t, 0, banks.creates)
	created := memoryAdminData[memoryapp.CreateResult](t, memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "default", "maintainer", in))
	require.Equal(t, "default", created.Application.TenantID)
	require.Equal(t, "default", created.Application.HubTenantID)
	require.Equal(t, "active", created.Application.Status)
	require.Equal(t, "356", created.Application.ChatModel)
	require.Equal(t, "337", created.Application.EmbeddingModel)
	require.Len(t, created.ApplicationToken, 64)
	for _, row := range store.rows {
		require.NotContains(t, row.Detail, created.ApplicationToken)
		require.NotContains(t, row.Detail, in.Name)
	}
	require.Len(t, store.rows[len(store.rows)-1].TargetID, 64)
	_, err := registry.Authenticate(context.Background(), created.ApplicationToken)
	require.NoError(t, err)
	repeated := memoryAdminData[memoryapp.CreateResult](t, memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "default", "admin", in))
	require.Equal(t, created.ApplicationToken, repeated.ApplicationToken)
	require.Equal(t, 1, banks.creates)
	in.ChatModel = "357"
	w := memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "default", "admin", in)
	require.Equal(t, 409, w.Code)
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Equal(t, 1, banks.creates)
	for _, path := range []string{"/applications", "/applications/console-app/provisioning"} {
		w = memoryAdminHTTP(t, r, "GET", "/api/v1/admin/memory"+path, "default", "admin", nil)
		require.Equal(t, 200, w.Code)
		require.NotContains(t, w.Body.String(), created.ApplicationToken)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}

func TestConsoleMemoryUnknownCreationReconcilesWithoutDuplicateBank(t *testing.T) {
	r, banks, _, h := consoleMemoryFixture(t)
	banks.unknown = true
	in := consoleMemoryInput()
	w := memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "hub-a", "admin", in)
	require.Equal(t, 202, w.Code)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"status":"unknown"`)
	apps := memoryAdminData[[]memoryAdminApplication](t, memoryAdminHTTP(t, r, "GET", "/api/v1/admin/memory/applications", "hub-a", "admin", nil))
	require.Len(t, apps, 1)
	require.Equal(t, "unknown", apps[0].Status)
	foreign := memoryAdminData[[]memoryAdminApplication](t, memoryAdminHTTP(t, r, "GET", "/api/v1/admin/memory/applications", "hub-b", "admin", nil))
	require.Empty(t, foreign)
	w = memoryAdminHTTP(t, r, "GET", "/api/v1/admin/memory/applications/console-app/provisioning", "hub-b", "admin", nil)
	require.Equal(t, 404, w.Code)
	require.Zero(t, banks.finds)
	h.applicationModels = &stubMultiRAGMyLLMs{err: memoryapp.ErrUnavailable}
	w = memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "hub-a", "admin", in)
	require.Equal(t, 202, w.Code)
	require.Equal(t, 1, banks.creates)
	banks.unknown = false
	app := memoryAdminData[memoryapp.Application](t, memoryAdminHTTP(t, r, "GET", "/api/v1/admin/memory/applications/console-app/provisioning", "hub-a", "admin", nil))
	require.Equal(t, "active", app.Status)
	confirmed := memoryAdminData[memoryapp.CreateResult](t, memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "hub-a", "admin", in))
	require.Equal(t, "reconciled-bank", confirmed.Application.MemoryID)
	require.Equal(t, 1, banks.creates)
}

func TestConsoleMemoryRejectsTenantAndUnavailableModelOverrides(t *testing.T) {
	r, banks, _, _ := consoleMemoryFixture(t)
	models := memoryAdminData[memoryApplicationModels](t, memoryAdminHTTP(t, r, "GET", "/api/v1/admin/memory/application-models", "hub", "admin", nil))
	require.Len(t, models.Chat, 2)
	require.Len(t, models.Embedding, 1)
	for _, model := range []string{"999", "337", "not-in-catalog"} {
		in := consoleMemoryInput()
		in.ChatModel = model
		w := memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "hub", "admin", in)
		require.Equal(t, 400, w.Code)
	}
	w := memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "hub", "admin", map[string]any{"tenantId": "victim", "applicationId": "app", "chatModel": "356", "embeddingModel": "337"})
	require.Equal(t, 400, w.Code)
	w = memoryAdminHTTP(t, r, "POST", "/api/v1/admin/memory/applications", "", "admin", consoleMemoryInput())
	require.Equal(t, 401, w.Code)
	require.Zero(t, banks.creates)
}
