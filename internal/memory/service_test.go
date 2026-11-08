package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testCtx = context.Background()

type testIndex struct {
	mu       sync.Mutex
	docs     map[string]map[string]string
	puts     int
	unknown  bool
	entered  chan struct{}
	release  chan struct{}
	searches int
}

func newTestIndex() *testIndex { return &testIndex{docs: map[string]map[string]string{}} }
func (i *testIndex) EnsureDocument(_ context.Context, _ string, name string, allow bool) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.docs[name]; !ok {
		if !allow {
			return "", nil
		}
		i.docs[name] = map[string]string{}
	}
	return name, nil
}
func (i *testIndex) Put(_ context.Context, _ string, doc, content string) (string, error) {
	if i.entered != nil {
		close(i.entered)
		<-i.release
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.puts++
	if i.docs[doc] == nil {
		i.docs[doc] = map[string]string{}
	}
	id := digest(content)
	i.docs[doc][id] = content
	if i.unknown {
		i.unknown = false
		return "", ErrIndexUnknown
	}
	return id, nil
}
func (i *testIndex) Find(_ context.Context, _ string, doc, content string) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for id, c := range i.docs[doc] {
		if c == content {
			return id, nil
		}
	}
	return "", nil
}
func (i *testIndex) Remove(_ context.Context, _ string, doc, id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.docs[doc], id)
	return nil
}
func (i *testIndex) DropDocument(_ context.Context, _ string, doc string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.docs, doc)
	return nil
}
func (i *testIndex) Search(_ context.Context, _ string, doc, _ string, _ int) ([]Hit, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.searches++
	out := []Hit{}
	for id, c := range i.docs[doc] {
		out = append(out, Hit{ChunkID: id, DocumentID: doc, Content: c})
	}
	return out, nil
}
func setup(t *testing.T) (*Service, Scope, *testIndex) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/memory.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	require.NoError(t, Migrate(db))
	wrapper, e := NewSyntheticKeyWrapper(strings.Repeat("01", 32))
	require.NoError(t, e)
	index := newTestIndex()
	s, e := NewService(db, Config{TenantID: "tenant", ApplicationID: "app", PolicyVersion: "policy-1", DatasetID: "dataset", SyntheticOnly: true, Wrapper: wrapper, Index: index})
	require.NoError(t, e)
	sc := Scope{"tenant", "app", "alice", "life-1"}
	_, e = s.Activate(testCtx, sc, "activate")
	require.NoError(t, e)
	return s, sc, index
}
func input(content string) CandidateInput {
	return CandidateInput{DataClass: "synthetic", Source: "trustedJava", OperationID: uuid.NewString(), TurnID: uuid.NewString(), SourceMessageID: uuid.NewString(), UserText: content, Quote: content, Subject: "asQuoted", Temporal: "asQuoted", Category: "preference", ConsentRef: "consent-1", PolicyVersion: "policy-1"}
}
func publish(t *testing.T, s *Service, sc Scope, in CandidateInput) Fact {
	t.Helper()
	c, e := s.SubmitCandidate(testCtx, sc, in)
	require.NoError(t, e)
	task, e := s.ClaimTask(testCtx, sc, uuid.NewString())
	require.NoError(t, e)
	facts, e := s.Publish(testCtx, sc, PublishInput{OperationID: uuid.NewString(), TaskID: task.ID, Items: []PublishItem{{CandidateID: c.ID, ExpectedVersion: c.Version, Content: c.Quote}}})
	require.NoError(t, e)
	require.Len(t, facts, 1)
	return facts[0]
}
func syncAll(t *testing.T, s *Service) {
	t.Helper()
	for n := 0; n < 3; n++ {
		require.NoError(t, s.Sync(testCtx, 100))
	}
}

