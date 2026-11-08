package memory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type managedConversationFake struct {
	*nativeConversationFake
	*testIndex
	configReads   int
	statusWrites  int
	messageID     string
	enabled       bool
	expected      bool
	diagnostics   int
	searchOptions NativeSearchOptions
}

func (f *managedConversationFake) BankInfo(context.Context) (NativeBankInfo, error) {
	f.configReads++
	temperature := float64(0)
	capacity := int64(5242880)
	return NativeBankInfo{Name: "Private native memory", MemoryTypes: []string{"raw", "semantic", "episodic", "procedural"}, Temperature: &temperature, CapacityBytes: &capacity}, nil
}

func (f *managedConversationFake) SetMessageStatus(_ context.Context, scope, id string, enabled, expected bool) (NativeMessage, error) {
	f.statusWrites++
	f.scope, f.messageID, f.enabled, f.expected = scope, id, enabled, expected
	return NativeMessage{ID: id, Type: "raw", Content: "Synthetic actual dialogue", NativeStatus: enabled, Status: enabled, StatusChangeAllowed: true}, nil
}

func (f *managedConversationFake) SearchDiagnostic(_ context.Context, scope, _ string, options NativeSearchOptions) ([]NativeMessage, error) {
	f.diagnostics++
	f.scope, f.searchOptions = scope, options
	return f.hits, nil
}

func managedConversationFixture(t *testing.T) (*Service, Scope, *managedConversationFake) {
	s, sc, native := nativeConversationFixture(t)
	manager := &managedConversationFake{nativeConversationFake: native, testIndex: newTestIndex()}
	s.cfg.Native, s.cfg.Index = manager, manager
	return s, sc, manager
}

func TestNativeManagementCapabilitiesAndExactFalseStatus(t *testing.T) {
	s, sc, manager := managedConversationFixture(t)
	require.True(t, s.Capabilities().NativeConfigurationSupported)
	require.True(t, s.Capabilities().NativeStatusSupported)
	info, err := s.NativeConfiguration(testCtx)
	require.NoError(t, err)
	raw, err := json.Marshal(info)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"temperature":0`)
	require.Equal(t, int64(5242880), *info.CapacityBytes)
	result, err := s.SetNativeMessageStatus(testCtx, sc, "9223372036854775806", false, true)
	require.NoError(t, err)
	require.False(t, result.Status)
	require.False(t, manager.enabled)
	require.True(t, manager.expected)
	require.Equal(t, "9223372036854775806", manager.messageID)
	require.Equal(t, "memory-"+scopeKey(sc), manager.scope)
}

func TestNativeManagementInvalidOrInactiveScopeHasNoProviderEffects(t *testing.T) {
	s, sc, manager := managedConversationFixture(t)
	for _, id := range []string{"", "0", "01", "+1", "1/other", "9223372036854775808"} {
		_, err := s.SetNativeMessageStatus(testCtx, sc, id, false, true)
		require.ErrorIs(t, err, ErrInvalid)
	}
	for _, field := range []string{"tenant", "app", "user", "lifecycle"} {
		other := sc
		switch field {
		case "tenant":
			other.TenantID = "other"
		case "app":
			other.ApplicationID = "other"
		case "user":
			other.UserID = "other"
		case "lifecycle":
			other.LifecycleID = "other"
		}
		_, err := s.SetNativeMessageStatus(testCtx, other, "20", false, true)
		require.Error(t, err)
	}
	require.NoError(t, s.db.Transaction(func(tx *gorm.DB) error {
		life, err := s.life(tx, sc, false)
		if err != nil {
			return err
		}
		return tx.Model(life).Update("state", "frozen").Error
	}))
	_, err := s.SetNativeMessageStatus(testCtx, sc, "20", false, true)
	require.ErrorIs(t, err, ErrForbidden)
	require.Zero(t, manager.statusWrites)
	require.Zero(t, manager.configReads)
}

func TestNativeRawLedgerAndKnowledgeCannotUseNativeStatusMutation(t *testing.T) {
	s, sc, manager := managedConversationFixture(t)
	s.cfg.Native = nil
	require.True(t, s.Capabilities().NativeConfigurationSupported)
	require.False(t, s.Capabilities().NativeStatusSupported)
	_, err := s.NativeConfiguration(testCtx)
	require.NoError(t, err)
	_, err = s.SetNativeMessageStatus(testCtx, sc, "20", false, true)
	require.ErrorIs(t, err, ErrUnsupported)
	s.cfg.Backend = BackendKnowledge
	_, err = s.NativeConfiguration(testCtx)
	require.ErrorIs(t, err, ErrUnsupported)
	require.False(t, s.Capabilities().NativeConfigurationSupported)
	require.Zero(t, manager.statusWrites)
}

func TestNativeDiagnosticsPreservesZeroParametersAndDoesNotAddRecentRaw(t *testing.T) {
	s, sc, manager := managedConversationFixture(t)
	manager.listing = []NativeMessage{{ID: "10", Type: "raw", Status: true, Content: "Recent conversation"}}
	manager.hits = []NativeMessage{{ID: "11", SourceID: "10", Type: "semantic", Status: true, Content: "Actual provider hit"}}
	opts := NativeSearchOptions{Limit: 5, SimilarityThreshold: 0, KeywordWeight: 0}
	got, err := s.DiagnoseNativeSearch(testCtx, sc, "query", opts)
	require.NoError(t, err)
	require.Equal(t, opts, manager.searchOptions)
	require.Equal(t, opts, got.Options)
	require.Equal(t, manager.hits, got.Messages)
	require.Zero(t, manager.reads)
	require.True(t, s.Capabilities().NativeDiagnosticsSupported)
	other := sc
	other.UserID = "other-user"
	_, err = s.DiagnoseNativeSearch(testCtx, other, "query", opts)
	require.Error(t, err)
	for _, invalid := range []NativeSearchOptions{{Limit: 0}, {Limit: 21}, {Limit: 5, SimilarityThreshold: -0.1}, {Limit: 5, KeywordWeight: 1.1}} {
		_, err = s.DiagnoseNativeSearch(testCtx, sc, "query", invalid)
		require.ErrorIs(t, err, ErrInvalid)
	}
	require.Equal(t, 1, manager.diagnostics)
}
