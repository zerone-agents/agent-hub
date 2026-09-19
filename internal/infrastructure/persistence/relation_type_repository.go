package repository

import (
	"control-panel/internal/domain/agentrelation"
	"control-panel/pkg/database"
	"gorm.io/gorm"
)

type RelationTypeRepository struct{ db *gorm.DB }

func NewRelationTypeRepository() *RelationTypeRepository {
	return &RelationTypeRepository{db: database.GetDB()}
}

func (r *RelationTypeRepository) List(tenantID string) ([]*agentrelation.RelationTypeTemplate, error) {
	var rows []*agentrelation.RelationTypeTemplate
	err := TenantOwned(r.db.Model(&agentrelation.RelationTypeTemplate{}), tenantID).Order("is_builtin DESC, title ASC").Find(&rows).Error
	return rows, err
}

func (r *RelationTypeRepository) Get(tenantID, name string) (*agentrelation.RelationTypeTemplate, error) {
	var row agentrelation.RelationTypeTemplate
	err := TenantOwned(r.db.Model(&row), tenantID).Where("name = ?", name).First(&row).Error
	return &row, err
}

func (r *RelationTypeRepository) Create(tenantID string, row *agentrelation.RelationTypeTemplate, version *agentrelation.RelationTypeVersion) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		row.TenantID = tenantID
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		version.TemplateID, version.TenantID = row.ID, tenantID
		return tx.Create(version).Error
	})
}

func (r *RelationTypeRepository) Update(tenantID string, row *agentrelation.RelationTypeTemplate, version *agentrelation.RelationTypeVersion) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		row.TenantID = tenantID
		if err := TenantOwned(tx, tenantID).Save(row).Error; err != nil {
			return err
		}
		if version == nil {
			return nil
		}
		version.TemplateID, version.TenantID = row.ID, tenantID
		return tx.Create(version).Error
	})
}

func (r *RelationTypeRepository) Delete(tenantID string, row *agentrelation.RelationTypeTemplate) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := TenantOwned(tx, tenantID).Where("template_id = ?", row.ID).Delete(&agentrelation.RelationTypeVersion{}).Error; err != nil {
			return err
		}
		return TenantOwned(tx, tenantID).Delete(row).Error
	})
}

func (r *RelationTypeRepository) CountUsage(tenantID, name string) (int64, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agentrelation.AgentRelation{}), tenantID).Where("relation_type_template_name = ?", name).Count(&count).Error
	return count, err
}
