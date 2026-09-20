package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"control-panel/internal/domain/agentrelation"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RegisterAgentMessageWaitInput struct {
	RunID, RuntimeSessionID           string
	WaitingAgentID                    uint64
	ParentMessageID, WaitingMessageID string
	DeadlineAt                        *time.Time
	DedupeKey                         string
}

type AgentMessageWaitService struct {
	repo *repository.AgentMessageWaitRepository
	now  func() time.Time
}

func NewAgentMessageWaitService() *AgentMessageWaitService {
	return &AgentMessageWaitService{repo: repository.NewAgentMessageWaitRepository(), now: func() time.Time { return time.Now().UTC() }}
}

func NewAgentMessageWaitServiceWithRepository(repo *repository.AgentMessageWaitRepository) *AgentMessageWaitService {
	return &AgentMessageWaitService{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

func (s *AgentMessageWaitService) Register(_ context.Context, tenantID string, input RegisterAgentMessageWaitInput) (*agentrelation.AgentMessageWait, bool, error) {
	return s.registerWithRepository(tenantID, input, s.repo)
}

func (s *AgentMessageWaitService) RegisterTx(_ context.Context, tx *gorm.DB, tenantID string, input RegisterAgentMessageWaitInput) (*agentrelation.AgentMessageWait, bool, error) {
	if tx == nil {
		return nil, false, errors.New("transaction is required")
	}
	return s.registerWithRepository(tenantID, input, repository.NewAgentMessageWaitRepositoryWithDB(tx))
}

func (s *AgentMessageWaitService) registerWithRepository(tenantID string, input RegisterAgentMessageWaitInput, repo *repository.AgentMessageWaitRepository) (*agentrelation.AgentMessageWait, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	input.RunID, input.RuntimeSessionID = strings.TrimSpace(input.RunID), strings.TrimSpace(input.RuntimeSessionID)
	input.ParentMessageID, input.WaitingMessageID = strings.TrimSpace(input.ParentMessageID), strings.TrimSpace(input.WaitingMessageID)
	input.DedupeKey = strings.TrimSpace(input.DedupeKey)
	if tenantID == "" || input.RunID == "" || input.RuntimeSessionID == "" || input.WaitingAgentID == 0 || input.ParentMessageID == "" || input.WaitingMessageID == "" || input.DedupeKey == "" {
		return nil, false, errors.New("tenant_id, run_id, runtime_session_id, waiting_agent_id, parent_message_id, waiting_message_id and dedupe_key are required")
	}
	now := s.now()
	if input.DeadlineAt != nil && !input.DeadlineAt.After(now) {
		return nil, false, errors.New("deadline_at must be in the future")
	}
	waitingAgent, err := repo.ValidateMessages(tenantID, input.RunID, input.ParentMessageID, input.WaitingMessageID, input.WaitingAgentID)
	if err != nil {
		return nil, false, err
	}
	wait := &agentrelation.AgentMessageWait{
		ID: uuid.NewString(), TenantID: tenantID, RunID: input.RunID,
		WaitingAgentID: input.WaitingAgentID, WaitingAgent: waitingAgent, RuntimeSessionID: input.RuntimeSessionID,
		ParentMessageID: input.ParentMessageID, WaitingMessageID: input.WaitingMessageID,
		Status: agentrelation.MessageWaitStatusWaiting, DeadlineAt: input.DeadlineAt,
		DedupeKey: input.DedupeKey, CreatedAt: now, UpdatedAt: now,
	}
	stored, created, err := repo.CreateIdempotent(tenantID, wait)
	if err != nil {
		return nil, false, err
	}
	if !created && (stored.RunID != wait.RunID || stored.RuntimeSessionID != wait.RuntimeSessionID || stored.WaitingAgentID != wait.WaitingAgentID || stored.ParentMessageID != wait.ParentMessageID || stored.WaitingMessageID != wait.WaitingMessageID) {
		return nil, false, errors.New("dedupe_key is already bound to a different callback wait")
	}
	return stored, created, nil
}

func (s *AgentMessageWaitService) ClaimReadyForMessage(_ context.Context, tenantID, runID, waitingMessageID, eventID string, limit int) ([]*agentrelation.AgentMessageWait, error) {
	return s.repo.ClaimReadyForMessage(strings.TrimSpace(tenantID), strings.TrimSpace(runID), strings.TrimSpace(waitingMessageID), strings.TrimSpace(eventID), s.now(), limit)
}

func (s *AgentMessageWaitService) ExistsForWaitingMessage(_ context.Context, tenantID, waitingMessageID string) (bool, error) {
	return s.repo.ExistsForWaitingMessage(strings.TrimSpace(tenantID), strings.TrimSpace(waitingMessageID))
}

func (s *AgentMessageWaitService) CompleteResume(_ context.Context, tenantID, waitID, eventID string) error {
	return s.repo.CompleteResume(strings.TrimSpace(tenantID), strings.TrimSpace(waitID), strings.TrimSpace(eventID), s.now())
}

func (s *AgentMessageWaitService) FailResume(_ context.Context, tenantID, waitID, eventID, cause string, retryable bool) error {
	return s.repo.FailResume(strings.TrimSpace(tenantID), strings.TrimSpace(waitID), strings.TrimSpace(eventID), strings.TrimSpace(cause), retryable, s.now())
}

func (s *AgentMessageWaitService) ExpireDue(_ context.Context, tenantID string, now time.Time, limit int) (int64, error) {
	if now.IsZero() {
		now = s.now()
	}
	return s.repo.ExpireDue(strings.TrimSpace(tenantID), now.UTC(), limit)
}

func (s *AgentMessageWaitService) Cancel(_ context.Context, tenantID, waitID, reason string) error {
	return s.repo.Cancel(strings.TrimSpace(tenantID), strings.TrimSpace(waitID), strings.TrimSpace(reason), s.now())
}
