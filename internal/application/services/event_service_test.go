package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	eventdomain "control-panel/internal/domain/event"
	rundomain "control-panel/internal/domain/run"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func eventTestService(t *testing.T) (*EventService, *gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&rundomain.Run{}, &eventdomain.StreamCursor{}, &eventdomain.Envelope{}, &eventdomain.Delivery{}, &eventdomain.DeliveryAttempt{}, &eventdomain.CausalBudget{}))
	runID := "00000000-0000-0000-0000-000000000001"
	require.NoError(t, db.Create(&rundomain.Run{ID: runID, TenantID: "tenant-a", Name: "test", Status: rundomain.StatusRunning}).Error)
	return NewEventService(db), db, runID
}

func validEvent(runID, key string) PublishEventInput {
	return PublishEventInput{RunID: runID, Type: "agenthub.task.requested.v1", Source: "agenthub:test", Scope: eventdomain.Ref{Type: "run", ID: runID}, Subject: eventdomain.Ref{Type: "agent", ID: "7"}, Actor: eventdomain.Ref{Type: "user", ID: "u1"}, IdempotencyKey: key, Data: map[string]any{"title": "review"}}
}

func TestEventPublishIsDurableIdempotentAndOrderedPerStream(t *testing.T) {
	s, db, runID := eventTestService(t)
	first, err := s.Publish("tenant-a", validEvent(runID, "request:1"))
	require.NoError(t, err)
	replayed, err := s.Publish("tenant-a", validEvent(runID, "request:1"))
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)
	second, err := s.Publish("tenant-a", validEvent(runID, "request:2"))
	require.NoError(t, err)
	require.Equal(t, uint64(1), first.Sequence)
	require.Equal(t, uint64(2), second.Sequence)

	other := validEvent(runID, "request:other")
	other.Subject.ID = "8"
	third, err := s.Publish("tenant-a", other)
	require.NoError(t, err)
	require.Equal(t, uint64(1), third.Sequence)
	var deliveries int64
	require.NoError(t, db.Model(&eventdomain.Delivery{}).Count(&deliveries).Error)
	require.Equal(t, int64(3), deliveries)
}

func TestEventDeliveryRetryDeadLetterAndNoOvertaking(t *testing.T) {
	s, db, runID := eventTestService(t)
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	firstInput := validEvent(runID, "request:1")
	firstInput.MaxAttempts = 2
	first, err := s.Publish("tenant-a", firstInput)
	require.NoError(t, err)
	second, err := s.Publish("tenant-a", validEvent(runID, "request:2"))
	require.NoError(t, err)
	delivery, event, err := s.ClaimNext("tenant-a")
	require.NoError(t, err)
	require.Equal(t, first.ID, event.ID)
	require.NoError(t, s.Fail("tenant-a", delivery.ID, errors.New("temporary")))

	// The second event cannot overtake the retrying first stream event.
	_, _, err = s.ClaimNext("tenant-a")
	require.ErrorIs(t, err, ErrNoDelivery)
	now = now.Add(time.Second)
	delivery, event, err = s.ClaimNext("tenant-a")
	require.NoError(t, err)
	require.Equal(t, first.ID, event.ID)
	require.NoError(t, s.Fail("tenant-a", delivery.ID, errors.New("permanent")))
	var dead eventdomain.Delivery
	require.NoError(t, db.Where("event_id = ?", first.ID).First(&dead).Error)
	require.Equal(t, eventdomain.DeliveryDeadLetter, dead.Status)

	_, event, err = s.ClaimNext("tenant-a")
	require.NoError(t, err)
	require.Equal(t, second.ID, event.ID)
}

func TestExpiredLeaseRecoversAfterRestartAndCancellationIsTerminal(t *testing.T) {
	s, db, runID := eventTestService(t)
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	in := validEvent(runID, "lease:1")
	in.Timeout = 10 * time.Millisecond
	_, err := s.Publish("tenant-a", in)
	require.NoError(t, err)
	claimed, _, err := s.ClaimNext("tenant-a")
	require.NoError(t, err)

	// A new service instance shares only the DB, proving no in-memory active flag
	// is needed to recover an abandoned delivery.
	restarted := NewEventService(db)
	now = now.Add(11 * time.Millisecond)
	restarted.now = func() time.Time { return now }
	reclaimed, _, err := restarted.ClaimNext("tenant-a")
	require.NoError(t, err)
	require.Equal(t, claimed.ID, reclaimed.ID)
	require.NoError(t, restarted.Cancel("tenant-a", reclaimed.ID))
	_, _, err = restarted.ClaimNext("tenant-a")
	require.ErrorIs(t, err, ErrNoDelivery)
}

