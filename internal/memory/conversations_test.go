package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type nativeConversationFake struct {
	adds     int
	finds    int
	reads    int
	searches int
	received NativeTurn
	scope    string
	result   NativeIngestResult
	err      error
	listing  []NativeMessage
	hits     []NativeMessage
}

func (f *nativeConversationFake) AddConversation(_ context.Context, scope string, in NativeTurn) (NativeIngestResult, error) {
	f.adds++
	f.received = in
	f.scope = scope
	return f.result, f.err
}
func (f *nativeConversationFake) FindConversation(_ context.Context, scope string, in NativeTurn) (NativeIngestResult, error) {
	f.finds++
	f.received = in
	f.scope = scope
	return f.result, f.err
}
func (f *nativeConversationFake) ListMemories(_ context.Context, scope string) ([]NativeMessage, error) {
	f.reads++
	f.scope = scope
	return f.listing, f.err
}
func (f *nativeConversationFake) SearchMemories(_ context.Context, scope, query string, limit int) ([]NativeMessage, error) {
	f.searches++
	f.scope = scope
	return f.hits, f.err
}
func nativeConversationFixture(t *testing.T) (*Service, Scope, *nativeConversationFake) {
	s, sc, _ := nativeFixture(t)
	native := &nativeConversationFake{result: NativeIngestResult{Status: "received", SourceID: "9100000000000000001", Messages: []NativeMessage{}}}
	s.cfg.Native = native
	return s, sc, native
}
func conversationInput() ConversationInput {
	return ConversationInput{NativeTurn: NativeTurn{OperationID: "chat-op", SessionID: "runtime-session", TurnID: "runtime-run", UserInput: "我喜欢无糖乌龙茶。", AgentResponse: "你平时喜欢无糖乌龙茶。"}, ConsentRef: "consent-1", PolicyVersion: "policy-1", DataClass: "synthetic"}
}
func TestConversationDurableReservationIdempotencyAndReadOnlyRecovery(t *testing.T) {
	s, sc, native := nativeConversationFixture(t)
	in := conversationInput()
	native.err = ErrIndexUnknown
	receipt, err := s.IngestConversation(testCtx, sc, in)
	require.NoError(t, err)
	require.Equal(t, "unknown", receipt.Status)
	require.Equal(t, 1, native.adds)
	require.Equal(t, in.NativeTurn, native.received)
	require.Equal(t, "memory-"+scopeKey(sc), native.scope)
	var row conversationRow
	require.NoError(t, s.db.First(&row).Error)
	require.NotContains(t, string(row.Payload), in.UserInput)
	again, err := s.IngestConversation(testCtx, sc, in)
	require.NoError(t, err)
	require.Equal(t, receipt, again)
	require.Equal(t, 1, native.adds)
	changed := in
	changed.UserInput = "我喜欢加糖红茶。"
	_, err = s.IngestConversation(testCtx, sc, changed)
	require.ErrorIs(t, err, ErrConflict)
	changed = in
	changed.OperationID = "new-id-same-runtime-turn"
	_, err = s.IngestConversation(testCtx, sc, changed)
	require.ErrorIs(t, err, ErrConflict)
	native.err = nil
	native.result.Status = "derivedAvailable"
	recovered, err := s.ConversationStatus(testCtx, sc, in.OperationID)
	require.NoError(t, err)
	require.Equal(t, "derivedAvailable", recovered.Status)
	require.Equal(t, 1, native.adds)
	require.Equal(t, 1, native.finds)
	require.Equal(t, in.NativeTurn, native.received)
	require.True(t, s.Capabilities().AutomaticExtractionSupported)
	require.False(t, s.Capabilities().RawOnly)
	require.False(t, s.Capabilities().CorrectionSupported)
	require.True(t, s.Capabilities().ConversationCorrectionsSupported)
	require.Equal(t, 4000, s.Capabilities().NativeMaxContextChars)
	require.Equal(t, 5, s.Capabilities().NativeDefaultMessages)
	require.Equal(t, 20, s.Capabilities().NativeMaxMessages)
	require.Equal(t, []string{"raw", "semantic", "episodic", "procedural"}, s.Capabilities().MemoryTypes)
}
func TestConversationInputsAndInactiveScopesProduceNoNativeWrites(t *testing.T) {
	s, sc, native := nativeConversationFixture(t)
	in := conversationInput()
	invalid := []ConversationInput{in, in, in, in}
	invalid[0].AgentResponse = ""
	invalid[1].PolicyVersion = "different"
	invalid[2].DataClass = "production"
	invalid[3].UserInput = strings.Repeat("好", 6001)
	for _, input := range invalid {
		_, err := s.IngestConversation(testCtx, sc, input)
		require.ErrorIs(t, err, ErrInvalid)
	}
	other := sc
	other.UserID = "other-user"
	_, err := s.IngestConversation(testCtx, other, in)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.ConversationStatus(testCtx, other, in.OperationID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.ListNativeMemories(testCtx, other)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.NativeRecall(testCtx, other, "饮品", 5)
	require.ErrorIs(t, err, ErrNotFound)
	require.Zero(t, native.adds)
	require.Zero(t, native.finds)
	require.Zero(t, native.reads)
	require.Zero(t, native.searches)
}
func TestNativeContextPrioritizesRecentOriginalCorrectionAndBoundsWholeRecords(t *testing.T) {
	s, sc, native := nativeConversationFixture(t)
	latest := NativeMessage{ID: "103", Type: "raw", Content: "User Input: 我现在更喜欢黑咖啡，乌龙茶是以前的偏好。\nAgent Response: 明白。", CreatedAt: "2026-11-08 12:00:00", Status: true}
	previous := NativeMessage{ID: "101", Type: "raw", Content: "User Input: 我喜欢乌龙茶。\nAgent Response: 好的。", CreatedAt: "2026-11-08 11:00:00", Status: true}
	old := NativeMessage{ID: "102", SourceID: "101", Type: "semantic", Content: "用户喜欢乌龙茶。", Status: true}
	native.listing = []NativeMessage{previous, latest}
	native.hits = []NativeMessage{old, {ID: "104", Type: "procedural", Content: strings.Repeat("长", 4001), Status: true}, {ID: "105", Type: "semantic", Content: "带原生事件时间的资料", Status: true, InvalidAt: "2020-01-01 00:00:00"}}
	got, err := s.NativeRecall(testCtx, sc, "我现在喜欢什么饮品", 5)
	require.NoError(t, err)
	require.Len(t, got.Messages, 4)
	require.Equal(t, latest, got.Messages[0])
	require.Equal(t, previous, got.Messages[1])
	require.Equal(t, old, got.Messages[2])
	require.True(t, got.BudgetLimited)
	require.True(t, got.RecentSourceIncluded)
	require.Contains(t, got.CurrentnessNote, "latest explicit user correction")
	require.NotContains(t, got.Messages, NativeMessage{ID: "104"})
	// With a single-record budget the latest original remains ahead of stale vectors.
	got, err = s.NativeRecall(testCtx, sc, "饮品", 1)
	require.NoError(t, err)
	require.Equal(t, []NativeMessage{latest}, got.Messages)
}

func TestNativeRecallRetainsPastAndFutureEventTimesAsData(t *testing.T) {
	s, sc, native := nativeConversationFixture(t)
	past := NativeMessage{ID: "201", Type: "episodic", Content: "昨天参加了茶文化分享会。", Status: true, ValidAt: "2026-11-07 10:00:00", InvalidAt: "2026-11-07 12:00:00"}
	future := NativeMessage{ID: "202", Type: "episodic", Content: "下个月计划去杭州参观。", Status: true, ValidAt: "2026-12-01 10:00:00", InvalidAt: "2026-12-01 12:00:00"}
	opaque := NativeMessage{ID: "203", Type: "semantic", Content: "时间由提供方解释。", Status: true, InvalidAt: "provider-specific-time"}
	disabled := NativeMessage{ID: "204", Type: "semantic", Content: "已停用消息不得供给。", Status: false}
	native.hits = []NativeMessage{past, future, opaque, disabled}
	got, err := s.NativeRecall(testCtx, sc, "活动与计划", 5)
	require.NoError(t, err)
	require.Equal(t, []NativeMessage{past, future, opaque}, got.Messages)
	// Native valid_at/invalid_at describe event times. Their presence does not
	// prove an access expiration; source status remains the availability switch.
	require.Equal(t, past.InvalidAt, got.Messages[0].InvalidAt)
	require.Equal(t, future.ValidAt, got.Messages[1].ValidAt)
}

func TestAutomaticApplicationsRejectManualLedgerMutationsBeforeAnyEffects(t *testing.T) {
	s, sc, index := nativeFixture(t)
	fact := publish(t, s, sc, input("原有审核事实。"))
	syncAll(t, s)
	before := ledgerSnapshot(t, s)
	puts, disabled := index.puts, index.disabledCalls
	native := &nativeConversationFake{}
	s.cfg.Native = native
	_, err := s.SubmitCandidate(testCtx, sc, input("不得作为人工候选提交。"))
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = s.ClaimTask(testCtx, sc, "manual-claim")
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = s.Publish(testCtx, sc, PublishInput{OperationID: "manual-publish", TaskID: "fake"})
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = s.Update(testCtx, sc, fact.ID, fact.Version, "不得替换事实。", "manual-edit")
	require.ErrorIs(t, err, ErrUnsupported)
	require.Equal(t, before, ledgerSnapshot(t, s))
	var count int64
	require.NoError(t, s.db.Model(&conversationRow{}).Count(&count).Error)
	require.Zero(t, count)
	require.Equal(t, puts, index.puts)
	require.Equal(t, disabled, index.disabledCalls)
	require.Zero(t, native.adds)
	require.Zero(t, native.finds)
	require.Zero(t, native.reads)
	require.Zero(t, native.searches)
}
