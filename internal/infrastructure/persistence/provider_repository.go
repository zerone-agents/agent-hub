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

// mustOwnProvider 写路径统一入口校验（同 Task 3 的 mustOwnAgent 模式）：
// provider 不属于该租户（含共享行——共享模板只读，写前必须 copy-on-write
// 复制为本租户行）则返回 gorm.ErrRecordNotFound，不暴露存在性。
// 子表方法（attributes/models，按 provider_id）不加 tenant 列，写路径
// 前置该校验继承主表归属。
func (r *ProviderRepository) mustOwnProvider(tx *gorm.DB, tenantID string, providerID uint64) error {
	var count int64
	err := TenantOwned(tx.Model(&provider.ProviderSummary{}), tenantID).
		Where("id = ?", providerID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// canReadProvider 读路径校验：本租户 + 共享行（tenant_id=”）可读，
// 他租户返回 gorm.ErrRecordNotFound。
func (r *ProviderRepository) canReadProvider(tx *gorm.DB, tenantID string, providerID uint64) error {
	var count int64
	err := TenantWithShared(tx.Model(&provider.ProviderSummary{}), tenantID).
		Where("id = ?", providerID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ── Summary (primary table) ──

func (r *ProviderRepository) ListAll(tenantID string) ([]*provider.ProviderSummary, error) {
	var items []*provider.ProviderSummary
	err := TenantWithShared(r.db.Model(&provider.ProviderSummary{}), tenantID).
		Order("id ASC").Find(&items).Error
	return items, err
}

func (r *ProviderRepository) GetByID(tenantID string, id uint64) (*provider.ProviderSummary, error) {
	var p provider.ProviderSummary
	err := TenantWithShared(r.db.Model(&provider.ProviderSummary{}), tenantID).
		Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Create 写入前强制盖章 TenantID——调用方传入的 TenantID 不可信。
// tenantID 传空串即系统路径（SeedIfEmpty 写共享种子模板）。
func (r *ProviderRepository) Create(tenantID string, p *provider.ProviderSummary) error {
	p.TenantID = tenantID
	return r.db.Create(p).Error
}

// Update 先校验归属（跨租户/共享行返回 ErrRecordNotFound，不暴露存在性），
// 再盖章 TenantID 后保存。
func (r *ProviderRepository) Update(tenantID string, p *provider.ProviderSummary) error {
	if err := r.mustOwnProvider(r.db, tenantID, p.ID); err != nil {
		return err
	}
	p.TenantID = tenantID
	return r.db.Save(p).Error
}

func (r *ProviderRepository) Delete(tenantID string, id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 入口校验：不属于该租户（含共享模板）则整体失败，
		// 子表级联删除也不会发生。
		if err := r.mustOwnProvider(tx, tenantID, id); err != nil {
			return err
		}
		if err := tx.Where("provider_id = ?", id).Delete(&provider.ProviderAttribute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("provider_id = ?", id).Delete(&provider.ProviderModel{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&provider.ProviderSummary{}).Error
	})
}

func (r *ProviderRepository) ExistsByKey(tenantID string, key string) (bool, error) {
	var count int64
	err := TenantWithShared(r.db.Model(&provider.ProviderSummary{}), tenantID).
		Where("`key` = ?", key).Count(&count).Error
	return count > 0, err
}

// Count 跨租户全表计数。仅限无租户上下文的系统路径：启动期 SeedIfEmpty
// 判断表是否全空。业务请求必须走 ListAll(tenantID)。
func (r *ProviderRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&provider.ProviderSummary{}).Count(&count).Error
	return count, err
}

// CopyForTenant copy-on-write：把共享模板（tenant_id=”，含 attributes 与
// models 子表行）在一个事务里完整复制为本租户行，返回新 summary。
// 他租户的行不可复制（ErrRecordNotFound）。
func (r *ProviderRepository) CopyForTenant(tenantID string, srcID uint64) (*provider.ProviderSummary, error) {
	var copied *provider.ProviderSummary
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var src provider.ProviderSummary
		// 只允许复制共享行：tenantID 与空串 scope 命中且原行 tenant_id 为空。
		if err := TenantWithShared(tx, tenantID).Where("id = ?", srcID).First(&src).Error; err != nil {
			return err
		}
		if src.TenantID != "" {
			// 命中了本租户已有的行——无需复制，直接返回。
			copied = &src
			return nil
		}
		clone := src
		clone.ID = 0
		clone.TenantID = tenantID
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}

		var attrs []provider.ProviderAttribute
		if err := tx.Where("provider_id = ?", srcID).Find(&attrs).Error; err != nil {
			return err
		}
		for i := range attrs {
			attrs[i].ID = 0
			attrs[i].ProviderID = clone.ID
		}
		if len(attrs) > 0 {
			if err := tx.Create(&attrs).Error; err != nil {
				return err
			}
		}

		var models []provider.ProviderModel
		if err := tx.Where("provider_id = ?", srcID).Find(&models).Error; err != nil {
			return err
		}
		for i := range models {
			models[i].ID = 0
			models[i].ProviderID = clone.ID
		}
		if len(models) > 0 {
			if err := tx.Create(&models).Error; err != nil {
				return err
			}
		}
		copied = &clone
		return nil
	})
	if err != nil {
		return nil, err
	}
	return copied, nil
}

// ── Attributes (EAV table) ──

// GetAttributes loads all attributes for a provider into a key→value map.
func (r *ProviderRepository) GetAttributes(tenantID string, providerID uint64) (map[string]provider.AttrValue, error) {
	if err := r.canReadProvider(r.db, tenantID, providerID); err != nil {
		return nil, err
	}
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
func (r *ProviderRepository) SetAttributes(tenantID string, providerID uint64, attrs map[string]provider.AttrValue) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := r.mustOwnProvider(tx, tenantID, providerID); err != nil {
			return err
		}
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

func (r *ProviderRepository) ListModels(tenantID string, providerID uint64) ([]provider.ProviderModel, error) {
	if err := r.canReadProvider(r.db, tenantID, providerID); err != nil {
		return nil, err
	}
	var rows []provider.ProviderModel
	err := r.db.Where("provider_id = ?", providerID).Order("sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *ProviderRepository) ListAllModels(tenantID string) ([]provider.ProviderModel, error) {
	var rows []provider.ProviderModel
	sub := TenantWithShared(r.db.Model(&provider.ProviderSummary{}), tenantID).Select("id")
	err := r.db.Where("provider_id IN (?)", sub).
		Order("provider_id ASC, sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

// ListAllModelsUnscoped 跨租户全表列出 provider_models。仅限无租户上下文
// 的系统链路：AIGC 模型码全局映射对账（aigc_config_service，模型码
// 0001=GLM-4.5 等按 model_id 全局复用，与租户无关）。业务请求必须走
// ListAllModels(tenantID)。
func (r *ProviderRepository) ListAllModelsUnscoped() ([]provider.ProviderModel, error) {
	var rows []provider.ProviderModel
	err := r.db.Order("provider_id ASC, sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *ProviderRepository) GetModelBySelectionID(tenantID string, providerID uint64, selectionID string) (*provider.ProviderModel, error) {
	if err := r.canReadProvider(r.db, tenantID, providerID); err != nil {
		return nil, err
	}
	var m provider.ProviderModel
	err := r.db.Where("provider_id = ? AND selection_id = ?", providerID, selectionID).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *ProviderRepository) CreateModel(tenantID string, m *provider.ProviderModel) error {
	if err := r.mustOwnProvider(r.db, tenantID, m.ProviderID); err != nil {
		return err
	}
	return r.db.Create(m).Error
}

func (r *ProviderRepository) UpdateModel(tenantID string, m *provider.ProviderModel) error {
	if err := r.mustOwnProvider(r.db, tenantID, m.ProviderID); err != nil {
		return err
	}
	return r.db.Save(m).Error
}

func (r *ProviderRepository) DeleteModel(tenantID string, providerID uint64, selectionID string) error {
	if err := r.mustOwnProvider(r.db, tenantID, providerID); err != nil {
		return err
	}
	return r.db.Where("provider_id = ? AND selection_id = ?", providerID, selectionID).
		Delete(&provider.ProviderModel{}).Error
}

// ReplaceModels transactionally replaces all models for a provider.
func (r *ProviderRepository) ReplaceModels(tenantID string, providerID uint64, models []provider.ProviderModel) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := r.mustOwnProvider(tx, tenantID, providerID); err != nil {
			return err
		}
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
