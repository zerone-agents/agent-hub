package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agentrelation"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type messageWaitFixture struct {
	db      *gorm.DB
	service *AgentMessageWaitService
	now     time.Time
}

func setupMessageWaitService(t *testing.T) messageWaitFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:wait-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", strings.ReplaceAll(t.Name(), "/", "-"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentrelation.AgentMessage{}, &agentrelation.AgentMessageWait{}))
	now := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	svc := NewAgentMessageWaitServiceWithRepository(repository.NewAgentMessageWaitRepositoryWithDB(db))
	svc.now = func() time.Time { return now }
	return messageWaitFixture{db: db, service: svc, now: now}
}

func (f messageWaitFixture) messages(t *testing.T, tenant, run string, agentID uint64) {
	t.Helper()
	parent := &agentrelation.AgentMessage{ID: "parent-1", TenantID: tenant, RunID: run, RelationID: 1, ConversationID: "conv-1", RootMessageID: "parent-1", Hop: 1, Scope: "org", SourceAgentID: 1, SourceAgent: "a", TargetAgentID: agentID, TargetAgent: "b", Action: "assign", DeliveryPolicy: "async", ContextPolicy: "none", Content: "work", Status: agentrelation.MessageStatusRunning, CreatedAt: f.now}
	child := &agentrelation.AgentMessage{ID: "child-1", TenantID: tenant, RunID: run, RelationID: 2, ConversationID: "conv-1", RootMessageID: "parent-1", ParentMessageID: "parent-1", Hop: 2, Scope: "org", SourceAgentID: agentID, SourceAgent: "b", TargetAgentID: 3, TargetAgent: "c", Action: "consult", DeliveryPolicy: "async", ContextPolicy: "none", Content: "help", Status: agentrelation.MessageStatusQueued, CreatedAt: f.now}
	require.NoError(t, f.db.Create(parent).Error)
	require.NoError(t, f.db.Create(child).Error)
}

func registerWait(t *testing.T, f messageWaitFixture, dedupe string) *agentrelation.AgentMessageWait {
	t.Helper()
	deadline := f.now.Add(time.Hour)
	wait, created, err := f.service.Register(context.Background(), "tenant-a", RegisterAgentMessageWaitInput{RunID: "run-1", RuntimeSessionID: "runtime-session-1", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DeadlineAt: &deadline, DedupeKey: dedupe})
	require.NoError(t, err)
	require.True(t, created)
	return wait
}

func TestAgentMessageWaitRegisterIsIdempotentAndTenantRunIsolated(t *testing.T) {
	f := setupMessageWaitService(t)
	f.messages(t, "tenant-a", "run-1", 2)
	wait := registerWait(t, f, "parent-1:child-1")
	require.Equal(t, "runtime-session-1", wait.RuntimeSessionID)

	deadline := f.now.Add(2 * time.Hour)
	replayed, created, err := f.service.Register(context.Background(), "tenant-a", RegisterAgentMessageWaitInput{RunID: "run-1", RuntimeSessionID: "runtime-session-1", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DeadlineAt: &deadline, DedupeKey: "parent-1:child-1"})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, wait.ID, replayed.ID)
	require.Equal(t, "runtime-session-1", replayed.RuntimeSessionID)
	_, _, err = f.service.Register(context.Background(), "tenant-a", RegisterAgentMessageWaitInput{RunID: "run-1", RuntimeSessionID: "different-session", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DedupeKey: "parent-1:child-1"})
	require.ErrorContains(t, err, "different callback wait")

	_, _, err = f.service.Register(context.Background(), "tenant-b", RegisterAgentMessageWaitInput{RunID: "run-1", RuntimeSessionID: "s", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DedupeKey: "x"})
	require.Error(t, err)
	_, _, err = f.service.Register(context.Background(), "tenant-a", RegisterAgentMessageWaitInput{RunID: "other-run", RuntimeSessionID: "s", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DedupeKey: "x"})
	require.Error(t, err)
}

