package repository

import (
	"context"

	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type DeploymentSnapshotRepository struct {
	db *gorm.DB
}

func NewDeploymentSnapshotRepository() *DeploymentSnapshotRepository {
	return &DeploymentSnapshotRepository{db: database.GetDB()}
}

func NewDeploymentSnapshotRepositoryWithDB(db *gorm.DB) *DeploymentSnapshotRepository {
	return &DeploymentSnapshotRepository{db: db}
}

func (r *DeploymentSnapshotRepository) Upsert(ctx context.Context, s *agent.DeploymentSnapshot) error {
	return r.db.WithContext(ctx).
		Save(s). // GORM Save 对主键存在时 UPDATE、不存在时 INSERT → upsert
		Error
}

func (r *DeploymentSnapshotRepository) GetByAgent(ctx context.Context, tenantID string, agentID uint64) (*agent.DeploymentSnapshot, error) {
	var s agent.DeploymentSnapshot
	if err := r.db.WithContext(ctx).
		Where("agent_id = ? AND tenant_id = ?", agentID, tenantID).
		First(&s).Error; err != nil {
		return nil, err // gorm.ErrRecordNotFound 原样透传
	}
	return &s, nil
}
