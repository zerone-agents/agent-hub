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

// mustOwnSkill 写路径统一入口校验（同 mustOwnProvider 模式）：skill 不属于
// 该租户则返回 gorm.ErrRecordNotFound，不暴露存在性。
func (r *SkillRepository) mustOwnSkill(tx *gorm.DB, tenantID string, skillID uint64) error {
	if tenantID == "" {
		return nil // 系统路径
	}
	var count int64
	err := TenantOwned(tx.Model(&skill.Skill{}), tenantID).
		Where("id = ?", skillID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListAll 返回本租户可见的 skills（本租户行 + 共享行）。
func (r *SkillRepository) ListAll(tenantID string) ([]*skill.Skill, error) {
	var skills []*skill.Skill
	err := TenantWithShared(r.db.Model(&skill.Skill{}), tenantID).
		Order("id ASC").Find(&skills).Error
	return skills, err
}

func (r *SkillRepository) ListByType(tenantID, skillType string) ([]*skill.Skill, error) {
	var skills []*skill.Skill
	err := TenantWithShared(r.db.Model(&skill.Skill{}), tenantID).
		Where("type = ?", skillType).Order("id ASC").Find(&skills).Error
	return skills, err
}

func (r *SkillRepository) GetByName(tenantID, name string) (*skill.Skill, error) {
	var s skill.Skill
	err := TenantWithShared(r.db.Model(&skill.Skill{}), tenantID).
		Where("name = ?", name).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create 写入前强制盖章 TenantID——调用方传入的 TenantID 不可信。
// skills 没有系统写通道：空 tenantID 直接拒绝（ErrTenantIDRequired），
// 防止把私有行静默提升为共享。
func (r *SkillRepository) Create(tenantID string, s *skill.Skill) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	s.TenantID = tenantID
	return r.db.Create(s).Error
}

// Update 先校验归属（跨租户返回 ErrRecordNotFound），再盖章保存。
func (r *SkillRepository) Update(tenantID string, s *skill.Skill) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	if err := r.mustOwnSkill(r.db, tenantID, s.ID); err != nil {
		return err
	}
	s.TenantID = tenantID
	return r.db.Save(s).Error
}

func (r *SkillRepository) Delete(tenantID string, id uint64) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}
	if err := r.mustOwnSkill(r.db, tenantID, id); err != nil {
		return err
	}
	return r.db.Where("id = ?", id).Delete(&skill.Skill{}).Error
}

func (r *SkillRepository) ExistsByName(tenantID, name string) (bool, error) {
	var count int64
	err := TenantWithShared(r.db.Model(&skill.Skill{}), tenantID).
		Where("name = ?", name).Count(&count).Error
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

// GetAllAgentSkills 返回本租户内 agent name -> skill names 的映射。
// 与 GetAllSubagents 同理：以 name 为 key 的跨 agent 聚合必须带租户过滤，
// 否则跨租户同名 agent 的绑定会被合并到同一 map 条目。
func (r *SkillRepository) GetAllAgentSkills(tenantID string) (map[string][]string, error) {
	type row struct {
		AgentName string
		SkillName string
	}
	var rows []row
	err := r.db.Table("agent_skills").
		Select("agent.name as agent_name, skills.name as skill_name").
		Joins("JOIN agents AS agent ON agent_skills.agent_id = agent.id").
		Joins("JOIN skills ON agent_skills.skill_id = skills.id").
		Where("agent.tenant_id = ?", tenantID).
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

// GetSkillBindingsScoped 返回仍绑定该技能的 Agent 视图（删除保护 #123）：
// own = 请求租户的 Agent 名单（409 载荷）；foreign = 他租户仍绑定（仅中性
// 事实，不携带任何他租户身份）。own ∪ foreign 任一命中即阻断删除。
func (r *SkillRepository) GetSkillBindingsScoped(tenantID string, skillID uint64) ([]string, bool, error) {
	type row struct {
		AgentName string
		TenantID  string
	}
	var rows []row
	err := r.db.Table("agent_skills").
		Select("agents.name as agent_name, agents.tenant_id as tenant_id").
		Joins("JOIN agents ON agent_skills.agent_id = agents.id").
		Where("agent_skills.skill_id = ?", skillID).
		Order("agents.name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	var own []string
	foreign := false
	for _, r := range rows {
		if r.TenantID == tenantID {
			own = append(own, r.AgentName)
		} else {
			foreign = true
		}
	}
	return own, foreign, nil
}
