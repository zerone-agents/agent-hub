package services

import "control-panel/internal/domain/provider"

// The old API has no instance identity. Only the read-only route probe may
// select it, and same-name local providers must not overwrite each other there.
func (s *ProviderService) validateLegacySyncIdentity(tenantID string, p provider.Provider) error {
	active := []provider.CatalogModel{}
	selected := map[string]bool{}
	for _, model := range p.DefaultModels() {
		if model.Status == "0" {
			continue
		}
		if selected[model.ModelID] {
			return provider.NewValidationErrorf("旧版MultiRAG不支持同名多能力实例，请升级后同步")
		}
		selected[model.ModelID] = true
		active = append(active, model)
	}
	rows, err := s.repo.ListAll(tenantID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == p.ID() {
			continue
		}
		other, err := provider.NewProviderFromSummary(row)
		if err != nil {
			return err
		}
		if other.MultiRAGFactoryName() != p.MultiRAGFactoryName() {
			continue
		}
		models, err := s.repo.ListModels(tenantID, row.ID)
		if err != nil {
			return err
		}
		for _, model := range models {
			if model.Status != "0" && selected[model.ModelID] {
				return provider.NewValidationErrorf("旧版MultiRAG无法区分同名供应商实例，请升级后同步")
			}
		}
	}
	p.Base().SetDefaultModels(active)
	return nil
}