func TestScopeEncryptionIdempotencyAndCAS(t *testing.T) {
	s, sc, _ := setup(t)
	in := input("我喜欢雨后散步。")
	c, e := s.SubmitCandidate(testCtx, sc, in)
	require.NoError(t, e)
	again, e := s.SubmitCandidate(testCtx, sc, in)
	require.NoError(t, e)
	require.Equal(t, c.ID, again.ID)
	in.Quote = "我喜欢登山。"
	in.UserText = in.Quote
	_, e = s.SubmitCandidate(testCtx, sc, in)
	require.ErrorIs(t, e, ErrConflict)
	other := sc
	other.UserID = "bob"
	_, e = s.Get(testCtx, other, c.ID)
	require.ErrorIs(t, e, ErrNotFound)
	other = sc
	other.ApplicationID = "another"
	_, e = s.List(testCtx, other)
	require.ErrorIs(t, e, ErrForbidden)
	task, e := s.ClaimTask(testCtx, sc, "claim")
	require.NoError(t, e)
	facts, e := s.Publish(testCtx, sc, PublishInput{OperationID: "publish", TaskID: task.ID, Items: []PublishItem{{c.ID, 1, c.Quote}}})
	require.NoError(t, e)
	f := facts[0]
	results := make(chan error, 2)
	for _, text := range []string{"我喜欢登山。", "我喜欢游泳。"} {
		go func(content string) { _, e := s.Update(testCtx, sc, f.ID, 1, content, uuid.NewString()); results <- e }(text)
	}
	a, b := <-results, <-results
	require.True(t, (a == nil && errors.Is(b, ErrConflict)) || (b == nil && errors.Is(a, ErrConflict)))
	var records []recordRow
	require.NoError(t, s.db.Find(&records).Error)
	for _, r := range records {
		require.NotContains(t, string(r.Payload), "喜欢")
	}
	var ops []operationRow
	require.NoError(t, s.db.Find(&ops).Error)
	for _, op := range ops {
		require.NotContains(t, string(op.Result), "喜欢")
	}
	l, e := s.life(s.db, sc, false)
	require.NoError(t, e)
	var r recordRow
	require.NoError(t, s.db.First(&r, "id = ?", f.ID).Error)
	var decoded value
	require.Error(t, s.decode(testCtx, l, "wrong-record", r.Payload, &decoded))
}
func TestCandidateAdmissionAndAtomicPublication(t *testing.T) {
	s, sc, _ := setup(t)
	for _, mutate := range []func(*CandidateInput){func(i *CandidateInput) { i.UserText = "我的朋友说：" + i.Quote }, func(i *CandidateInput) { i.Risk = true }, func(i *CandidateInput) { i.Sensitive = true }, func(i *CandidateInput) { i.PolicyVersion = "wrong" }, func(i *CandidateInput) { i.ConsentRef = "" }, func(i *CandidateInput) { i.Source = "app" }, func(i *CandidateInput) { i.Subject = "inferred" }} {
		in := input("我喜欢散步。")
		mutate(&in)
		_, e := s.SubmitCandidate(testCtx, sc, in)
		require.ErrorIs(t, e, ErrInvalid)
	}
	in := input("我过去喜欢游泳，现在喜欢散步。")
	c, e := s.SubmitCandidate(testCtx, sc, in)
	require.NoError(t, e)
	task, e := s.ClaimTask(testCtx, sc, "claim")
	require.NoError(t, e)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "bad", TaskID: task.ID, Items: []PublishItem{{c.ID, 1, "我喜欢游泳。"}}})
	require.ErrorIs(t, e, ErrInvalid)
	facts, e := s.List(testCtx, sc)
	require.NoError(t, e)
	require.Empty(t, facts)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "good", TaskID: task.ID, Items: []PublishItem{{c.ID, 1, c.Quote}}})
	require.NoError(t, e)
}
func TestLateWriteDeletionDoesNotReviveAndInvalidatesContext(t *testing.T) {
	s, sc, index := setup(t)
	f := publish(t, s, sc, input("我喜欢绿色。"))
	index.entered = make(chan struct{})
	index.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- s.Sync(testCtx, 100) }()
	<-index.entered
	l, e := s.life(s.db, sc, false)
	require.NoError(t, e)
	oldVersion := l.Revision
	op, e := s.Delete(testCtx, sc, f.ID, 1, "delete")
	require.NoError(t, e)
	require.True(t, op.StoppedSupply)
	valid, e := s.Validate(testCtx, sc, oldVersion)
	require.NoError(t, e)
	require.False(t, valid)
	close(index.release)
	require.NoError(t, <-done)
	index.entered = nil
	syncAll(t, s)
	facts, e := s.List(testCtx, sc)
	require.NoError(t, e)
	require.Empty(t, facts)
	got, e := s.Recall(testCtx, sc, "颜色")
	require.NoError(t, e)
	require.Empty(t, got.Facts)
	index.mu.Lock()
	for _, chunks := range index.docs {
		require.Empty(t, chunks)
	}
	index.mu.Unlock()
	receipt, e := s.GetOperation(testCtx, sc, "delete")
	require.NoError(t, e)
	require.True(t, receipt.PhysicalCleanup)
	// A fresh source cannot reconstruct a deleted fact from old conversation.
	_, e = s.SubmitCandidate(testCtx, sc, input(f.Content))
	require.ErrorIs(t, e, ErrConflict)
	changedCategory := input(f.Content)
	changedCategory.Category = "situation"
	_, e = s.SubmitCandidate(testCtx, sc, changedCategory)
	require.ErrorIs(t, e, ErrConflict)
}
func TestUnknownWriteReadOnlyRecoverySurvivesRestart(t *testing.T) {
	s, sc, index := setup(t)
	f := publish(t, s, sc, input("我喜欢蓝色。"))
	index.unknown = true
	require.ErrorIs(t, s.Sync(testCtx, 100), ErrIndexUnknown)
	require.Equal(t, 1, index.puts)
	restart, e := NewService(s.db, s.cfg)
	require.NoError(t, e)
	syncAll(t, restart)
	require.Equal(t, 1, index.puts)
	got, e := restart.Get(testCtx, sc, f.ID)
	require.NoError(t, e)
	require.Equal(t, "confirmed", got.SyncStatus)
	context, e := restart.Recall(testCtx, sc, "颜色")
	require.NoError(t, e)
	require.Len(t, context.Facts, 1)
}
func TestCorrectionRevocationAndReregistration(t *testing.T) {
	s, sc, _ := setup(t)
	old := publish(t, s, sc, input("我以前喜欢红色。"))
	syncAll(t, s)
	stale, e := s.SubmitCandidate(testCtx, sc, input(old.Content))
	require.NoError(t, e)
	correction := input("我现在喜欢蓝色。")
	correction.ReplaceFactID = old.ID
	correction.ExpectedVersion = 1
	c, e := s.SubmitCandidate(testCtx, sc, correction)
	require.NoError(t, e)
	_, e = s.Get(testCtx, sc, old.ID)
	require.ErrorIs(t, e, ErrNotFound)
	task, e := s.ClaimTask(testCtx, sc, "correct")
	require.NoError(t, e)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "stale", TaskID: task.ID, Items: []PublishItem{{stale.ID, 1, stale.Quote}}})
	require.ErrorIs(t, e, ErrConflict)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "correction", TaskID: task.ID, Items: []PublishItem{{c.ID, 1, c.Quote}}})
	require.NoError(t, e)
	syncAll(t, s)
	_, e = s.Revoke(testCtx, sc, "consent-1", "revoke")
	require.NoError(t, e)
	_, e = s.SubmitCandidate(testCtx, sc, input("我喜欢咖啡。"))
	require.ErrorIs(t, e, ErrForbidden)
	syncAll(t, s)
	facts, e := s.List(testCtx, sc)
	require.NoError(t, e)
	require.Empty(t, facts)
	receipt, e := s.Deactivate(testCtx, sc, "close")
	require.NoError(t, e)
	require.True(t, receipt.StoppedSupply)
	next := sc
	next.LifecycleID = "life-2"
	_, e = s.Activate(testCtx, next, "new-account")
	require.NoError(t, e)
	_, e = s.Activate(testCtx, sc, "reuse")
	require.ErrorIs(t, e, ErrConflict)
	syncAll(t, s)
	receipt, e = s.GetOperation(testCtx, sc, "close")
	require.NoError(t, e)
	require.True(t, receipt.PhysicalCleanup)
	facts, e = s.List(testCtx, next)
	require.NoError(t, e)
	require.Empty(t, facts)
	_, e = s.SubmitCandidate(testCtx, sc, input("我喜欢咖啡。"))
	require.ErrorIs(t, e, ErrForbidden)
	var l lifecycleRow
	require.NoError(t, s.db.First(&l, "id = ?", scopeKey(sc)).Error)
	require.Empty(t, l.WrappedKey)
	var count int64
	require.NoError(t, s.db.Model(&recordRow{}).Where("scope_key = ?", l.ID).Count(&count).Error)
	require.Zero(t, count)
}
func TestSharedChunkReferenceAndAppWorkerScope(t *testing.T) {
	s, sc, index := setup(t)
	a := publish(t, s, sc, input("我喜欢早睡。"))
	in := input(a.Content)
	in.Category = "situation"
	b := publish(t, s, sc, in)
	cfg := s.cfg
	cfg.ApplicationID = "other"
	cfg.DatasetID = "other-dataset"
	other, e := NewService(s.db, cfg)
	require.NoError(t, e)
	require.NoError(t, other.Sync(testCtx, 100))
	require.Zero(t, index.puts)
	syncAll(t, s)
	_, e = s.Delete(testCtx, sc, a.ID, 1, "delete-a")
	require.NoError(t, e)
	syncAll(t, s)
	got, e := s.Recall(testCtx, sc, "作息")
	require.NoError(t, e)
	require.Len(t, got.Facts, 1)
	require.Equal(t, b.ID, got.Facts[0].ID)
	_, e = s.Delete(testCtx, sc, b.ID, 1, "delete-b")
	require.NoError(t, e)
	syncAll(t, s)
	got, e = s.Recall(testCtx, sc, "作息")
	require.NoError(t, e)
	require.Empty(t, got.Facts)
}

