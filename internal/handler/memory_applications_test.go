package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/config"
	"control-panel/internal/memory"
	"control-panel/internal/memoryapp"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type auditTestBanks struct{}

func (auditTestBanks) Create(_ context.Context, s memoryapp.BankSpec) (memoryapp.Bank, error) {
	return memoryapp.Bank{ID: "audit-bank", Name: s.Name, ChatModel: s.ChatModel, EmbeddingModel: s.EmbeddingModel}, nil
}
func (auditTestBanks) Find(context.Context, memoryapp.BankSpec) (memoryapp.Bank, error) {
	return memoryapp.Bank{}, memoryapp.ErrNotFound
}

func TestMemoryApplicationAuditOmitsSecretsAndRequestContent(t *testing.T) {
	_, _, db := memoryTestRouter(t)
	router := gin.New()
	require.NoError(t, memoryapp.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("12", 32))
	require.NoError(t, err)
	registry, err := memoryapp.NewRegistry(db, wrapper, auditTestBanks{})
	require.NoError(t, err)
	store := &memoryAdminAuditStore{}
	manager := strings.Repeat("k", 32)
	RegisterMemoryApplicationRoutes(router, NewMemoryApplicationHandler(registry, []config.MemoryManager{{Token: manager, TenantID: "tenant", HubTenantID: "hub", ChatModel: "chat", EmbeddingModel: "embed", MaxApplications: 32}}, services.NewAuditRecorder(store)))
	created := memoryData[memoryapp.CreateResult](t, memoryHTTP(t, router, "POST", "/api/v1/memory/applications", "", "", memoryapp.CreateInput{OperationID: "create-audit", ApplicationID: "app", Name: "PRIVATE_DISPLAY_NAME", Description: "PRIVATE_DESCRIPTION", PolicyVersion: "synthetic-v1"}))
	require.Len(t, store.rows, 1)
	row := store.rows[0]
	require.Equal(t, "tenant", row.TenantID)
	require.Equal(t, "memory.application.manage", string(row.Action))
	require.Len(t, row.TargetID, 64)
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	for _, secret := range []string{manager, created.ApplicationToken, "PRIVATE_DISPLAY_NAME", "PRIVATE_DESCRIPTION", "tokenDigest", "tokenCipher"} {
		require.NotContains(t, string(raw), secret)
	}
	require.Contains(t, row.Detail, `"applicationStatus":"active"`)
	// List/detail can be audited without ever retrieving a creation secret.
	w := memoryHTTP(t, router, "GET", "/api/v1/memory/applications/app", "", "", nil)
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), created.ApplicationToken)
	require.Len(t, store.rows, 2)
}
