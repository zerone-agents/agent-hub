package repository

import (
	"control-panel/internal/domain/provider"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ProviderRepository) GetMultiRAGBinding(tenantID string, id uint64, target string) (*provider.MultiRAGSyncBinding, error) {
	if err := r.mustOwnProvider(r.db, tenantID, id); err != nil {
		return nil, err
	}
	var binding provider.MultiRAGSyncBinding
	err := r.db.Where("provider_id = ? AND target = ?", id, target).First(&binding).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &binding, err
}

func (r *ProviderRepository) ReserveMultiRAGBinding(tenantID string, binding *provider.MultiRAGSyncBinding) (bool, error) {
	if err := r.mustOwnProvider(r.db, tenantID, binding.ProviderID); err != nil {
		return false, err
	}
	result := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(binding)
	return result.RowsAffected == 1, result.Error
}

func (r *ProviderRepository) ConfirmMultiRAGBinding(tenantID string, binding *provider.MultiRAGSyncBinding) error {
	if err := r.mustOwnProvider(r.db, tenantID, binding.ProviderID); err != nil {
		return err
	}
	result := r.db.Model(&provider.MultiRAGSyncBinding{}).
		Where("provider_id = ? AND target = ? AND instance_name = ? AND state = ? AND version = ? AND lease = ?", binding.ProviderID, binding.Target, binding.InstanceName, binding.State, binding.Version, binding.Lease).
		Updates(map[string]any{"instance_id": binding.InstanceID, "config_hash": binding.ConfigHash, "state": "ready", "lease": "", "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProviderRepository) ReleaseMultiRAGBinding(tenantID string, binding *provider.MultiRAGSyncBinding) error {
	if err := r.mustOwnProvider(r.db, tenantID, binding.ProviderID); err != nil {
		return err
	}
	return r.db.Where("provider_id = ? AND target = ? AND instance_name = ? AND state = ?", binding.ProviderID, binding.Target, binding.InstanceName, "creating").Delete(&provider.MultiRAGSyncBinding{}).Error
}

// A durable CAS owns the complete update/readback window across Hub processes.
// No expiring lease: public has_api_key cannot prove an unknown credential PATCH.
func (r *ProviderRepository) BeginMultiRAGSync(tenantID string, binding *provider.MultiRAGSyncBinding) (bool, error) {
	if err := r.mustOwnProvider(r.db, tenantID, binding.ProviderID); err != nil {
		return false, err
	}
	result := r.db.Model(&provider.MultiRAGSyncBinding{}).
		Where("provider_id = ? AND target = ? AND state = ? AND version = ?", binding.ProviderID, binding.Target, "ready", binding.Version).
		Updates(map[string]any{"state": "syncing", "lease": binding.Lease, "config_hash": binding.ConfigHash, "version": gorm.Expr("version + 1")})
	return result.RowsAffected == 1, result.Error
}