func TestEmptyClaimDoesNotBlockLaterCandidatesAndOperationReceipts(t *testing.T) {
	s, sc, _ := setup(t)
	task, e := s.ClaimTask(testCtx, sc, "empty")
	require.NoError(t, e)
	require.Empty(t, task.ID)
	require.Empty(t, task.Candidates)
	c, e := s.SubmitCandidate(testCtx, sc, input("我喜欢周末看书。"))
	require.NoError(t, e)
	task, e = s.ClaimTask(testCtx, sc, "later")
	require.NoError(t, e)
	require.Len(t, task.Candidates, 1)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "published", TaskID: task.ID, Items: []PublishItem{{c.ID, 1, c.Quote}}})
	require.NoError(t, e)
	op, e := s.GetOperation(testCtx, sc, "published")
	require.NoError(t, e)
	require.Equal(t, "completed", op.Status)
}

func TestContextBudgetCategoryAndProfileProvenance(t *testing.T) {
	s, sc, _ := setup(t)
	facts := []Fact{}
	for n := 0; n < 6; n++ {
		in := input(string(rune('A'+n)) + strings.Repeat("合", 39))
		if n == 5 {
			in.Category = "event"
		}
		facts = append(facts, publish(t, s, sc, in))
	}
	syncAll(t, s)
	got, e := s.RecallCategories(testCtx, sc, "测试", []string{"event"})
	require.NoError(t, e)
	require.Len(t, got.Facts, 1)
	require.Equal(t, facts[5].ID, got.Facts[0].ID)
	got, e = s.Recall(testCtx, sc, "测试")
	require.NoError(t, e)
	require.LessOrEqual(t, len(got.Facts), 5)
	total := 0
	for _, f := range got.Facts {
		total += len([]rune(f.Content))
	}
	require.LessOrEqual(t, total, 200)
	_, e = s.SubmitCandidate(testCtx, sc, input("我喜欢晴天。"))
	require.NoError(t, e)
	task, e := s.ClaimTask(testCtx, sc, "profile-task")
	require.NoError(t, e)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "invented-profile", TaskID: task.ID, Profile: "用户有未被证实的喜好", ProfileFactIDs: []string{facts[0].ID}})
	require.ErrorIs(t, e, ErrInvalid)
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "valid-profile", TaskID: task.ID, Profile: facts[0].Content, ProfileFactIDs: []string{facts[0].ID}})
	require.NoError(t, e)
	got, e = s.Recall(testCtx, sc, "测试")
	require.NoError(t, e)
	require.Equal(t, facts[0].Content, got.Profile)
	got, e = s.RecallCategories(testCtx, sc, "测试", []string{"event"})
	require.NoError(t, e)
	require.Empty(t, got.Profile)
	_, e = s.Delete(testCtx, sc, facts[0].ID, 1, "clear-profile")
	require.NoError(t, e)
	got, e = s.Recall(testCtx, sc, "测试")
	require.NoError(t, e)
	require.Empty(t, got.Profile)
}
func TestUnknownOldVersionReconcilesEncryptedSnapshot(t *testing.T) {
	s, sc, index := setup(t)
	f := publish(t, s, sc, input("我喜欢蓝色。"))
	index.unknown = true
	require.ErrorIs(t, s.Sync(testCtx, 100), ErrIndexUnknown)
	next, e := s.Update(testCtx, sc, f.ID, 1, "我喜欢绿色。", "edit")
	require.NoError(t, e)
	syncAll(t, s)
	got, e := s.Get(testCtx, sc, f.ID)
	require.NoError(t, e)
	require.Equal(t, next.Content, got.Content)
	require.Equal(t, "confirmed", got.SyncStatus)
	require.Equal(t, 2, index.puts)
	index.mu.Lock()
	defer index.mu.Unlock()
	for _, chunks := range index.docs {
		require.Len(t, chunks, 1)
		for _, content := range chunks {
			require.Equal(t, next.Content, content)
		}
	}
}

