package repository

import (
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/personality"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type PersonalityRepository struct{ db *gorm.DB }

func NewPersonalityRepository() *PersonalityRepository {
	return &PersonalityRepository{db: database.GetDB()}
}

func (r *PersonalityRepository) List(tenantID string) ([]*personality.Template, error) {
	var rows []*personality.Template
	err := TenantOwned(r.db.Model(&personality.Template{}), tenantID).
		Order("is_builtin DESC, title ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *PersonalityRepository) GetByName(tenantID, name string) (*personality.Template, error) {
	var row personality.Template
	err := TenantOwned(r.db.Model(&personality.Template{}), tenantID).
		Where("name = ?", name).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PersonalityRepository) ListVersions(tenantID string, templateID uint64) ([]*personality.Version, error) {
	var rows []*personality.Version
	err := TenantOwned(r.db.Model(&personality.Version{}), tenantID).
		Where("template_id = ?", templateID).Order("version DESC").Find(&rows).Error
	return rows, err
}

func (r *PersonalityRepository) ExistsByName(tenantID, name string) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&personality.Template{}), tenantID).
		Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *PersonalityRepository) Create(tenantID string, row *personality.Template, initial *personality.Version) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		row.TenantID = tenantID
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		initial.TemplateID = row.ID
		initial.TenantID = tenantID
		return tx.Create(initial).Error
	})
}

func (r *PersonalityRepository) Update(tenantID string, row *personality.Template, nextVersion *personality.Version) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := TenantOwned(tx.Model(&personality.Template{}), tenantID).
			Where("id = ?", row.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		row.TenantID = tenantID
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		if nextVersion == nil {
			return nil
		}
		nextVersion.TemplateID = row.ID
		nextVersion.TenantID = tenantID
		return tx.Create(nextVersion).Error
	})
}

func (r *PersonalityRepository) Delete(tenantID string, row *personality.Template) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := TenantOwned(tx.Model(&personality.Template{}), tenantID).
			Where("id = ?", row.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := TenantOwned(tx, tenantID).Where("template_id = ?", row.ID).
			Delete(&personality.Version{}).Error; err != nil {
			return err
		}
		return TenantOwned(tx, tenantID).Where("id = ?", row.ID).
			Delete(&personality.Template{}).Error
	})
}

func (r *PersonalityRepository) CountAgentUsage(tenantID, name string) (int64, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agent.AgentConfig{}), tenantID).
		Where("personality_template_name = ?", name).Count(&count).Error
	return count, err
}
