package repository

import (
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/skill"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type SkillRepository struct {
	db *gorm.DB
}

func NewSkillRepository() *SkillRepository {
	return &SkillRepository{db: database.GetDB()}
}

// NewSkillRepositoryWithDB builds a SkillRepository backed by the given *gorm.DB.
// Used by tests that need an isolated DB instance.
func NewSkillRepositoryWithDB(db *gorm.DB) *SkillRepository {
	return &SkillRepository{db: db}
}

func (r *SkillRepository) ListAll() ([]*skill.Skill, error) {
	var skills []*skill.Skill
	err := r.db.Order("id ASC").Find(&skills).Error
	return skills, err
}

func (r *SkillRepository) ListByType(skillType string) ([]*skill.Skill, error) {
	var skills []*skill.Skill
	err := r.db.Where("type = ?", skillType).Order("id ASC").Find(&skills).Error
	return skills, err
}

func (r *SkillRepository) GetByName(name string) (*skill.Skill, error) {
	var s skill.Skill
	err := r.db.Where("name = ?", name).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SkillRepository) Create(s *skill.Skill) error {
	return r.db.Create(s).Error
}

func (r *SkillRepository) Update(s *skill.Skill) error {
	return r.db.Save(s).Error
}

func (r *SkillRepository) Delete(id uint64) error {
	return r.db.Where("id = ?", id).Delete(&skill.Skill{}).Error
}

func (r *SkillRepository) ExistsByName(name string) (bool, error) {
	var count int64
	err := r.db.Model(&skill.Skill{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *SkillRepository) GetAgentSkills(agentID uint64) ([]string, error) {
	var names []string
	err := r.db.Table("skills").
		Select("skills.name").
		Joins("JOIN agent_skills ON agent_skills.skill_id = skills.id").
		Where("agent_skills.agent_id = ?", agentID).
		Pluck("skills.name", &names).Error
	return names, err
}

// GetAgentSkillsFull returns the full skill records associated with an agent,
// including url and file_hash needed by the deployer.
func (r *SkillRepository) GetAgentSkillsFull(agentID uint64) ([]*skill.Skill, error) {
	var skills []*skill.Skill
	err := r.db.Table("skills").
		Joins("JOIN agent_skills ON agent_skills.skill_id = skills.id").
		Where("agent_skills.agent_id = ?", agentID).
		Find(&skills).Error
	return skills, err
}

func (r *SkillRepository) GetAllAgentSkills() (map[string][]string, error) {
	type row struct {
		AgentName string
		SkillName string
	}
	var rows []row
	err := r.db.Table("agent_skills").
		Select("agent.name as agent_name, skills.name as skill_name").
		Joins("JOIN agents AS agent ON agent_skills.agent_id = agent.id").
		Joins("JOIN skills ON agent_skills.skill_id = skills.id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, r := range rows {
		result[r.AgentName] = append(result[r.AgentName], r.SkillName)
	}
	return result, nil
}

func (r *SkillRepository) ReplaceAgentSkills(agentID uint64, skillIDs []uint64) error {
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if err := tx.Where("agent_id = ?", agentID).Delete(&agent.AgentSkill{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, skillID := range skillIDs {
		if err := tx.Create(&agent.AgentSkill{
			AgentID: agentID,
			SkillID: skillID,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}