func TestAgentMessageWaitClaimCompleteAndEventReplay(t *testing.T) {
	f := setupMessageWaitService(t)
	f.messages(t, "tenant-a", "run-1", 2)
	wait := registerWait(t, f, "wait-1")

	claimed, err := f.service.ClaimReadyForMessage(context.Background(), "tenant-a", "run-1", "child-1", "event-1", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, wait.ID, claimed[0].ID)
	require.Equal(t, 1, claimed[0].ResumeAttempts)
	require.Equal(t, agentrelation.MessageWaitStatusResuming, claimed[0].Status)

	again, err := f.service.ClaimReadyForMessage(context.Background(), "tenant-a", "run-1", "child-1", "event-1", 10)
	require.NoError(t, err)
	require.Empty(t, again)
	require.NoError(t, f.service.CompleteResume(context.Background(), "tenant-a", wait.ID, "event-1"))
	require.NoError(t, f.service.CompleteResume(context.Background(), "tenant-a", wait.ID, "event-1"))
	require.Error(t, f.service.CompleteResume(context.Background(), "tenant-a", wait.ID, "different-event"))
}

func TestAgentMessageWaitConcurrentClaimHasSingleWinner(t *testing.T) {
	f := setupMessageWaitService(t)
	f.messages(t, "tenant-a", "run-1", 2)
	registerWait(t, f, "wait-concurrent")
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	for _, eventID := range []string{"event-a", "event-b"} {
		wg.Add(1)
		go func(eventID string) {
			defer wg.Done()
			claimed, _ := f.service.ClaimReadyForMessage(context.Background(), "tenant-a", "run-1", "child-1", eventID, 10)
			counts <- len(claimed)
		}(eventID)
	}
	wg.Wait()
	close(counts)
	total := 0
	for count := range counts {
		total += count
	}
	require.Equal(t, 1, total)
}

func TestAgentMessageWaitRetryExpireAndCancel(t *testing.T) {
	f := setupMessageWaitService(t)
	f.messages(t, "tenant-a", "run-1", 2)
	wait := registerWait(t, f, "wait-retry")
	_, err := f.service.ClaimReadyForMessage(context.Background(), "tenant-a", "run-1", "child-1", "event-1", 10)
	require.NoError(t, err)
	require.NoError(t, f.service.FailResume(context.Background(), "tenant-a", wait.ID, "event-1", "runtime down", true))
	claimed, err := f.service.ClaimReadyForMessage(context.Background(), "tenant-a", "run-1", "child-1", "event-2", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, 2, claimed[0].ResumeAttempts)
	require.NoError(t, f.service.FailResume(context.Background(), "tenant-a", wait.ID, "event-2", "invalid session", false))

	// A separate overdue wait is expired and can no longer be claimed.
	f.now = f.now.Add(2 * time.Hour)
	f.service.now = func() time.Time { return f.now }
	deadline := f.now.Add(time.Hour)
	second, created, err := f.service.Register(context.Background(), "tenant-a", RegisterAgentMessageWaitInput{RunID: "run-1", RuntimeSessionID: "session-2", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DeadlineAt: &deadline, DedupeKey: "wait-expire"})
	require.NoError(t, err)
	require.True(t, created)
	expired, err := f.service.ExpireDue(context.Background(), "tenant-a", f.now.Add(2*time.Hour), 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, expired)
	row, err := f.service.repo.Get("tenant-a", second.ID)
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageWaitStatusExpired, row.Status)

	third, created, err := f.service.Register(context.Background(), "tenant-a", RegisterAgentMessageWaitInput{RunID: "run-1", RuntimeSessionID: "session-3", WaitingAgentID: 2, ParentMessageID: "parent-1", WaitingMessageID: "child-1", DedupeKey: "wait-cancel"})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, f.service.Cancel(context.Background(), "tenant-a", third.ID, "parent cancelled"))
	row, err = f.service.repo.Get("tenant-a", third.ID)
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageWaitStatusCancelled, row.Status)
}
