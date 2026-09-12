package repository

import (
	"errors"

	"control-panel/internal/domain/agentrelation"
	"control-panel/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentRelationRepository struct {
	db *gorm.DB
}

func NewAgentRelationRepository() *AgentRelationRepository {
	return &AgentRelationRepository{db: database.GetDB()}
}

func NewAgentRelationRepositoryWithDB(db *gorm.DB) *AgentRelationRepository {
	return &AgentRelationRepository{db: db}
}

func (r *AgentRelationRepository) DB() *gorm.DB { return r.db }

func (r *AgentRelationRepository) ListAll(tenantID string) ([]*agentrelation.AgentRelation, error) {
	var relations []*agentrelation.AgentRelation
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Preload("SourceAgent").Preload("TargetAgent").
		Order("scope ASC, source_agent_id ASC, target_agent_id ASC").
		Find(&relations).Error
	return relations, err
}

func (r *AgentRelationRepository) GetByID(tenantID string, id uint64) (*agentrelation.AgentRelation, error) {
	var relation agentrelation.AgentRelation
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Preload("SourceAgent").Preload("TargetAgent").
		Where("id = ?", id).First(&relation).Error
	if err != nil {
		return nil, err
	}
	return &relation, nil
}

func (r *AgentRelationRepository) ExistsEdge(tenantID, scope string, sourceAgentID, targetAgentID uint64) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Where("scope = ? AND source_agent_id = ? AND target_agent_id = ?", scope, sourceAgentID, targetAgentID).
		Count(&count).Error
	return count > 0, err
}

// GetEnabledEdge resolves the exact directed policy used to authorize a
// runtime message. Disabled rows intentionally behave as missing routes.
func (r *AgentRelationRepository) GetEnabledEdge(tenantID, scope string, sourceAgentID, targetAgentID uint64) (*agentrelation.AgentRelation, error) {
	var relation agentrelation.AgentRelation
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Preload("SourceAgent").Preload("TargetAgent").
		Where("scope = ? AND source_agent_id = ? AND target_agent_id = ? AND enabled = ?", scope, sourceAgentID, targetAgentID, true).
		First(&relation).Error
	if err != nil {
		return nil, err
	}
	return &relation, nil
}

// FindEnabledEdge is the quiet optional variant used for relationship context:
// a missing reverse edge is normal and must not be logged as a database error.
func (r *AgentRelationRepository) FindEnabledEdge(tenantID, scope string, sourceAgentID, targetAgentID uint64) (*agentrelation.AgentRelation, error) {
	var relation agentrelation.AgentRelation
	result := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Preload("SourceAgent").Preload("TargetAgent").
		Where("scope = ? AND source_agent_id = ? AND target_agent_id = ? AND enabled = ?", scope, sourceAgentID, targetAgentID, true).
		Limit(1).Find(&relation)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &relation, nil
}

// ListEnabledForAgent returns both incoming and outgoing edges so the runtime
// can understand its local organization view without exposing other tenants.
func (r *AgentRelationRepository) ListEnabledForAgent(tenantID string, agentID uint64) ([]*agentrelation.AgentRelation, error) {
	var relations []*agentrelation.AgentRelation
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Preload("SourceAgent").Preload("TargetAgent").
		Where("enabled = ? AND (source_agent_id = ? OR target_agent_id = ?)", true, agentID, agentID).
		Order("scope ASC, source_agent_id ASC, target_agent_id ASC").
		Find(&relations).Error
	return relations, err
}

func (r *AgentRelationRepository) HasEnabledForAgent(tenantID string, agentID uint64) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Where("enabled = ? AND (source_agent_id = ? OR target_agent_id = ?)", true, agentID, agentID).
		Count(&count).Error
	return count > 0, err
}

func (r *AgentRelationRepository) CreateMany(tenantID string, relations []*agentrelation.AgentRelation) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, relation := range relations {
			relation.TenantID = tenantID
			if err := tx.Omit("SourceAgent", "TargetAgent").Create(relation).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AgentRelationRepository) Update(tenantID string, relation *agentrelation.AgentRelation) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.updateStructuralFields(r.db, tenantID, relation)
}

// UpdateAndResetScore applies a structural edit and an administrator-requested
// relationship baseline reset atomically. The absolute target is calculated
// while holding the relationship row lock, so a concurrent event cannot be
// accidentally overwritten by a stale score from the edit form.
func (r *AgentRelationRepository) UpdateAndResetScore(tenantID string, relation *agentrelation.AgentRelation, event *agentrelation.AgentRelationEvent, targetScore int) (*agentrelation.AgentRelation, *agentrelation.AgentRelationEvent, error) {
	if tenantID == "" {
		return nil, nil, ErrTenantIDRequired
	}
	var updated *agentrelation.AgentRelation
	var applied *agentrelation.AgentRelationEvent
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := r.updateStructuralFields(tx, tenantID, relation); err != nil {
			return err
		}
		var err error
		updated, applied, err = r.applyEvent(tx, tenantID, event, &targetScore)
		return err
	})
	return updated, applied, err
}

