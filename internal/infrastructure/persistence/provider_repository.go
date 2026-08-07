package repository

import (
	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type ProviderRepository struct {
	db *gorm.DB
}

func NewProviderRepository() *ProviderRepository {
	return &ProviderRepository{db: database.GetDB()}
}

// ── Summary (primary table) ──

func (r *ProviderRepository) ListAll() ([]*provider.ProviderSummary, error) {
	var items []*provider.ProviderSummary
	err := r.db.Order("id ASC").Find(&items).Error
	return items, err
}

func (r *ProviderRepository) GetByID(id uint64) (*provider.ProviderSummary, error) {
	var p provider.ProviderSummary
	err := r.db.Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProviderRepository) Create(p *provider.ProviderSummary) error {
	return r.db.Create(p).Error
}

func (r *ProviderRepository) Update(p *provider.ProviderSummary) error {
	return r.db.Save(p).Error
}

func (r *ProviderRepository) Delete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", id).Delete(&provider.ProviderAttribute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("provider_id = ?", id).Delete(&provider.ProviderModel{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&provider.ProviderSummary{}).Error
	})
}

func (r *ProviderRepository) ExistsByKey(key string) (bool, error) {
	var count int64
	err := r.db.Model(&provider.ProviderSummary{}).Where("`key` = ?", key).Count(&count).Error
	return count > 0, err
}

func (r *ProviderRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&provider.ProviderSummary{}).Count(&count).Error
	return count, err
}

// ── Attributes (EAV table) ──

// GetAttributes loads all attributes for a provider into a key→value map.
func (r *ProviderRepository) GetAttributes(providerID uint64) (map[string]provider.AttrValue, error) {
	var rows []provider.ProviderAttribute
	if err := r.db.Where("provider_id = ?", providerID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]provider.AttrValue, len(rows))
	for _, row := range rows {
		out[row.AttrKey] = provider.AttrValue{Type: row.AttrType, Value: row.AttrValue}
	}
	return out, nil
}

// SetAttributes upserts the full attribute set for a provider, replacing
// any existing attributes.
func (r *ProviderRepository) SetAttributes(providerID uint64, attrs map[string]provider.AttrValue) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", providerID).Delete(&provider.ProviderAttribute{}).Error; err != nil {
			return err
		}
		rows := make([]provider.ProviderAttribute, 0, len(attrs))
		for k, v := range attrs {
			rows = append(rows, provider.ProviderAttribute{
				ProviderID: providerID,
				AttrKey:    k,
				AttrType:   v.Type,
				AttrValue:  v.Value,
			})
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ── Models (normalized child table) ──

func (r *ProviderRepository) ListModels(providerID uint64) ([]provider.ProviderModel, error) {
	var rows []provider.ProviderModel
	err := r.db.Where("provider_id = ?", providerID).Order("sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *ProviderRepository) ListAllModels() ([]provider.ProviderModel, error) {
	var rows []provider.ProviderModel
	err := r.db.Order("provider_id ASC, sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *ProviderRepository) GetModelBySelectionID(providerID uint64, selectionID string) (*provider.ProviderModel, error) {
	var m provider.ProviderModel
	err := r.db.Where("provider_id = ? AND selection_id = ?", providerID, selectionID).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *ProviderRepository) CreateModel(m *provider.ProviderModel) error {
	return r.db.Create(m).Error
}

func (r *ProviderRepository) UpdateModel(m *provider.ProviderModel) error {
	return r.db.Save(m).Error
}

func (r *ProviderRepository) DeleteModel(providerID uint64, selectionID string) error {
	return r.db.Where("provider_id = ? AND selection_id = ?", providerID, selectionID).
		Delete(&provider.ProviderModel{}).Error
}

// ReplaceModels transactionally replaces all models for a provider.
func (r *ProviderRepository) ReplaceModels(providerID uint64, models []provider.ProviderModel) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", providerID).Delete(&provider.ProviderModel{}).Error; err != nil {
			return err
		}
		for i := range models {
			models[i].ProviderID = providerID
			if models[i].Status == "" {
				models[i].Status = "1"
			}
			if err := tx.Create(&models[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