type absentUnknownIndex struct{ *testIndex }

func (i *absentUnknownIndex) Put(context.Context, string, string, string) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.puts++
	return "", ErrIndexUnknown
}
func TestUnknownAbsentOutcomeDoesNotReplayWrite(t *testing.T) {
	s, sc, index := setup(t)
	s.cfg.Index = &absentUnknownIndex{index}
	publish(t, s, sc, input("我喜欢阅读。"))
	require.ErrorIs(t, s.Sync(testCtx, 100), ErrIndexUnknown)
	for n := 0; n < 3; n++ {
		require.ErrorIs(t, s.Sync(testCtx, 100), ErrIndexUnknown)
	}
	require.Equal(t, 1, index.puts)
}
func TestLegalBatchSnapshotsExceedSmallBlobAndRemainEncrypted(t *testing.T) {
	s, sc, _ := setup(t)
	for n := 0; n < 100; n++ {
		in := input("我" + strings.Repeat("合", 199))
		in.SourceMessageID = uuid.NewString() + strings.Repeat("m", 90)
		in.TurnID = uuid.NewString() + strings.Repeat("t", 90)
		_, e := s.SubmitCandidate(testCtx, sc, in)
		require.NoError(t, e)
	}
	task, e := s.ClaimTask(testCtx, sc, "large-task")
	require.NoError(t, e)
	require.Len(t, task.Candidates, 100)
	var stored taskRow
	require.NoError(t, s.db.First(&stored, "id = ?", task.ID).Error)
	require.Greater(t, len(stored.Payload), 65535)
	items := []PublishItem{}
	for _, c := range task.Candidates {
		items = append(items, PublishItem{c.ID, c.Version, c.Quote})
	}
	_, e = s.Publish(testCtx, sc, PublishInput{OperationID: "large-publish", TaskID: task.ID, Items: items})
	require.NoError(t, e)
	var op operationRow
	require.NoError(t, s.db.First(&op, "id = ?", digest([]string{scopeKey(sc), "large-publish"})).Error)
	require.Greater(t, len(op.Result), 65535)
}

