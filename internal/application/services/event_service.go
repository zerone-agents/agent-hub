package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	eventdomain "control-panel/internal/domain/event"
	rundomain "control-panel/internal/domain/run"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNoDelivery      = errors.New("no event delivery ready")
	ErrDeliveryClosed  = errors.New("event delivery is already closed")
	ErrInvalidEnvelope = errors.New("invalid event envelope")
	ErrChainTerminated = errors.New("causal chain budget terminated")
)

type EventService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewEventService(db *gorm.DB) *EventService {
	return &EventService{db: db, now: func() time.Time { return time.Now().UTC() }}
}

type PublishEventInput struct {
	RunID, Type, Source, Visibility         string
	Scope, Subject, Actor                   eventdomain.Ref
	CorrelationID, CausationID, RootEventID string
	IdempotencyKey, DataSchema              string
	Data                                    map[string]any
	OccurredAt, ScheduledAt                 time.Time
	MaxAttempts                             int
	Timeout                                 time.Duration
}

// Publish atomically accepts an immutable event and creates its outbox row.
func (s *EventService) Publish(tenantID string, input PublishEventInput) (*eventdomain.Envelope, error) {
	var accepted *eventdomain.Envelope
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		accepted, err = s.PublishTx(tx, tenantID, input)
		return err
	})
	return accepted, err
}