func TestProcessNextHonorsTimeoutAndRetries(t *testing.T) {
	s, db, runID := eventTestService(t)
	in := validEvent(runID, "timeout:1")
	in.Timeout = time.Millisecond
	_, err := s.Publish("tenant-a", in)
	require.NoError(t, err)
	err = s.ProcessNext(context.Background(), "tenant-a", func(ctx context.Context, _ eventdomain.Envelope) error {
		<-ctx.Done()
		return ctx.Err()
	})
	require.Error(t, err)
	var row eventdomain.Delivery
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, eventdomain.DeliveryRetry, row.Status)
}

func TestDerivedEventCarriesExplicitCausalChain(t *testing.T) {
	s, _, runID := eventTestService(t)
	root, err := s.Publish("tenant-a", validEvent(runID, "root"))
	require.NoError(t, err)
	derivedInput := validEvent(runID, "derived")
	derivedInput.Type = "agenthub.task.reviewed.v1"
	derivedInput.CorrelationID = root.CorrelationID
	derivedInput.CausationID = root.ID
	derivedInput.RootEventID = root.ID
	derived, err := s.Publish("tenant-a", derivedInput)
	require.NoError(t, err)
	require.Equal(t, root.ID, derived.CausationID)
	require.Equal(t, root.ID, derived.RootEventID)
}

func TestCausalBudgetTerminatesAndEmitsAuditableGuard(t *testing.T) {
	s, db, runID := eventTestService(t)
	root, err := s.Publish("tenant-a", validEvent(runID, "budget-root"))
	require.NoError(t, err)
	require.NoError(t, db.Model(&eventdomain.CausalBudget{}).Where("tenant_id = ? AND root_event_id = ?", "tenant-a", root.ID).Updates(map[string]any{"max_tool_calls": 1, "max_tokens": 10}).Error)
	budget, err := s.ConsumeUsage("tenant-a", runID, root.ID, 2, 4)
	require.NoError(t, err)
	require.True(t, budget.Terminated)
	require.Equal(t, "tool_budget_exceeded", budget.TerminationReason)
	require.NotEmpty(t, budget.GuardEventID)
	var guard eventdomain.Envelope
	require.NoError(t, db.Where("id = ?", budget.GuardEventID).First(&guard).Error)
	require.Equal(t, "agenthub.guard.triggered.v1", guard.Type)

	derived := validEvent(runID, "after-termination")
	derived.RootEventID = root.ID
	derived.CorrelationID = root.CorrelationID
	derived.CausationID = root.ID
	_, err = s.Publish("tenant-a", derived)
	require.ErrorIs(t, err, ErrChainTerminated)
}

func TestListRunAndChainAreTenantScopedAndIncludeDelivery(t *testing.T) {
	s, _, runID := eventTestService(t)
	root, err := s.Publish("tenant-a", validEvent(runID, "list-root"))
	require.NoError(t, err)
	derivedInput := validEvent(runID, "list-child")
	derivedInput.RootEventID, derivedInput.CorrelationID, derivedInput.CausationID = root.ID, root.CorrelationID, root.ID
	derived, err := s.Publish("tenant-a", derivedInput)
	require.NoError(t, err)
	records, err := s.ListRun("tenant-a", runID, 10)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, eventdomain.DeliveryPending, records[0].Delivery.Status)
	chain, err := s.ListChain("tenant-a", runID, root.ID, 10)
	require.NoError(t, err)
	require.Len(t, chain, 2)
	require.Equal(t, derived.ID, chain[1].Event.ID)
	_, err = s.ListRun("tenant-b", runID, 10)
	require.ErrorIs(t, err, rundomain.ErrNotFound)
}
