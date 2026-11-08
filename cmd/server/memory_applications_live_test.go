package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"control-panel/internal/config"
	"control-panel/internal/infrastructure/nativememory"
	"control-panel/internal/memory"
	"control-panel/internal/memoryapp"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Opt-in only: requires a pre-registered directory and an isolated MultiRAG SDK
// account. It creates two empty native banks and retains them, disabled in Hub;
// there is deliberately no DELETE or compensating cleanup in this feature.
func TestMemoryApplicationsLive(t *testing.T) {
	directory := os.Getenv("HUB_MEMORY_LIVE_DIRECTORY")
	if directory == "" {
		t.Skip("opt-in native provisioning check; see docs/memory/applications.md")
	}
	require.FileExists(t, filepath.Join(directory, "resources.json"))
	run := os.Getenv("HUB_MEMORY_LIVE_RUN")
	require.NotEmpty(t, run)
	url, key := os.Getenv("MEMORY_MULTIRAG_URL"), os.Getenv("MEMORY_MULTIRAG_KEY")
	require.NotEmpty(t, url)
	require.NotEmpty(t, key)
	chat, embedding := os.Getenv("HUB_MEMORY_LIVE_CHAT_MODEL"), os.Getenv("HUB_MEMORY_LIVE_EMBEDDING_MODEL")
	require.NotEmpty(t, chat)
	require.NotEmpty(t, embedding)
	wrapping := os.Getenv("MEMORY_WRAPPING_KEY")
	wrapper, err := memory.NewSyntheticKeyWrapper(wrapping)
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open(filepath.Join(directory, "live.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, memory.Migrate(db))
	require.NoError(t, memoryapp.Migrate(db))
	provider, err := nativememory.NewBankProvisioner(nativememory.BankProvisionerConfig{BaseURL: url, APIKey: key, ExpectedOwnerID: os.Getenv("HUB_MEMORY_LIVE_OWNER")})
	require.NoError(t, err)
	registry, err := memoryapp.NewRegistry(db, wrapper, provider)
	require.NoError(t, err)
	source := &memoryApplicationSource{db: db, registry: registry, wrapper: wrapper, cfg: config.MemoryConfig{MultiragURL: url, MultiragKey: key}}
	// The integration harness uses the production HTTP handlers and dynamic
	// source, with only the management credential/model defaults fixed for test.
	router := applicationTestRouter(source, chat, embedding)
	owner := memoryapp.Owner{TenantID: "tenant-a", HubTenantID: "hub-a", ChatModel: chat, EmbeddingModel: embedding, MaxApplications: 32}
	results := []memoryapp.CreateResult{}
	for _, suffix := range []string{"a", "b"} {
		in := memoryapp.CreateInput{OperationID: "live-create-" + run + "-" + suffix, ApplicationID: "live-" + run + "-" + suffix, Name: "Synthetic provisioning check " + suffix, PolicyVersion: "synthetic-v1"}
		response := applicationHTTP(t, router, "POST", "/api/v1/memory/applications", strings.Repeat("m", 32), in)
		var envelope struct{ Data memoryapp.CreateResult }
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		created := envelope.Data
		if created.Application.ApplicationID != "" {
			results = append(results, created)
		}
		// Persist each receipt immediately so a later assertion cannot hide an
		// already created resource. This file includes secrets and stays private.
		raw, err := json.MarshalIndent(results, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "live-receipts.private.json"), raw, 0600))
		require.Equal(t, 200, response.Code, "native application creation was not confirmed; inspect the private receipt")
		require.Equal(t, "active", created.Application.Status)
		verified, err := provider.Find(context.Background(), memoryapp.BankSpec{Name: created.Application.MemoryName, ChatModel: chat, EmbeddingModel: embedding})
		require.NoError(t, err)
		require.Equal(t, created.Application.MemoryID, verified.ID)
		for _, user := range []string{"synthetic-user-1", "synthetic-user-2"} {
			binding, err := source.Authenticate(context.Background(), created.ApplicationToken)
			require.NoError(t, err)
			_, err = binding.Service.Activate(context.Background(), memory.Scope{TenantID: owner.TenantID, ApplicationID: in.ApplicationID, UserID: user, LifecycleID: "life-" + run}, "activate-"+user)
			require.NoError(t, err)
		}
	}
	require.NotEqual(t, results[0].Application.MemoryID, results[1].Application.MemoryID)
	// A new registry instance reuses durable bindings and blocks disabled keys.
	restarted, err := memoryapp.NewRegistry(db, wrapper, provider)
	require.NoError(t, err)
	for _, result := range results {
		b, err := restarted.Authenticate(context.Background(), result.ApplicationToken)
		require.NoError(t, err)
		require.Equal(t, result.Application.MemoryID, b.MemoryID)
		no := false
		_, err = restarted.Update(context.Background(), owner.TenantID, b.ApplicationID, memoryapp.UpdateInput{ExpectedVersion: result.Application.Version, Enabled: &no})
		require.NoError(t, err)
		_, err = registry.Authenticate(context.Background(), result.ApplicationToken)
		require.ErrorIs(t, err, memoryapp.ErrDisabled)
	}
	bindings, err := restarted.ForHubTenant(context.Background(), owner.HubTenantID)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	// Output is deliberately credential-free; native model execution/extraction
	// is not part of this bank-provisioning proof.
	receipt := map[string]any{"run": run, "nativeBanks": []string{results[0].Application.MemoryID, results[1].Application.MemoryID}, "applications": 2, "lifecycles": 4, "independentReadback": true, "distinctBanks": true, "restartAdmissionChecked": true, "finalStatus": "disabled", "nativeMessagesCreated": 0, "nativeBanksRetained": true, "multiragRestarted": false, "modelExecutionVerified": false}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "live-result.json"), raw, 0600))
}