// PublishTx allows a state commit and its emitted event/outbox record to share
// one database transaction. Callers must not retain tx beyond their callback.
func (s *EventService) PublishTx(tx *gorm.DB, tenantID string, input PublishEventInput) (*eventdomain.Envelope, error) {
	if err := validateEventInput(tenantID, input); err != nil {
		return nil, err
	}
	var existing eventdomain.Envelope
	err := tx.Where("tenant_id = ? AND idempotency_key = ?", tenantID, input.IdempotencyKey).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var run rundomain.Run
	if err := tx.Where("tenant_id = ? AND id = ?", tenantID, input.RunID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, rundomain.ErrNotFound
		}
		return nil, err
	}
	if run.Status == rundomain.StatusCompleted || run.Status == rundomain.StatusArchived {
		return nil, rundomain.ErrFrozen
	}

	now := s.now()
	id := uuid.NewString()
	correlationID := strings.TrimSpace(input.CorrelationID)
	if correlationID == "" {
		correlationID = id
	}
	rootID := strings.TrimSpace(input.RootEventID)
	if rootID == "" {
		rootID = id
	}
	budgetReason := ""
	if input.Type != "agenthub.guard.triggered.v1" {
		var err error
		budgetReason, err = s.consumeBudgetTx(tx, tenantID, input.RunID, rootID, 1, 0, 0)
		if err != nil {
			return nil, err
		}
	}
	sequence, err := s.nextSequence(tx, tenantID, input)
	if err != nil {
		return nil, err
	}
	e := &eventdomain.Envelope{
		ID: id, SpecVersion: "1.0", TenantID: tenantID, RunID: input.RunID,
		Type: strings.TrimSpace(input.Type), Source: strings.TrimSpace(input.Source),
		ScopeType: input.Scope.Type, ScopeID: input.Scope.ID,
		SubjectType: input.Subject.Type, SubjectID: input.Subject.ID,
		ActorType: input.Actor.Type, ActorID: input.Actor.ID,
		Visibility:    defaultString(input.Visibility, "participants"),
		CorrelationID: correlationID, CausationID: input.CausationID, RootEventID: rootID,
		IdempotencyKey: input.IdempotencyKey, Sequence: sequence,
		DataSchema: input.DataSchema, Data: input.Data,
		OccurredAt: defaultTime(input.OccurredAt, now), ScheduledAt: defaultTime(input.ScheduledAt, now), RecordedAt: now,
	}
	if err := tx.Create(e).Error; err != nil {
		if isDuplicate(err) {
			if findErr := tx.Where("tenant_id = ? AND idempotency_key = ?", tenantID, input.IdempotencyKey).First(&existing).Error; findErr == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	d := eventdomain.Delivery{ID: uuid.NewString(), TenantID: tenantID, RunID: input.RunID, EventID: e.ID, Status: eventdomain.DeliveryPending, MaxAttempts: maxAttempts, TimeoutMillis: timeout.Milliseconds(), NextAttemptAt: e.ScheduledAt}
	if err := tx.Create(&d).Error; err != nil {
		return nil, err
	}
	if budgetReason != "" {
		guard, err := s.PublishTx(tx, tenantID, PublishEventInput{
			RunID: input.RunID, Type: "agenthub.guard.triggered.v1", Source: "agenthub:event-budget",
			Scope: input.Scope, Subject: input.Subject, Actor: eventdomain.Ref{Type: "platform", ID: "event-budget"},
			Visibility: input.Visibility, CorrelationID: correlationID, CausationID: e.ID, RootEventID: rootID,
			IdempotencyKey: "guard:" + rootID + ":" + budgetReason,
			Data:           map[string]any{"reason": budgetReason, "rootEventId": rootID},
		})
		if err != nil {
			return nil, err
		}
		if err := tx.Model(&eventdomain.CausalBudget{}).Where("tenant_id = ? AND run_id = ? AND root_event_id = ?", tenantID, input.RunID, rootID).Update("guard_event_id", guard.ID).Error; err != nil {
			return nil, err
		}
	}
	return e, nil
}

// ConsumeUsage accounts non-event resources against the same durable causal
// budget. Tool and model executors call this after obtaining actual usage.
func (s *EventService) ConsumeUsage(tenantID, runID, rootEventID string, toolCalls, tokens int64) (*eventdomain.CausalBudget, error) {
	var result eventdomain.CausalBudget
	err := s.db.Transaction(func(tx *gorm.DB) error {
		row, err := s.ConsumeUsageTx(tx, tenantID, runID, rootEventID, toolCalls, tokens)
		if row != nil {
			result = *row
		}
		return err
	})
	return &result, err
}

// ConsumeUsageTx joins tool accounting, state effects, and a possible guard
// event in the caller's transaction.
func (s *EventService) ConsumeUsageTx(tx *gorm.DB, tenantID, runID, rootEventID string, toolCalls, tokens int64) (*eventdomain.CausalBudget, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(runID) == "" || strings.TrimSpace(rootEventID) == "" || toolCalls < 0 || tokens < 0 {
		return nil, fmt.Errorf("tenant, run and rootEventId are required and usage cannot be negative")
	}
	reason, err := s.consumeBudgetTx(tx, tenantID, runID, rootEventID, 0, toolCalls, tokens)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		guard, err := s.PublishTx(tx, tenantID, PublishEventInput{
			RunID: runID, Type: "agenthub.guard.triggered.v1", Source: "agenthub:event-budget",
			Scope: eventdomain.Ref{Type: "run", ID: runID}, Subject: eventdomain.Ref{Type: "run", ID: runID},
			Actor: eventdomain.Ref{Type: "platform", ID: "event-budget"}, CorrelationID: rootEventID, RootEventID: rootEventID,
			IdempotencyKey: "guard:" + rootEventID + ":" + reason, Data: map[string]any{"reason": reason, "rootEventId": rootEventID},
		})
		if err != nil {
			return nil, err
		}
		if err := tx.Model(&eventdomain.CausalBudget{}).Where("tenant_id = ? AND run_id = ? AND root_event_id = ?", tenantID, runID, rootEventID).Update("guard_event_id", guard.ID).Error; err != nil {
			return nil, err
		}
	}
	var result eventdomain.CausalBudget
	if err := tx.Where("tenant_id = ? AND run_id = ? AND root_event_id = ?", tenantID, runID, rootEventID).First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *EventService) consumeBudgetTx(tx *gorm.DB, tenantID, runID, rootID string, events, toolCalls, tokens int64) (string, error) {
	now := s.now()
	seed := eventdomain.CausalBudget{TenantID: tenantID, RunID: runID, RootEventID: rootID, MaxEvents: 1000, MaxToolCalls: 100, MaxTokens: 1_000_000, DeadlineAt: now.Add(24 * time.Hour)}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return "", err
	}
	var budget eventdomain.CausalBudget
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND run_id = ? AND root_event_id = ?", tenantID, runID, rootID).First(&budget).Error; err != nil {
		return "", err
	}
	if budget.Terminated {
		return "", ErrChainTerminated
	}
	budget.EventsUsed += events
	budget.ToolCallsUsed += toolCalls
	budget.TokensUsed += tokens
	reason := ""
	switch {
	case !now.Before(budget.DeadlineAt):
		reason = "deadline_exceeded"
	case budget.EventsUsed > budget.MaxEvents:
		reason = "event_budget_exceeded"
	case budget.ToolCallsUsed > budget.MaxToolCalls:
		reason = "tool_budget_exceeded"
	case budget.TokensUsed > budget.MaxTokens:
		reason = "token_budget_exceeded"
	}
	updates := map[string]any{"events_used": budget.EventsUsed, "tool_calls_used": budget.ToolCallsUsed, "tokens_used": budget.TokensUsed}
	if reason != "" {
		updates["terminated"], updates["termination_reason"] = true, reason
	}
	return reason, tx.Model(&budget).Updates(updates).Error
}

