package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"
	"github.com/stretchr/testify/require"
)

type instanceSyncFixture struct {
	cSyncCapture
	instances                           []provider.RemoteInstance
	models                              []provider.RemoteModel
	creates, verifies, patches, adds    int
	inputs                              []provider.InstanceInput
	lostCreate, skipCreate, unavailable bool
	patchEntered                        chan struct{}
	patchContinue                       chan struct{}
	lostPatch                           bool
	beforeList                          func()
}

func (f *instanceSyncFixture) SyncTarget() string { return "fixture-tenant" }
func (f *instanceSyncFixture) ListInstances(context.Context, string) ([]provider.RemoteInstance, error) {
	if f.beforeList != nil {
		f.beforeList()
	}
	if f.unavailable {
		return nil, provider.ErrInstanceAPIUnavailable
	}
	return f.instances, nil
}
func (f *instanceSyncFixture) VerifyInstance(context.Context, string, provider.InstanceCandidate) error {
	f.verifies++
	return nil
}
func (f *instanceSyncFixture) CreateInstance(_ context.Context, factory string, in provider.InstanceInput) (*provider.RemoteInstance, error) {
	f.creates++
	f.inputs = append(f.inputs, in)
	row := provider.RemoteInstance{ID: fmt.Sprint("instance-", f.creates), ProviderName: factory, Name: in.InstanceName, BaseURL: in.BaseURL, Region: in.Region, HasAPIKey: in.APIKey != "", Status: "active"}
	if !f.skipCreate {
		f.instances = append(f.instances, row)
		for _, model := range in.CustomModels {
			f.insertModel(row, model)
		}
	}
	if f.lostCreate {
		return nil, errors.New("unconfirmed")
	}
	return &row, nil
}
func (f *instanceSyncFixture) insertModel(row provider.RemoteInstance, model provider.ModelRegistration) provider.RemoteModel {
	ref := fmt.Sprint(9007199254740993 + len(f.models))
	stored := provider.RemoteModel{ID: json.Number(ref), Reference: ref, Name: model.ModelName, ProviderName: row.ProviderName, InstanceID: row.ID, InstanceName: row.Name, ModelTypes: model.ModelTypes, MaxTokens: model.MaxTokens, EffectiveStatus: "active", Status: "active", IsTools: model.IsTools}
	f.models = append(f.models, stored)
	return stored
}
func (f *instanceSyncFixture) UpdateInstance(_ context.Context, factory, name string, in provider.InstanceCandidate) (*provider.RemoteInstance, error) {
	f.patches++
	if f.patchEntered != nil {
		close(f.patchEntered)
		<-f.patchContinue
	}
	if f.lostPatch {
		return nil, errors.New("unconfirmed patch")
	}
	for index, row := range f.instances {
		if row.Name == name {
			row.BaseURL = in.BaseURL
			row.Region = in.Region
			row.HasAPIKey = in.APIKey != ""
			f.instances[index] = row
			return &row, nil
		}
	}
	return nil, errors.New("not found")
}
func (f *instanceSyncFixture) ListInstanceModels(_ context.Context, _ string, name string) ([]provider.RemoteModel, error) {
	rows := []provider.RemoteModel{}
	for _, row := range f.models {
		if row.InstanceName == name {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (f *instanceSyncFixture) AddInstanceModel(_ context.Context, _ string, name string, in provider.ModelRegistration) (*provider.RemoteModel, error) {
	f.adds++
	for _, row := range f.instances {
		if row.Name == name {
			model := f.insertModel(row, in)
			return &model, nil
		}
	}
	return nil, errors.New("not found")
}

func setupInstanceSync(t *testing.T) *ProviderService {
	svc := setupProviderService(t, "fixture-key")
	require.NoError(t, database.DB.AutoMigrate(&provider.MultiRAGSyncBinding{}))
	require.NoError(t, database.DB.Create(&provider.ProviderModel{ProviderID: 1, SelectionID: "embed", ModelID: "embed", ModelType: "embedding", ContextWindow: 2048, Status: "1"}).Error)
	return svc
}

func TestInstanceSyncAtomicSelectionAndDurableIdentity(t *testing.T) {
	svc := setupInstanceSync(t)
	require.NoError(t, database.DB.Create(&provider.ProviderModel{ProviderID: 1, SelectionID: "vision", ModelID: "vision", ModelType: "vlm", Status: "1"}).Error)
	f := &instanceSyncFixture{}
	first, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	require.True(t, first.Success)
	require.Equal(t, "instances", first.Endpoint)
	require.Len(t, first.PerCall, 2)
	require.Equal(t, "9007199254740993", first.PerCall[0].Reference)
	require.Len(t, f.inputs, 1)
	require.Empty(t, f.inputs[0].ModelIDs)
	require.Len(t, f.inputs[0].CustomModels, 2)
	require.Equal(t, 0, f.verifies, "atomic creation verifies upstream; no duplicate paid probe")
	second, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, []string{"embed"})
	require.NoError(t, err)
	require.Equal(t, first.InstanceName, second.InstanceName)
	require.Equal(t, 1, f.creates)
	require.Zero(t, f.patches)
	require.Zero(t, f.adds)
	require.Empty(t, f.calls)
	var binding provider.MultiRAGSyncBinding
	require.NoError(t, database.DB.First(&binding).Error)
	require.Equal(t, "ready", binding.State)
	require.Equal(t, first.InstanceName, binding.InstanceName)
}

func TestInstanceSyncVerifyOnlyNeverCreatesBinding(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	result, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, true, []string{"embed"})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, f.verifies)
	require.Zero(t, f.creates)
	var count int64
	require.NoError(t, database.DB.Model(&provider.MultiRAGSyncBinding{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestInstanceSyncOCRUsesStringCredentialEnvelopeAndSelectedModel(t *testing.T) {
	svc := setupInstanceSync(t)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Updates(map[string]any{"protocol": "mineru", "base_url": "https://ocr.invalid"}).Error)
	require.NoError(t, database.DB.Model(&provider.ProviderModel{}).Where("provider_id = 1").Updates(map[string]any{"model_id": "ocr-one", "model_type": "ocr", "context_window": 0}).Error)
	require.NoError(t, svc.repo.SetAttributes("", 1, map[string]provider.AttrValue{"backend": {Type: "string", Value: "vlm-http-client"}}))
	f := &instanceSyncFixture{}
	result, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, []string{"ocr-one"})
	require.NoError(t, err)
	require.Equal(t, "MinerU", result.FactoryName)
	require.Len(t, f.inputs, 1)
	var credentials map[string]string
	require.NoError(t, json.Unmarshal([]byte(f.inputs[0].APIKey), &credentials))
	require.Equal(t, "https://ocr.invalid", credentials["mineru_apiserver"])
	require.Equal(t, "vlm-http-client", credentials["mineru_backend"])
	require.Empty(t, f.inputs[0].BaseURL)
	require.Equal(t, []string{"ocr"}, f.inputs[0].CustomModels[0].ModelTypes)
}

func TestInstanceSyncMergesCapabilitiesIntoOneModelRegistration(t *testing.T) {
	svc := setupInstanceSync(t)
	require.NoError(t, database.DB.Create(&provider.ProviderModel{ProviderID: 1, SelectionID: "same-chat", ModelID: "embed", ModelType: "llm", ContextWindow: 2048, Status: "1"}).Error)
	f := &instanceSyncFixture{}
	result, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, []string{"embed"})
	require.NoError(t, err)
	require.Len(t, result.PerCall, 1)
	require.Len(t, f.inputs[0].CustomModels, 1)
	require.ElementsMatch(t, []string{"chat", "embedding"}, f.inputs[0].CustomModels[0].ModelTypes)
}

func TestInstanceSyncLegacyFallbackRejectsCrossProviderNameCollision(t *testing.T) {
	svc := setupInstanceSync(t)
	require.NoError(t, database.DB.Create(&provider.ProviderSummary{ID: 2, Key: "other", Name: "Other", Protocol: "openai", AuthStyle: "api_key"}).Error)
	require.NoError(t, database.DB.Create(&provider.ProviderModel{ProviderID: 2, SelectionID: "same", ModelID: "embed", ModelType: "embedding", Status: "1"}).Error)
	f := &instanceSyncFixture{unavailable: true}
	_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, []string{"embed"})
	require.Error(t, err)
	require.Empty(t, f.calls)
}

