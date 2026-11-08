package memory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type logicalNativeIndex struct {
	*testIndex
	disabled      map[string]map[string]bool
	disabledCalls int
	dropCalls     int
}

func newLogicalNativeIndex() *logicalNativeIndex {
	return &logicalNativeIndex{testIndex: newTestIndex(), disabled: map[string]map[string]bool{}}
}
func (i *logicalNativeIndex) Remove(_ context.Context, _ string, doc, id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.disabled[doc] == nil {
		i.disabled[doc] = map[string]bool{}
	}
	i.disabled[doc][id] = true
	i.disabledCalls++
	return nil
}
func (i *logicalNativeIndex) DropDocument(context.Context, string, string) error {
	i.dropCalls++
	return ErrUnsupported
}
func (i *logicalNativeIndex) Search(_ context.Context, _ string, doc, _ string, _ int) ([]Hit, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := []Hit{}
	for id, content := range i.docs[doc] {
		if !i.disabled[doc][id] {
			out = append(out, Hit{ChunkID: id, DocumentID: doc, Content: content})
		}
	}
	return out, nil
}
func nativeFixture(t *testing.T) (*Service, Scope, *logicalNativeIndex) {
	t.Helper()
	legacy, _, _ := setup(t)
	index := newLogicalNativeIndex()
	cfg := legacy.cfg
	cfg.Backend = BackendNative
	cfg.ApplicationID = "native-app"
	cfg.DatasetID = "native-bank"
	cfg.Index = index
	s, err := NewService(legacy.db, cfg)
	require.NoError(t, err)
	sc := Scope{TenantID: cfg.TenantID, ApplicationID: cfg.ApplicationID, UserID: "alice", LifecycleID: "native-life"}
	_, err = s.Activate(testCtx, sc, "activate-native")
	require.NoError(t, err)
	return s, sc, index
}
func ledgerSnapshot(t *testing.T, s *Service) string {
	t.Helper()
	snapshot := map[string][]map[string]any{}
	for _, table := range []string{"memory_users", "memory_lifecycles", "memory_records", "memory_operations", "memory_tasks", "memory_outbox"} {
		var rows []map[string]any
		require.NoError(t, s.db.Table(table).Order("id").Find(&rows).Error)
		snapshot[table] = rows
	}
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	return string(raw)
}
func TestNativeUnsupportedActionsHaveNoLedgerOrOutboxEffects(t *testing.T) {
	s, sc, index := nativeFixture(t)
	f := publish(t, s, sc, input("我喜欢乌龙茶。"))
	syncAll(t, s)
	before := ledgerSnapshot(t, s)
	for _, action := range []func() (Operation, error){func() (Operation, error) { return s.Delete(testCtx, sc, f.ID, 1, "not-delete") }, func() (Operation, error) { return s.Revoke(testCtx, sc, "consent-1", "not-revoke") }, func() (Operation, error) { return s.Deactivate(testCtx, sc, "not-deactivate") }} {
		receipt, err := action()
		require.ErrorIs(t, err, ErrUnsupported)
		require.Equal(t, Operation{}, receipt)
		require.Equal(t, before, ledgerSnapshot(t, s))
	}
	require.Zero(t, index.disabledCalls)
	require.Zero(t, index.dropCalls)
	got, err := s.Get(testCtx, sc, f.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(1), got.Version)
	caps := s.Capabilities()
	require.Equal(t, BackendNative, caps.Backend)
	require.True(t, caps.RawOnly)
	require.True(t, caps.CorrectionSupported)
	require.False(t, caps.ForgetSupported)
	require.False(t, caps.PhysicalDeleteSupported)
	require.False(t, caps.ConsentRevokeSupported)
	require.False(t, caps.DeactivateSupported)
}
func TestNativeCorrectionsDisableOldRawRecordsWithoutPhysicalDeletion(t *testing.T) {
	s, sc, index := nativeFixture(t)
	old := publish(t, s, sc, input("我喜欢乌龙茶。"))
	syncAll(t, s)
	contextBefore, err := s.Recall(testCtx, sc, "喜好")
	require.NoError(t, err)
	require.Len(t, contextBefore.Facts, 1)
	updated, err := s.Update(testCtx, sc, old.ID, 1, "我现在喜欢黑咖啡。", "correct-edit")
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Version)
	valid, err := s.Validate(testCtx, sc, contextBefore.Version)
	require.NoError(t, err)
	require.False(t, valid)
	syncAll(t, s)
	var life lifecycleRow
	require.NoError(t, s.db.First(&life, "id = ?", scopeKey(sc)).Error)
	require.Equal(t, old.Content, index.docs[life.DocumentID][digest(old.Content)])
	require.True(t, index.disabled[life.DocumentID][digest(old.Content)])
	require.Equal(t, 1, index.disabledCalls)
	now, err := s.Recall(testCtx, sc, "喜好")
	require.NoError(t, err)
	require.Len(t, now.Facts, 1)
	require.Equal(t, updated.Content, now.Facts[0].Content)
	correction := input("我现在更喜欢茉莉花茶。")
	correction.ReplaceFactID = updated.ID
	correction.ExpectedVersion = 2
	candidate, err := s.SubmitCandidate(testCtx, sc, correction)
	require.NoError(t, err)
	_, err = s.Get(testCtx, sc, updated.ID)
	require.ErrorIs(t, err, ErrNotFound)
	task, err := s.ClaimTask(testCtx, sc, "claim-replacement")
	require.NoError(t, err)
	facts, err := s.Publish(testCtx, sc, PublishInput{OperationID: "publish-replacement", TaskID: task.ID, Items: []PublishItem{{CandidateID: candidate.ID, ExpectedVersion: 1, Content: candidate.Quote}}})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	syncAll(t, s)
	require.True(t, index.disabled[life.DocumentID][digest(updated.Content)])
	require.Equal(t, updated.Content, index.docs[life.DocumentID][digest(updated.Content)])
	require.Zero(t, index.dropCalls)
	now, err = s.Recall(testCtx, sc, "喜好")
	require.NoError(t, err)
	require.Len(t, now.Facts, 1)
	require.Equal(t, correction.Quote, now.Facts[0].Content)
}
func TestNativeBackendAndBankDoNotReuseLegacyLifecycleOrOutbox(t *testing.T) {
	legacy, sc, _ := setup(t)
	publish(t, legacy, sc, input("我喜欢散步。"))
	cfg := legacy.cfg
	index := newLogicalNativeIndex()
	cfg.Backend = BackendNative
	cfg.DatasetID = "native-bank"
	cfg.Index = index
	native, err := NewService(legacy.db, cfg)
	require.NoError(t, err)
	before := ledgerSnapshot(t, legacy)
	_, err = native.Activate(testCtx, sc, "activate")
	require.ErrorIs(t, err, ErrConflict)
	_, err = native.List(testCtx, sc)
	require.ErrorIs(t, err, ErrConflict)
	subjects, err := native.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Empty(t, subjects.Subjects)
	require.NoError(t, native.Sync(testCtx, 100))
	require.Equal(t, before, ledgerSnapshot(t, legacy))
	require.Zero(t, index.puts)
	native, sc, index = nativeFixture(t)
	publish(t, native, sc, input("我喜欢阅读。"))
	cfg = native.cfg
	cfg.DatasetID = "changed-bank"
	changed, err := NewService(native.db, cfg)
	require.NoError(t, err)
	before = ledgerSnapshot(t, native)
	_, err = changed.GetOperation(testCtx, sc, "activate-native")
	require.ErrorIs(t, err, ErrConflict)
	require.NoError(t, changed.Sync(testCtx, 100))
	require.Equal(t, before, ledgerSnapshot(t, native))
	require.Zero(t, index.puts)
}
