package repository

import (
	"control-panel/internal/domain/agentrelation"
	"control-panel/pkg/database"

	"gorm.io/gorm"
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
	var count int64
	if err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).
		Where("id = ?", relation.ID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	relation.TenantID = tenantID
	return r.db.Omit("SourceAgent", "TargetAgent").Save(relation).Error
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

type AgentMessageRepository struct {
	db *gorm.DB
}

func NewAgentMessageRepository() *AgentMessageRepository {
	return &AgentMessageRepository{db: database.GetDB()}
}

func NewAgentMessageRepositoryWithDB(db *gorm.DB) *AgentMessageRepository {
	return &AgentMessageRepository{db: db}
}

func (r *AgentMessageRepository) Create(tenantID string, message *agentrelation.AgentMessage) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	message.TenantID = tenantID
	return r.db.Create(message).Error
}

func (r *AgentMessageRepository) Save(tenantID string, message *agentrelation.AgentMessage) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	message.TenantID = tenantID
	result := TenantOwned(r.db, tenantID).Where("id = ?", message.ID).Updates(message)
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
