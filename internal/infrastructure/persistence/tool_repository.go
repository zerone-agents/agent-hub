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

// mustOwnTool 写路径统一入口校验（同 mustOwnProvider 模式）：tool 不属于该
// 租户则返回 gorm.ErrRecordNotFound，不暴露存在性。共享内置行（tenant_id=”
// 的 Skill/Task/... 模板）只读——tenantID 为空串的系统路径（SeedBuiltins）
// 例外，允许刷新共享行。
func (r *ToolRepository) mustOwnTool(tx *gorm.DB, tenantID string, toolID uint64) error {
	if tenantID == "" {
		return nil // 系统路径（启动 seeding）
	}
	var count int64
	err := tx.Model(&agent.Tool{}).Where("tenant_id = ? AND tenant_id != ''", tenantID).
		Where("id = ?", toolID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListAll 返回本租户可见的 tools（本租户行 + 共享内置行）。
func (r *ToolRepository) ListAll(tenantID string) ([]*agent.Tool, error) {
	var tools []*agent.Tool
	err := TenantWithShared(r.db.Model(&agent.Tool{}), tenantID).
		Order("id ASC").Find(&tools).Error
	return tools, err
}

func (r *ToolRepository) GetByName(tenantID, name string) (*agent.Tool, error) {
	var t agent.Tool
	err := TenantWithShared(r.db.Model(&agent.Tool{}), tenantID).
		Where("name = ?", name).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create 写入前强制盖章 TenantID——调用方传入的 TenantID 不可信。
// tenantID 传空串即系统路径（SeedBuiltins/SeedIfEmpty 写共享行）。
func (r *ToolRepository) Create(tenantID string, t *agent.Tool) error {
	t.TenantID = tenantID
	return r.db.Create(t).Error
}

// Update 先校验归属（跨租户/共享内置行返回 ErrRecordNotFound），再盖章保存。
func (r *ToolRepository) Update(tenantID string, t *agent.Tool) error {
	if err := r.mustOwnTool(r.db, tenantID, t.ID); err != nil {
		return err
	}
	t.TenantID = tenantID
	return r.db.Save(t).Error
}

func (r *ToolRepository) Delete(tenantID string, id uint64) error {
	if err := r.mustOwnTool(r.db, tenantID, id); err != nil {
		return err
	}
	return r.db.Where("id = ?", id).Delete(&agent.Tool{}).Error
}

func (r *ToolRepository) ExistsByName(tenantID, name string) (bool, error) {
	var count int64
	err := TenantWithShared(r.db.Model(&agent.Tool{}), tenantID).
		Where("name = ?", name).Count(&count).Error
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

// AddToolToAllAgents 把工具绑定到租户内全部 agent。agents 无共享行，
// Pluck 用 TenantOwned（仅本租户），确保绑定目标 agent 与工具同租户。
func (r *ToolRepository) AddToolToAllAgents(tenantID string, toolID uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var agentIDs []uint64
		if err := TenantOwned(tx.Model(&agent.AgentConfig{}), tenantID).
			Pluck("id", &agentIDs).Error; err != nil {
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

// GetDefaultToolIDs 返回该租户可见的默认工具 ID（本租户 is_default 行 +
// 共享 is_default 行，TenantWithShared）。
func (r *ToolRepository) GetDefaultToolIDs(tenantID string) ([]uint64, error) {
	var ids []uint64
	err := TenantWithShared(r.db.Model(&agent.Tool{}), tenantID).
		Where("is_default = ?", true).Pluck("id", &ids).Error
	return ids, err
}

// GetDefaultToolNames 返回该租户可见的默认工具名（本租户 + 共享行）。
func (r *ToolRepository) GetDefaultToolNames(tenantID string) ([]string, error) {
	var names []string
	err := TenantWithShared(r.db.Model(&agent.Tool{}), tenantID).
		Where("is_default = ?", true).Pluck("name", &names).Error
	return names, err
}

// BindDefaultToolsToAgent 给 agent 绑定该租户可见的默认工具（本租户 + 共享行）。
func (r *ToolRepository) BindDefaultToolsToAgent(tenantID string, agentID uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var toolIDs []uint64
		if err := TenantWithShared(tx.Model(&agent.Tool{}), tenantID).
			Where("is_default = ?", true).Pluck("id", &toolIDs).Error; err != nil {
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
