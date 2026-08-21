package repository

import (
	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type ToolRepository struct {
	db *gorm.DB
}

func NewToolRepository() *ToolRepository {
	return &ToolRepository{db: database.GetDB()}
}

func (r *ToolRepository) ListAll() ([]*agent.Tool, error) {
	var tools []*agent.Tool
	err := r.db.Order("id ASC").Find(&tools).Error
	return tools, err
}

func (r *ToolRepository) GetByName(name string) (*agent.Tool, error) {
	var t agent.Tool
	err := r.db.Where("name = ?", name).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *ToolRepository) Create(t *agent.Tool) error {
	return r.db.Create(t).Error
}

func (r *ToolRepository) Update(t *agent.Tool) error {
	return r.db.Save(t).Error
}

func (r *ToolRepository) Delete(id uint64) error {
	return r.db.Where("id = ?", id).Delete(&agent.Tool{}).Error
}

func (r *ToolRepository) ExistsByName(name string) (bool, error) {
	var count int64
	err := r.db.Model(&agent.Tool{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *ToolRepository) GetToolsByAgent(agentID uint64) ([]string, error) {
	var names []string
	err := r.db.Table("agent_tools").
		Select("tools.name").
		Joins("JOIN tools ON agent_tools.tool_id = tools.id").
		Where("agent_tools.agent_id = ?", agentID).
		Pluck("tools.name", &names).Error
	return names, err
}

// GetAllAgentTools 返回本租户内 agent name -> tool names 的映射。
// 与 GetAllSubagents 同理：以 name 为 key 的跨 agent 聚合必须带租户过滤，
// 否则跨租户同名 agent 的绑定会被合并到同一 map 条目。
func (r *ToolRepository) GetAllAgentTools(tenantID string) (map[string][]string, error) {
	type row struct {
		AgentName string
		ToolName  string
	}
	var rows []row
	err := r.db.Table("agent_tools").
		Select("agent.name as agent_name, tools.name as tool_name").
		Joins("JOIN agents AS agent ON agent_tools.agent_id = agent.id").
		Joins("JOIN tools ON agent_tools.tool_id = tools.id").
		Where("agent.tenant_id = ?", tenantID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, r := range rows {
		result[r.AgentName] = append(result[r.AgentName], r.ToolName)
	}
	return result, nil
}

func (r *ToolRepository) ReplaceAgentTools(agentID uint64, toolIDs []uint64) error {
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if err := tx.Where("agent_id = ?", agentID).Delete(&agent.AgentTool{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, toolID := range toolIDs {
		if err := tx.Create(&agent.AgentTool{
			AgentID: agentID,
			ToolID:  toolID,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func (r *ToolRepository) AddToolToAllAgents(toolID uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var agentIDs []uint64
		if err := tx.Model(&agent.AgentConfig{}).Pluck("id", &agentIDs).Error; err != nil {
			return err
		}
		for _, agentID := range agentIDs {
			at := agent.AgentTool{AgentID: agentID, ToolID: toolID}
			if err := tx.FirstOrCreate(&at, at).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ToolRepository) GetDefaultToolIDs() ([]uint64, error) {
	var ids []uint64
	err := r.db.Model(&agent.Tool{}).Where("is_default = ?", true).Pluck("id", &ids).Error
	return ids, err
}

func (r *ToolRepository) GetDefaultToolNames() ([]string, error) {
	var names []string
	err := r.db.Model(&agent.Tool{}).Where("is_default = ?", true).Pluck("name", &names).Error
	return names, err
}

func (r *ToolRepository) BindDefaultToolsToAgent(agentID uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var toolIDs []uint64
		if err := tx.Model(&agent.Tool{}).Where("is_default = ?", true).Pluck("id", &toolIDs).Error; err != nil {
			return err
		}
		for _, toolID := range toolIDs {
			at := agent.AgentTool{AgentID: agentID, ToolID: toolID}
			if err := tx.FirstOrCreate(&at, at).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