func (s *EventService) nextSequence(tx *gorm.DB, tenantID string, input PublishEventInput) (uint64, error) {
	where := eventdomain.StreamCursor{TenantID: tenantID, RunID: input.RunID, ScopeType: input.Scope.Type, ScopeID: input.Scope.ID, SubjectType: input.Subject.Type, SubjectID: input.Subject.ID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&where).Error; err != nil {
		return 0, err
	}
	var cursor eventdomain.StreamCursor
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND run_id = ? AND scope_type = ? AND scope_id = ? AND subject_type = ? AND subject_id = ?", tenantID, input.RunID, input.Scope.Type, input.Scope.ID, input.Subject.Type, input.Subject.ID).First(&cursor).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&cursor).UpdateColumn("sequence", gorm.Expr("sequence + 1")).Error; err != nil {
		return 0, err
	}
	return cursor.Sequence + 1, nil
}

// ClaimNext leases the oldest eligible event while preventing a later event
// in the same stream from overtaking an unfinished earlier one.
func (s *EventService) ClaimNext(tenantID string) (*eventdomain.Delivery, *eventdomain.Envelope, error) {
	var delivery eventdomain.Delivery
	var envelope eventdomain.Envelope
	now := s.now()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// A process may die after claiming. Expired leases become retryable.
		if err := tx.Model(&eventdomain.Delivery{}).Where("tenant_id = ? AND status = ? AND lease_expires_at <= ?", tenantID, eventdomain.DeliveryProcessing, now).Updates(map[string]any{"status": eventdomain.DeliveryRetry, "next_attempt_at": now, "lease_expires_at": nil, "last_error": "delivery lease expired"}).Error; err != nil {
			return err
		}
		q := tx.Table("event_deliveries AS d").Select("d.*").Joins("JOIN events e ON e.id = d.event_id").
			Where("d.tenant_id = ? AND d.status IN ? AND d.next_attempt_at <= ?", tenantID, []string{eventdomain.DeliveryPending, eventdomain.DeliveryRetry}, now).
			Where("NOT EXISTS (SELECT 1 FROM events prior_e JOIN event_deliveries prior_d ON prior_d.event_id = prior_e.id WHERE prior_e.tenant_id = e.tenant_id AND prior_e.run_id = e.run_id AND prior_e.scope_type = e.scope_type AND prior_e.scope_id = e.scope_id AND prior_e.subject_type = e.subject_type AND prior_e.subject_id = e.subject_id AND prior_e.sequence < e.sequence AND prior_d.status NOT IN ?)", []string{eventdomain.DeliveryDelivered, eventdomain.DeliveryCancelled, eventdomain.DeliveryDeadLetter}).
			Order("e.recorded_at ASC, e.sequence ASC").Limit(1)
		if err := q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Scan(&delivery).Error; err != nil {
			return err
		}
		if delivery.ID == "" {
			return ErrNoDelivery
		}
		lease := now.Add(time.Duration(delivery.TimeoutMillis) * time.Millisecond)
		result := tx.Model(&eventdomain.Delivery{}).Where("id = ? AND status IN ?", delivery.ID, []string{eventdomain.DeliveryPending, eventdomain.DeliveryRetry}).Updates(map[string]any{"status": eventdomain.DeliveryProcessing, "attempts": gorm.Expr("attempts + 1"), "lease_expires_at": lease})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNoDelivery
		}
		delivery.Status, delivery.Attempts, delivery.LeaseExpiresAt = eventdomain.DeliveryProcessing, delivery.Attempts+1, &lease
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, delivery.EventID).First(&envelope).Error; err != nil {
			return err
		}
		attempt := eventdomain.DeliveryAttempt{ID: uuid.NewString(), TenantID: tenantID, DeliveryID: delivery.ID, Attempt: delivery.Attempts, Status: eventdomain.DeliveryProcessing, StartedAt: now}
		return tx.Create(&attempt).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return &delivery, &envelope, nil
}

