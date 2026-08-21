package repository

import (
	"control-panel/internal/domain/scene"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type SceneRepository struct {
	db *gorm.DB
}

func NewSceneRepository() *SceneRepository {
	return &SceneRepository{db: database.GetDB()}
}

// mustOwnScene 写路径统一入口校验（同 mustOwnAgent 模式）：scene 不属于该
// 租户则返回 gorm.ErrRecordNotFound，不暴露存在性。
func (r *SceneRepository) mustOwnScene(tx *gorm.DB, tenantID string, sceneID uint64) error {
	if tenantID == "" {
		return nil // 系统路径
	}
	var count int64
	err := TenantOwned(tx.Model(&scene.Scene{}), tenantID).
		Where("id = ?", sceneID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListByAgent 返回本租户内某 Agent 的场景（agent 归属已由 agent 表租户
// 校验兜底，这里再按 scene 自身 tenant 过滤）。
func (r *SceneRepository) ListByAgent(tenantID string, agentID uint64) ([]*scene.Scene, error) {
	var scenes []*scene.Scene
	err := TenantOwned(r.db.Model(&scene.Scene{}), tenantID).
		Where("agent_id = ?", agentID).Order("id ASC").Find(&scenes).Error
	return scenes, err
}

func (r *SceneRepository) ListAll(tenantID string) ([]*scene.Scene, error) {
	var scenes []*scene.Scene
	err := TenantOwned(r.db.Model(&scene.Scene{}), tenantID).
		Order("id ASC").Find(&scenes).Error
	return scenes, err
}

func (r *SceneRepository) GetByName(tenantID, name string) (*scene.Scene, error) {
	var s scene.Scene
	err := TenantOwned(r.db.Model(&scene.Scene{}), tenantID).
		Where("name = ?", name).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create 写入前强制盖章 TenantID——调用方传入的 TenantID 不可信。
func (r *SceneRepository) Create(tenantID string, s *scene.Scene) error {
	s.TenantID = tenantID
	return r.db.Create(s).Error
}

// Update 先校验归属（跨租户返回 ErrRecordNotFound），再盖章保存。
func (r *SceneRepository) Update(tenantID string, s *scene.Scene) error {
	if err := r.mustOwnScene(r.db, tenantID, s.ID); err != nil {
		return err
	}
	s.TenantID = tenantID
	return r.db.Save(s).Error
}

func (r *SceneRepository) Delete(tenantID string, id uint64) error {
	if err := r.mustOwnScene(r.db, tenantID, id); err != nil {
		return err
	}
	return r.db.Where("id = ?", id).Delete(&scene.Scene{}).Error
}

func (r *SceneRepository) ExistsByName(tenantID, name string) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&scene.Scene{}), tenantID).
		Where("name = ?", name).Count(&count).Error
	return count > 0, err
}