func TestUnknownJobDoesNotStarveAnotherUser(t *testing.T) {
	s, sc, index := setup(t)
	publish(t, s, sc, input("我喜欢蓝色。"))
	other := sc
	other.UserID = "bob"
	_, e := s.Activate(testCtx, other, "activate-bob")
	require.NoError(t, e)
	b := publish(t, s, other, input("我喜欢红色。"))
	index.unknown = true
	require.ErrorIs(t, s.Sync(testCtx, 1), ErrIndexUnknown)
	require.NoError(t, s.Sync(testCtx, 1))
	got, e := s.Get(testCtx, other, b.ID)
	require.NoError(t, e)
	require.Equal(t, "confirmed", got.SyncStatus)
}

type recallLockIndex struct {
	*testIndex
	hold func()
}

func (i *recallLockIndex) Search(ctx context.Context, dataset, doc, query string, limit int) ([]Hit, error) {
	hits, err := i.testIndex.Search(ctx, dataset, doc, query, limit)
	i.hold()
	return hits, err
}
func TestRecallDeadlineInterruptsBothApplicationLockWaits(t *testing.T) {
	for _, phase := range []string{"before-snapshot", "after-search"} {
		t.Run(phase, func(t *testing.T) {
			s, sc, index := setup(t)
			publish(t, s, sc, input("我喜欢看书。"))
			syncAll(t, s)
			held := false
			defer func() {
				if held {
					s.mu.Unlock()
				}
			}()
			if phase == "before-snapshot" {
				s.mu.Lock()
				held = true
			} else {
				s.cfg.Index = &recallLockIndex{testIndex: index, hold: func() { s.mu.Lock(); held = true }}
			}
			ctx, cancel := context.WithTimeout(testCtx, 10*time.Millisecond)
			defer cancel()
			start := time.Now()
			out, err := s.RecallCategories(ctx, sc, "阅读", nil)
			elapsed := time.Since(start)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Less(t, elapsed, 200*time.Millisecond)
			require.Empty(t, out.Status)
			require.Empty(t, out.Facts)
		})
	}
}
func TestCleanupReceiptsDistinguishReplicaFromRetainedLedger(t *testing.T) {
	s, sc, _ := setup(t)
	f := publish(t, s, sc, input("我喜欢蓝色。"))
	syncAll(t, s)
	created, e := s.Delete(testCtx, sc, f.ID, 1, "delete-scoped")
	require.NoError(t, e)
	require.Equal(t, "retrievalReplica", created.CleanupScope)
	require.True(t, created.LedgerRetained)
	syncAll(t, s)
	receipt, e := s.GetOperation(testCtx, sc, "delete-scoped")
	require.NoError(t, e)
	require.True(t, receipt.PhysicalCleanup)
	require.Equal(t, "retrievalReplica", receipt.CleanupScope)
	require.True(t, receipt.LedgerRetained)
	var count int64
	require.NoError(t, s.db.Model(&recordRow{}).Where("scope_key = ?", scopeKey(sc)).Count(&count).Error)
	require.Positive(t, count, "ordinary deletion retains encrypted ledger evidence")
	in := input("我喜欢晴天。")
	in.ConsentRef = "consent-2"
	publish(t, s, sc, in)
	syncAll(t, s)
	created, e = s.Revoke(testCtx, sc, "consent-2", "revoke-scoped")
	require.NoError(t, e)
	require.Equal(t, "retrievalReplica", created.CleanupScope)
	require.True(t, created.LedgerRetained)
	syncAll(t, s)
	receipt, e = s.GetOperation(testCtx, sc, "revoke-scoped")
	require.NoError(t, e)
	require.True(t, receipt.PhysicalCleanup)
	require.Equal(t, "retrievalReplica", receipt.CleanupScope)
	require.True(t, receipt.LedgerRetained)
	created, e = s.Deactivate(testCtx, sc, "close-scoped")
	require.NoError(t, e)
	require.Equal(t, "retrievalReplica", created.CleanupScope)
	require.True(t, created.LedgerRetained)
	syncAll(t, s)
	receipt, e = s.GetOperation(testCtx, sc, "close-scoped")
	require.NoError(t, e)
	require.Equal(t, "hubAndRetrievalReplica", receipt.CleanupScope)
	require.False(t, receipt.LedgerRetained)
	require.True(t, receipt.PhysicalCleanup)
	replay, e := s.Deactivate(testCtx, sc, "close-scoped")
	require.NoError(t, e)
	require.Equal(t, receipt, replay)
}