type EventHandler func(context.Context, eventdomain.Envelope) error

// RunWorker polls the durable outbox until ctx is cancelled. Applications
// start one worker per tenant/handler registry; an idle queue is not an error.
func (s *EventService) RunWorker(ctx context.Context, tenantID string, pollInterval time.Duration, handler EventHandler) error {
	if handler == nil {
		return errors.New("event handler is required")
	}
	if pollInterval <= 0 {
		pollInterval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		// Delivery failures are persisted by ProcessNext. Continuing keeps the
		// worker available for independent streams.
		_ = s.ProcessNext(ctx, tenantID, handler)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type EventRecord struct {
	Event    eventdomain.Envelope `json:"event"`
	Delivery eventdomain.Delivery `json:"delivery"`
}

func (s *EventService) ListRun(tenantID, runID string, limit int) ([]EventRecord, error) {
	return s.list(tenantID, runID, "", limit)
}

func (s *EventService) ListChain(tenantID, runID, rootEventID string, limit int) ([]EventRecord, error) {
	if strings.TrimSpace(rootEventID) == "" {
		return nil, fmt.Errorf("rootEventId is required")
	}
	return s.list(tenantID, runID, rootEventID, limit)
}

func (s *EventService) list(tenantID, runID, rootEventID string, limit int) ([]EventRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var run rundomain.Run
	if err := s.db.Select("id").Where("tenant_id = ? AND id = ?", tenantID, runID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, rundomain.ErrNotFound
		}
		return nil, err
	}
	q := s.db.Where("tenant_id = ? AND run_id = ?", tenantID, runID)
	if rootEventID != "" {
		q = q.Where("root_event_id = ?", rootEventID)
	}
	var events []eventdomain.Envelope
	if err := q.Order("recorded_at ASC, sequence ASC, id ASC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return []EventRecord{}, nil
	}
	ids := make([]string, len(events))
	for i := range events {
		ids[i] = events[i].ID
	}
	var deliveries []eventdomain.Delivery
	if err := s.db.Where("tenant_id = ? AND event_id IN ?", tenantID, ids).Find(&deliveries).Error; err != nil {
		return nil, err
	}
	byEvent := make(map[string]eventdomain.Delivery, len(deliveries))
	for _, delivery := range deliveries {
		byEvent[delivery.EventID] = delivery
	}
	records := make([]EventRecord, len(events))
	for i, event := range events {
		records[i] = EventRecord{Event: event, Delivery: byEvent[event.ID]}
	}
	return records, nil
}

// ProcessNext executes one leased delivery with its persisted timeout.
func (s *EventService) ProcessNext(ctx context.Context, tenantID string, handler EventHandler) error {
	d, e, err := s.ClaimNext(tenantID)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(d.TimeoutMillis)*time.Millisecond)
	defer cancel()
	err = handler(callCtx, *e)
	if err == nil && callCtx.Err() != nil {
		err = callCtx.Err()
	}
	if err == nil {
		return s.Complete(tenantID, d.ID)
	}
	if persistErr := s.Fail(tenantID, d.ID, err); persistErr != nil {
		return errors.Join(err, persistErr)
	}
	return err
}