func (r *AgentRelationRepository) updateStructuralFields(db *gorm.DB, tenantID string, relation *agentrelation.AgentRelation) error {
	relation.TenantID = tenantID
	result := TenantOwned(db.Model(&agentrelation.AgentRelation{}), tenantID).
		Where("id = ?", relation.ID).
		Select("scope", "relation_type", "allowed_actions", "context_policy", "delivery_policy", "constraint", "enabled", "updated_at").
		Updates(relation)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AgentRelationRepository) Delete(tenantID string, id uint64) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	result := TenantOwned(r.db, tenantID).Where("id = ?", id).Delete(&agentrelation.AgentRelation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ApplyEvent appends an immutable event and updates the materialized score in
// one transaction. The row lock serializes simultaneous observations about the
// same directed relationship on databases that support SELECT FOR UPDATE.
func (r *AgentRelationRepository) ApplyEvent(tenantID string, event *agentrelation.AgentRelationEvent) (*agentrelation.AgentRelation, *agentrelation.AgentRelationEvent, error) {
	if tenantID == "" {
		return nil, nil, ErrTenantIDRequired
	}
	var updated *agentrelation.AgentRelation
	var applied *agentrelation.AgentRelationEvent
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var err error
		updated, applied, err = r.applyEvent(tx, tenantID, event, nil)
		return err
	})
	return updated, applied, err
}

func (r *AgentRelationRepository) applyEvent(tx *gorm.DB, tenantID string, event *agentrelation.AgentRelationEvent, targetScore *int) (*agentrelation.AgentRelation, *agentrelation.AgentRelationEvent, error) {
	// Lock before checking idempotency. Concurrent retries for the same edge
	// then serialize here instead of both observing a missing event and racing
	// on the unique key during insert.
	var relation agentrelation.AgentRelation
	if err := TenantOwned(tx.Model(&agentrelation.AgentRelation{}), tenantID).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("SourceAgent").Preload("TargetAgent").
		Where("id = ?", event.RelationID).First(&relation).Error; err != nil {
		return nil, nil, err
	}

	if event.IdempotencyKey != "" {
		var existing agentrelation.AgentRelationEvent
		err := TenantOwned(tx.Model(&agentrelation.AgentRelationEvent{}), tenantID).
			Where("relation_id = ? AND idempotency_key = ?", event.RelationID, event.IdempotencyKey).First(&existing).Error
		if err == nil {
			return &relation, &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
	}

	event.TenantID = tenantID
	event.Scope = relation.Scope
	event.SourceAgentID = relation.SourceAgentID
	event.TargetAgentID = relation.TargetAgentID
	event.ScoreBefore = relation.RelationshipScore
	event.StanceBefore = relation.Stance
	if targetScore != nil {
		event.ScoreAfter = agentrelation.ClampRelationshipScore(*targetScore)
		event.Delta = event.ScoreAfter - event.ScoreBefore
	} else {
		event.ScoreAfter = agentrelation.ClampRelationshipScore(event.ScoreBefore + event.Delta)
	}
	event.StanceAfter = agentrelation.StanceForScore(event.ScoreAfter)

	relation.RelationshipScore = event.ScoreAfter
	relation.Stance = event.StanceAfter
	relation.LastChangedAt = &event.OccurredAt
	if err := TenantOwned(tx.Model(&agentrelation.AgentRelation{}), tenantID).
		Where("id = ?", relation.ID).
		Updates(map[string]interface{}{
			"relationship_score": relation.RelationshipScore,
			"stance":             relation.Stance,
			"last_changed_at":    relation.LastChangedAt,
		}).Error; err != nil {
		return nil, nil, err
	}
	if err := tx.Create(event).Error; err != nil {
		return nil, nil, err
	}
	return &relation, event, nil
}

func (r *AgentRelationRepository) ListEvents(tenantID string, relationID uint64, limit int) ([]*agentrelation.AgentRelationEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var events []*agentrelation.AgentRelationEvent
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelationEvent{}), tenantID).
		Where("relation_id = ?", relationID).
		Order("occurred_at DESC, created_at DESC").
		Limit(limit).Find(&events).Error
	return events, err
}

type AgentMessageRepository struct {
	db *gorm.DB
}

func NewAgentMessageRepository() *AgentMessageRepository {
	return &AgentMessageRepository{db: database.GetDB()}
}

func NewAgentMessageRepositoryWithDB(db *gorm.DB) *AgentMessageRepository {
	return &AgentMessageRepository{db: db}
}

func (r *AgentMessageRepository) DB() *gorm.DB { return r.db }

func (r *AgentMessageRepository) Create(tenantID string, message *agentrelation.AgentMessage) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	message.TenantID = tenantID
	return r.db.Create(message).Error
}