func TestRejectedCandidateIsConsumedAndReleasesTaskSnapshot(t *testing.T) {
	s, sc, _ := setup(t)
	rejected, err := s.SubmitCandidate(testCtx, sc, input("我刚刚喝了一杯水。"))
	require.NoError(t, err)
	remaining, err := s.SubmitCandidate(testCtx, sc, input("我喜欢阅读。"))
	require.NoError(t, err)
	oldTask, err := s.ClaimTask(testCtx, sc, "before-reject")
	require.NoError(t, err)
	require.Len(t, oldTask.Candidates, 2)
	result, err := s.RejectCandidate(testCtx, sc, rejected.ID, 1, "notDurable", "reject-1")
	require.NoError(t, err)
	require.Equal(t, "rejected", result.Status)
	require.Equal(t, "notDurable", result.RejectionReason)
	require.Equal(t, uint64(2), result.Version)
	replay, err := s.RejectCandidate(testCtx, sc, rejected.ID, 1, "notDurable", "reject-1")
	require.NoError(t, err)
	require.Equal(t, result, replay)
	_, err = s.RejectCandidate(testCtx, sc, rejected.ID, 1, "policy", "reject-1")
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.RejectCandidate(testCtx, sc, rejected.ID, 1, "notDurable", "stale-reject")
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.Publish(testCtx, sc, PublishInput{OperationID: "old-snapshot", TaskID: oldTask.ID, Items: []PublishItem{{remaining.ID, 1, remaining.Quote}}})
	require.ErrorIs(t, err, ErrConflict)
	next, err := s.ClaimTask(testCtx, sc, "after-reject")
	require.NoError(t, err)
	require.NotEqual(t, oldTask.ID, next.ID)
	require.Len(t, next.Candidates, 1)
	require.Equal(t, remaining.ID, next.Candidates[0].ID)
	_, err = s.RejectCandidate(testCtx, sc, remaining.ID, 1, "unsupported", "reject-2")
	require.NoError(t, err)
	empty, err := s.ClaimTask(testCtx, sc, "all-rejected")
	require.NoError(t, err)
	require.Empty(t, empty.ID)
	require.Empty(t, empty.Candidates)
	var jobs int64
	require.NoError(t, s.db.Model(&outboxRow{}).Count(&jobs).Error)
	require.Zero(t, jobs)
}