func (s *EventService) Complete(tenantID, deliveryID string) error {
	now := s.now()
	return s.finishAttempt(tenantID, deliveryID, eventdomain.DeliveryDelivered, "", map[string]any{"status": eventdomain.DeliveryDelivered, "delivered_at": now, "lease_expires_at": nil})
}

func (s *EventService) Fail(tenantID, deliveryID string, cause error) error {
	if cause == nil {
		cause = errors.New("event handler failed")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var d eventdomain.Delivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, deliveryID).First(&d).Error; err != nil {
			return err
		}
		if d.Status != eventdomain.DeliveryProcessing {
			return ErrDeliveryClosed
		}
		now := s.now()
		status := eventdomain.DeliveryRetry
		updates := map[string]any{"status": status, "last_error": cause.Error(), "lease_expires_at": nil}
		if d.Attempts >= d.MaxAttempts {
			status = eventdomain.DeliveryDeadLetter
			updates["status"], updates["dead_lettered_at"] = status, now
		} else {
			updates["next_attempt_at"] = now.Add(backoff(d.Attempts))
		}
		if err := tx.Model(&d).Updates(updates).Error; err != nil {
			return err
		}
		return finishLatestAttempt(tx, tenantID, d.ID, status, cause.Error(), now)
	})
}

func (s *EventService) Cancel(tenantID, deliveryID string) error {
	now := s.now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var d eventdomain.Delivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, deliveryID).First(&d).Error; err != nil {
			return err
		}
		if d.Status == eventdomain.DeliveryDelivered || d.Status == eventdomain.DeliveryCancelled || d.Status == eventdomain.DeliveryDeadLetter {
			return ErrDeliveryClosed
		}
		if err := tx.Model(&d).Updates(map[string]any{"status": eventdomain.DeliveryCancelled, "cancelled_at": now, "lease_expires_at": nil}).Error; err != nil {
			return err
		}
		if d.Status == eventdomain.DeliveryProcessing {
			return finishLatestAttempt(tx, tenantID, d.ID, eventdomain.DeliveryCancelled, "cancelled", now)
		}
		return nil
	})
}

func (s *EventService) finishAttempt(tenantID, deliveryID, status, message string, updates map[string]any) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var d eventdomain.Delivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, deliveryID).First(&d).Error; err != nil {
			return err
		}
		if d.Status != eventdomain.DeliveryProcessing {
			return ErrDeliveryClosed
		}
		if err := tx.Model(&d).Updates(updates).Error; err != nil {
			return err
		}
		return finishLatestAttempt(tx, tenantID, d.ID, status, message, s.now())
	})
}

func finishLatestAttempt(tx *gorm.DB, tenantID, deliveryID, status, message string, now time.Time) error {
	var attempt eventdomain.DeliveryAttempt
	if err := tx.Where("tenant_id = ? AND delivery_id = ? AND finished_at IS NULL", tenantID, deliveryID).Order("attempt DESC").First(&attempt).Error; err != nil {
		return err
	}
	return tx.Model(&attempt).Updates(map[string]any{"status": status, "error": message, "finished_at": now}).Error
}

func validateEventInput(tenantID string, in PublishEventInput) error {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.Type) == "" || strings.TrimSpace(in.Source) == "" || strings.TrimSpace(in.IdempotencyKey) == "" || strings.TrimSpace(in.Scope.Type) == "" || strings.TrimSpace(in.Scope.ID) == "" || strings.TrimSpace(in.Subject.Type) == "" || strings.TrimSpace(in.Subject.ID) == "" {
		return fmt.Errorf("%w: tenant, run, type, source, scope, subject and idempotency key are required", ErrInvalidEnvelope)
	}
	if !strings.Contains(in.Type, ".v") {
		return fmt.Errorf("%w: event type must include a version suffix", ErrInvalidEnvelope)
	}
	return nil
}

func defaultTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value.UTC()
}
func backoff(attempt int) time.Duration {
	seconds := math.Pow(2, float64(max(attempt-1, 0)))
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}
