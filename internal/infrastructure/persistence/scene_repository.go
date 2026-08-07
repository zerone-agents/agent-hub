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

func (r *SceneRepository) ListByAgent(agentID uint64) ([]*scene.Scene, error) {
	var scenes []*scene.Scene
	err := r.db.Where("agent_id = ?", agentID).Order("id ASC").Find(&scenes).Error
	return scenes, err
}

func (r *SceneRepository) ListAll() ([]*scene.Scene, error) {
	var scenes []*scene.Scene
	err := r.db.Order("id ASC").Find(&scenes).Error
	return scenes, err
}

func (r *SceneRepository) GetByName(name string) (*scene.Scene, error) {
	var s scene.Scene
	err := r.db.Where("name = ?", name).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SceneRepository) Create(s *scene.Scene) error {
	return r.db.Create(s).Error
}

func (r *SceneRepository) Update(s *scene.Scene) error {
	return r.db.Save(s).Error
}

func (r *SceneRepository) Delete(id uint64) error {
	return r.db.Where("id = ?", id).Delete(&scene.Scene{}).Error
}

func (r *SceneRepository) ExistsByName(name string) (bool, error) {
	var count int64
	err := r.db.Model(&scene.Scene{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}
