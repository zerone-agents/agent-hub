package repository

import (
	"errors"
	"time"

	"control-panel/internal/domain/agentrelation"
	"control-panel/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentMessageWaitRepository struct{ db *gorm.DB }

func NewAgentMessageWaitRepository() *AgentMessageWaitRepository {
	return &AgentMessageWaitRepository{db: database.GetDB()}
}

func NewAgentMessageWaitRepositoryWithDB(db *gorm.DB) *AgentMessageWaitRepository {
	return &AgentMessageWaitRepository{db: db}
}

func (r *AgentMessageWaitRepository) DB() *gorm.DB { return r.db }

// CreateIdempotent reserves a tenant-scoped dedupe key. A retry returns the
// original row and never creates a second continuation.
func (r *AgentMessageWaitRepository) CreateIdempotent(tenantID string, wait *agentrelation.AgentMessageWait) (*agentrelation.AgentMessageWait, bool, error) {
	if tenantID == "" {
		return nil, false, ErrTenantIDRequired
	}
	wait.TenantID = tenantID
	result := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(wait)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return wait, true, nil
	}
	var existing agentrelation.AgentMessageWait
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("dedupe_key = ?", wait.DedupeKey).First(&existing).Error
	return &existing, false, err
}

func (r *AgentMessageWaitRepository) Get(tenantID, id string) (*agentrelation.AgentMessageWait, error) {
	if tenantID == "" {
		return nil, ErrTenantIDRequired
	}
	var row agentrelation.AgentMessageWait
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).Where("id = ?", id).First(&row).Error
	return &row, err
}

// ExistsForWaitingMessage reports whether messageID has ever registered a
// callback continuation in this tenant. Terminal waits deliberately still
// count: a late explicit report/inform must not create a second return path
// after the callback has already resumed the waiting runtime session.
func (r *AgentMessageWaitRepository) ExistsForWaitingMessage(tenantID, messageID string) (bool, error) {
	if tenantID == "" {
		return false, ErrTenantIDRequired
	}
	if messageID == "" {
		return false, errors.New("waiting_message_id is required")
	}
	var count int64
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("waiting_message_id = ?", messageID).Limit(1).Count(&count).Error
	return count > 0, err
}

// ValidateMessages loads both sides under the same tenant and proves they
// belong to one non-empty Run. The waiting agent must own the child request
// and participate in the parent execution.
func (r *AgentMessageWaitRepository) ValidateMessages(tenantID, runID, parentID, waitingID string, waitingAgentID uint64) (string, error) {
	if tenantID == "" {
		return "", ErrTenantIDRequired
	}
	if runID == "" {
		return "", errors.New("run_id is required")
	}
	var parent, child agentrelation.AgentMessage
	if err := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).Where("id = ? AND run_id = ?", parentID, runID).First(&parent).Error; err != nil {
		return "", err
	}
	if err := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).Where("id = ? AND run_id = ?", waitingID, runID).First(&child).Error; err != nil {
		return "", err
	}
	if child.SourceAgentID != waitingAgentID {
		return "", errors.New("waiting agent must be the child message source")
	}
	if parent.SourceAgentID != waitingAgentID && parent.TargetAgentID != waitingAgentID {
		return "", errors.New("waiting agent does not participate in parent message")
	}
	if child.ParentMessageID != parent.ID {
		return "", errors.New("waiting message is not a child of parent message")
	}
	return child.SourceAgent, nil
}

// ClaimReadyForMessage atomically moves at most limit matching waits from
// waiting to resuming. The status predicate is the compare-and-swap: concurrent
// workers may select the same candidates but only one can claim each row.
func (r *AgentMessageWaitRepository) ClaimReadyForMessage(tenantID, runID, messageID, eventID string, now time.Time, limit int) ([]*agentrelation.AgentMessageWait, error) {
	if tenantID == "" {
		return nil, ErrTenantIDRequired
	}
	if runID == "" || messageID == "" || eventID == "" {
		return nil, errors.New("run_id, waiting_message_id and event_id are required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var candidates []*agentrelation.AgentMessageWait
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("run_id = ? AND waiting_message_id = ? AND status = ? AND (deadline_at IS NULL OR deadline_at > ?)", runID, messageID, agentrelation.MessageWaitStatusWaiting, now).
		Order("created_at ASC, id ASC").Limit(limit).Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	claimed := make([]*agentrelation.AgentMessageWait, 0, len(candidates))
	for _, candidate := range candidates {
		result := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
			Where("id = ? AND run_id = ? AND waiting_message_id = ? AND status = ? AND (deadline_at IS NULL OR deadline_at > ?)", candidate.ID, runID, messageID, agentrelation.MessageWaitStatusWaiting, now).
			Updates(map[string]any{"status": agentrelation.MessageWaitStatusResuming, "resume_event_id": eventID, "resume_attempts": gorm.Expr("resume_attempts + 1"), "claimed_at": now, "last_error": ""})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			row, getErr := r.Get(tenantID, candidate.ID)
			if getErr != nil {
				return nil, getErr
			}
			claimed = append(claimed, row)
		}
	}
	return claimed, nil
}

func (r *AgentMessageWaitRepository) CompleteResume(tenantID, waitID, eventID string, now time.Time) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	result := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("id = ? AND status = ? AND resume_event_id = ?", waitID, agentrelation.MessageWaitStatusResuming, eventID).
		Updates(map[string]any{"status": agentrelation.MessageWaitStatusResumed, "resumed_at": now, "last_error": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	// Completion is replay-safe only if this exact event already completed it.
	var count int64
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("id = ? AND status = ? AND resume_event_id = ?", waitID, agentrelation.MessageWaitStatusResumed, eventID).Count(&count).Error
	if err == nil && count == 1 {
		return nil
	}
	if err != nil {
		return err
	}
	return gorm.ErrRecordNotFound
}

func (r *AgentMessageWaitRepository) FailResume(tenantID, waitID, eventID, cause string, retryable bool, now time.Time) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	status := agentrelation.MessageWaitStatusCancelled
	updates := map[string]any{"status": status, "last_error": cause, "cancel_reason": cause, "cancelled_at": now}
	if retryable {
		updates = map[string]any{"status": agentrelation.MessageWaitStatusWaiting, "last_error": cause, "resume_event_id": "", "claimed_at": nil}
	}
	result := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("id = ? AND status = ? AND resume_event_id = ?", waitID, agentrelation.MessageWaitStatusResuming, eventID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AgentMessageWaitRepository) ExpireDue(tenantID string, now time.Time, limit int) (int64, error) {
	if tenantID == "" {
		return 0, ErrTenantIDRequired
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var ids []string
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("status = ? AND deadline_at IS NOT NULL AND deadline_at <= ?", agentrelation.MessageWaitStatusWaiting, now).
		Order("deadline_at ASC").Limit(limit).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	result := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("id IN ? AND status = ? AND deadline_at <= ?", ids, agentrelation.MessageWaitStatusWaiting, now).
		Updates(map[string]any{"status": agentrelation.MessageWaitStatusExpired, "expired_at": now})
	return result.RowsAffected, result.Error
}

func (r *AgentMessageWaitRepository) Cancel(tenantID, waitID, reason string, now time.Time) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	result := TenantOwned(r.db.Model(&agentrelation.AgentMessageWait{}), tenantID).
		Where("id = ? AND status IN ?", waitID, []string{agentrelation.MessageWaitStatusWaiting, agentrelation.MessageWaitStatusResuming}).
		Updates(map[string]any{"status": agentrelation.MessageWaitStatusCancelled, "cancel_reason": reason, "cancelled_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