// CreateIdempotent reserves the caller-scoped key and writes the message in
// one transaction. It closes the concurrent check-then-insert race without
// imposing a unique index on legacy agent_messages rows.
func (r *AgentMessageRepository) CreateIdempotent(tenantID string, message *agentrelation.AgentMessage) (*agentrelation.AgentMessage, bool, error) {
	if tenantID == "" {
		return nil, false, ErrTenantIDRequired
	}
	message.TenantID = tenantID
	var result *agentrelation.AgentMessage
	created := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, created, err = r.CreateIdempotentTx(tx, tenantID, message)
		return err
	})
	return result, created, err
}

func (r *AgentMessageRepository) CreateIdempotentTx(tx *gorm.DB, tenantID string, message *agentrelation.AgentMessage) (*agentrelation.AgentMessage, bool, error) {
	if tenantID == "" {
		return nil, false, ErrTenantIDRequired
	}
	message.TenantID = tenantID
	var result *agentrelation.AgentMessage
	created := false
	reservation := agentrelation.AgentMessageDedupe{TenantID: tenantID, SourceAgentID: message.SourceAgentID, IdempotencyKey: message.IdempotencyKey, MessageID: message.ID}
	insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reservation)
	if insert.Error != nil {
		return nil, false, insert.Error
	}
	if insert.RowsAffected == 0 {
		var existingReservation agentrelation.AgentMessageDedupe
		if err := tx.Where("tenant_id=? AND source_agent_id=? AND idempotency_key=?", tenantID, message.SourceAgentID, message.IdempotencyKey).First(&existingReservation).Error; err != nil {
			return nil, false, err
		}
		var existing agentrelation.AgentMessage
		if err := tx.Where("tenant_id=? AND id=?", tenantID, existingReservation.MessageID).First(&existing).Error; err != nil {
			return nil, false, err
		}
		result = &existing
		return result, false, nil
	}
	if err := tx.Create(message).Error; err != nil {
		return nil, false, err
	}
	result, created = message, true
	return result, created, nil
}

func (r *AgentMessageRepository) FindByIdempotencyKey(tenantID, key string, sourceAgentID uint64) (*agentrelation.AgentMessage, error) {
	var message agentrelation.AgentMessage
	result := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).
		Where("idempotency_key = ? AND source_agent_id = ?", key, sourceAgentID).Limit(1).Find(&message)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &message, nil
}

func (r *AgentMessageRepository) Save(tenantID string, message *agentrelation.AgentMessage) error {
	return r.SaveTx(r.db, tenantID, message)
}

func (r *AgentMessageRepository) SaveTx(tx *gorm.DB, tenantID string, message *agentrelation.AgentMessage) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	message.TenantID = tenantID
	result := TenantOwned(tx, tenantID).Where("id = ?", message.ID).Updates(message)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AgentMessageRepository) GetForAgent(tenantID, id string, agentID uint64) (*agentrelation.AgentMessage, error) {
	var message agentrelation.AgentMessage
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).
		Where("id = ? AND (source_agent_id = ? OR target_agent_id = ?)", id, agentID, agentID).
		First(&message).Error
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *AgentMessageRepository) ListForAgent(tenantID string, agentID uint64, limit int) ([]*agentrelation.AgentMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var messages []*agentrelation.AgentMessage
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).
		Where("source_agent_id = ? OR target_agent_id = ?", agentID, agentID).
		Order("created_at DESC").Limit(limit).Find(&messages).Error
	return messages, err
}

func (r *AgentMessageRepository) ListForRun(tenantID, runID string, limit int) ([]*agentrelation.AgentMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var messages []*agentrelation.AgentMessage
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).
		Where("run_id = ?", runID).Order("created_at ASC, id ASC").Limit(limit).Find(&messages).Error
	return messages, err
}

func (r *AgentMessageRepository) ValidateRunParticipants(tenantID, runID string, sourceAgentID, targetAgentID uint64) (bool, bool, error) {
	var runCount int64
	if err := r.db.Table("runs").Where("tenant_id = ? AND id = ? AND status = ?", tenantID, runID, "running").Count(&runCount).Error; err != nil {
		return false, false, err
	}
	if runCount != 1 {
		return false, false, nil
	}
	var participantCount int64
	if err := r.db.Table("run_agents").Where("tenant_id = ? AND run_id = ? AND agent_id IN ?", tenantID, runID, []uint64{sourceAgentID, targetAgentID}).Distinct("agent_id").Count(&participantCount).Error; err != nil {
		return true, false, err
	}
	if participantCount != 2 {
		return true, false, nil
	}
	return true, true, nil
}

func (r *AgentMessageRepository) HasRunningSourceInChain(tenantID, rootMessageID string, agentID uint64) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agentrelation.AgentMessage{}), tenantID).
		Where("root_message_id = ? AND source_agent_id = ? AND status = ?", rootMessageID, agentID, agentrelation.MessageStatusRunning).
		Count(&count).Error
	return count > 0, err
}