func TestInstanceSyncLostCreateRecoveryNeverReplays(t *testing.T) {
	for _, visible := range []bool{true, false} {
		t.Run(fmt.Sprint(visible), func(t *testing.T) {
			svc := setupInstanceSync(t)
			f := &instanceSyncFixture{lostCreate: true, skipCreate: !visible}
			_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
			require.Error(t, err)
			// Simulate a later request/restart: the DB reservation is authoritative.
			restarted := NewProviderService(providerServiceTestEncryptionKey)
			result, err := restarted.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
			if visible {
				require.NoError(t, err)
				require.True(t, result.Success)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
			require.Equal(t, 1, f.creates)
		})
	}
}

func TestInstanceSyncPreservesExistingCapabilitiesAndTools(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	f.models[0].IsTools = true
	_, err = svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	require.True(t, f.models[0].IsTools)
	require.Zero(t, f.adds)
	f.models[0].ModelTypes = []string{"chat"}
	_, err = svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.Error(t, err)
	require.Equal(t, 1, f.creates)
	require.Zero(t, f.adds)
}

func TestInstanceSyncUpdatesOnlyExplicitlyBoundInstance(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	first, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	f.instances = append(f.instances, provider.RemoteInstance{ID: "unrelated", ProviderName: "OpenAI-API-Compatible", Name: "default", Status: "active", BaseURL: "https://foreign.invalid"})
	replacement, _ := provider.Encrypt("new-key", providerServiceTestEncryptionKey)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Updates(map[string]any{"base_url": "https://new.invalid/v1", "locked_api_key": replacement}).Error)
	result, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	require.Equal(t, first.InstanceName, result.InstanceName)
	require.Equal(t, 1, f.patches)
	require.Equal(t, 1, f.verifies)
	require.Equal(t, "https://foreign.invalid", f.instances[1].BaseURL)
}

func TestInstanceBindingCannotBeReadOrReservedAcrossTenants(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	store := svc.repo.(provider.MultiRAGBindingRepository)
	_, err = store.GetMultiRAGBinding("other", 1, f.SyncTarget())
	require.Error(t, err)
	_, err = store.ReserveMultiRAGBinding("other", &provider.MultiRAGSyncBinding{ProviderID: 1, Target: "other", InstanceName: "not-owned", State: "creating"})
	require.Error(t, err)
}

func TestInstanceSyncPersistentLeaseSerializesCredentialRotations(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	store := svc.repo.(provider.MultiRAGBindingRepository)
	stale, err := store.GetMultiRAGBinding("", 1, f.SyncTarget())
	require.NoError(t, err)
	keyA, _ := provider.Encrypt("key-A", providerServiceTestEncryptionKey)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Update("locked_api_key", keyA).Error)
	f.patchEntered = make(chan struct{})
	f.patchContinue = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil); done <- err }()
	<-f.patchEntered
	keyB, _ := provider.Encrypt("key-B", providerServiceTestEncryptionKey)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Update("locked_api_key", keyB).Error)
	// A different service instance cannot acquire the durable in-flight binding.
	other := NewProviderService(providerServiceTestEncryptionKey)
	_, err = other.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.Error(t, err)
	require.Error(t, store.ConfirmMultiRAGBinding("", stale), "stale completion must fail its version/state/lease CAS")
	close(f.patchContinue)
	require.ErrorContains(t, <-done, "变化")
	f.patchEntered = nil
	f.patchContinue = nil
	_, err = other.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	require.Equal(t, 2, f.patches)
	latest, err := store.GetMultiRAGBinding("", 1, f.SyncTarget())
	require.NoError(t, err)
	require.Equal(t, configHash(provider.InstanceCandidate{APIKey: "key-B"}), latest.ConfigHash)
	require.Equal(t, "ready", latest.State)
}

func TestInstanceSyncUnknownCredentialPatchCannotRestoreOldSuccess(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	store := svc.repo.(provider.MultiRAGBindingRepository)
	old, err := store.GetMultiRAGBinding("", 1, f.SyncTarget())
	require.NoError(t, err)
	key, _ := provider.Encrypt("new-key-same-base", providerServiceTestEncryptionKey)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Update("locked_api_key", key).Error)
	f.lostPatch = true
	_, err = svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.Error(t, err)
	require.Error(t, store.ConfirmMultiRAGBinding("", old))
	_, err = NewProviderService(providerServiceTestEncryptionKey).SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.Error(t, err)
	require.Equal(t, 1, f.patches, "no automatic replay of an unknown credential write")
	pending, err := store.GetMultiRAGBinding("", 1, f.SyncTarget())
	require.NoError(t, err)
	require.Equal(t, "syncing", pending.State)
	require.NotEqual(t, old.ConfigHash, pending.ConfigHash)
}

func TestInstanceSyncRejectsStaleSourceBeforeAcquiringNewerBinding(t *testing.T) {
	svc := setupInstanceSync(t)
	f := &instanceSyncFixture{}
	_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	keyA, _ := provider.Encrypt("key-A", providerServiceTestEncryptionKey)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Update("locked_api_key", keyA).Error)
	entered, proceed := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	f.beforeList = func() {
		if blocked.CompareAndSwap(false, true) {
			close(entered)
			<-proceed
		}
	}
	done := make(chan error, 1)
	go func() { _, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil); done <- err }()
	<-entered
	keyB, _ := provider.Encrypt("key-B", providerServiceTestEncryptionKey)
	require.NoError(t, database.DB.Model(&provider.ProviderSummary{}).Where("id = 1").Update("locked_api_key", keyB).Error)
	_, err = NewProviderService(providerServiceTestEncryptionKey).SyncProviderToMultiRAG(context.Background(), "", 1, f, false, nil)
	require.NoError(t, err)
	close(proceed)
	require.ErrorContains(t, <-done, "变化")
	require.Equal(t, 1, f.patches, "stale A cannot PATCH key-A over B's completed key-B")
	store := svc.repo.(provider.MultiRAGBindingRepository)
	latest, err := store.GetMultiRAGBinding("", 1, f.SyncTarget())
	require.NoError(t, err)
	require.Equal(t, "ready", latest.State)
	require.Equal(t, configHash(provider.InstanceCandidate{APIKey: "key-B"}), latest.ConfigHash)
}
