package repository

import (
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type AgentRepository struct {
	db *gorm.DB
}

func NewAgentRepository() *AgentRepository {
	return &AgentRepository{db: database.GetDB()}
}

func (r *AgentRepository) ListAll() ([]*agent.AgentConfig, error) {
	var agents []*agent.AgentConfig
	err := r.db.Order("id ASC").Find(&agents).Error
	return agents, err
}

// ListForPlatform returns agents enabled for the given client platform
// ("desktop" or "mobile"). Unknown platforms yield an empty list.
func (r *AgentRepository) ListForPlatform(platform string) ([]*agent.AgentConfig, error) {
	var agents []*agent.AgentConfig
	var err error
	switch platform {
	case agent.PlatformMobile:
		err = r.db.Where("mobile_enabled = ?", true).Order("id ASC").Find(&agents).Error
	default: // agent.PlatformDesktop
		err = r.db.Where("desktop_enabled = ?", true).Order("id ASC").Find(&agents).Error
	}
	return agents, err
}

func (r *AgentRepository) GetByID(id uint64) (*agent.AgentConfig, error) {
	var a agent.AgentConfig
	err := r.db.Where("id = ?", id).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AgentRepository) GetByName(name string) (*agent.AgentConfig, error) {
	var a agent.AgentConfig
	err := r.db.Where("name = ?", name).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AgentRepository) Create(a *agent.AgentConfig) error {
	return r.db.Create(a).Error
}

func (r *AgentRepository) Update(a *agent.AgentConfig) error {
	return r.db.Save(a).Error
}

func (r *AgentRepository) Delete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Cascade-delete all association rows that reference the agent.
		// AgentSubagent can reference the agent either as the parent
		// (agent_id) or as the subagent (subagent_id), so clear both.
		if err := tx.Where("agent_id = ? OR subagent_id = ?", id, id).Delete(&agent.AgentSubagent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ?", id).Delete(&agent.AgentTool{}).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ?", id).Delete(&agent.AgentSkill{}).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ?", id).Delete(&mcp.AgentMcpServer{}).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ?", id).Delete(&agent.AgentKnowledgeDataset{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&agent.AgentConfig{}).Error
	})
}

func (r *AgentRepository) Exists(id uint64) (bool, error) {
	var count int64
	err := r.db.Model(&agent.AgentConfig{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *AgentRepository) ExistsByName(name string) (bool, error) {
	var count int64
	err := r.db.Model(&agent.AgentConfig{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *AgentRepository) GetSubagents(agentID uint64) ([]string, error) {
	var names []string
	err := r.db.Table("agent_subagents").
		Select("sub.name").
		Joins("JOIN agents AS sub ON agent_subagents.subagent_id = sub.id").
		Where("agent_subagents.agent_id = ?", agentID).
		Pluck("sub.name", &names).Error
	return names, err
}

func (r *AgentRepository) GetAllSubagents() (map[string][]string, error) {
	type row struct {
		AgentName    string
		SubagentName string
	}
	var rows []row
	err := r.db.Table("agent_subagents").
		Select("main.name as agent_name, sub.name as subagent_name").
		Joins("JOIN agents AS main ON agent_subagents.agent_id = main.id").
		Joins("JOIN agents AS sub ON agent_subagents.subagent_id = sub.id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, r := range rows {
		result[r.AgentName] = append(result[r.AgentName], r.SubagentName)
	}
	return result, nil
}

func (r *AgentRepository) ReplaceSubagents(agentID uint64, subagentIDs []uint64) error {
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if err := tx.Where("agent_id = ?", agentID).Delete(&agent.AgentSubagent{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, subID := range subagentIDs {
		if err := tx.Create(&agent.AgentSubagent{
			AgentID:    agentID,
			SubagentID: subID,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func (r *AgentRepository) ClearDefaultExcept(exceptID uint64) error {
	return r.db.Model(&agent.AgentConfig{}).
		Where("id != ? AND is_default = ?", exceptID, true).
		Update("is_default", false).Error
}

func (r *AgentRepository) ClearAllDefault() error {
	return r.db.Model(&agent.AgentConfig{}).
		Where("is_default = ?", true).
		Update("is_default", false).Error
}

// ReplaceAgentKnowledgeDatasets replaces all dataset bindings for an agent.
func (r *AgentRepository) ReplaceAgentKnowledgeDatasets(agentID uint64, datasetIDs []string) error {
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if err := tx.Where("agent_id = ?", agentID).Delete(&agent.AgentKnowledgeDataset{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, datasetID := range datasetIDs {
		if datasetID == "" {
			continue
		}
		if err := tx.Create(&agent.AgentKnowledgeDataset{
			AgentID:   agentID,
			DatasetID: datasetID,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// GetKnowledgeDatasetIDsByAgent returns the dataset IDs bound to an agent.
func (r *AgentRepository) GetKnowledgeDatasetIDsByAgent(agentID uint64) ([]string, error) {
	var ids []string
	err := r.db.Model(&agent.AgentKnowledgeDataset{}).
		Where("agent_id = ?", agentID).
		Pluck("dataset_id", &ids).Error
	return ids, err
}

// GetAllAgentKnowledgeDatasetIDs returns all Agent name -> dataset IDs bindings.
func (r *AgentRepository) GetAllAgentKnowledgeDatasetIDs() (map[string][]string, error) {
	type row struct {
		AgentName string
		DatasetID string
	}
	var rows []row
	err := r.db.Table("agent_knowledge_datasets").
		Select("agents.name as agent_name, agent_knowledge_datasets.dataset_id as dataset_id").
		Joins("JOIN agents ON agent_knowledge_datasets.agent_id = agents.id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, r := range rows {
		result[r.AgentName] = append(result[r.AgentName], r.DatasetID)
	}
	return result, nil
}

// EnsureAgentToolBinding makes sure the agent is bound to the given tool.
func (r *AgentRepository) EnsureAgentToolBinding(agentID, toolID uint64) error {
	var count int64
	err := r.db.Model(&agent.AgentTool{}).
		Where("agent_id = ? AND tool_id = ?", agentID, toolID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.db.Create(&agent.AgentTool{
		AgentID: agentID,
		ToolID:  toolID,
	}).Error
}

// RemoveAgentToolBinding removes the binding between an agent and a tool.
func (r *AgentRepository) RemoveAgentToolBinding(agentID, toolID uint64) error {
	return r.db.Where("agent_id = ? AND tool_id = ?", agentID, toolID).
		Delete(&agent.AgentTool{}).Error
}

// EnsureAgentMcpBinding makes sure the agent is bound to the given MCP server.
func (r *AgentRepository) EnsureAgentMcpBinding(agentID, mcpServerID uint64) error {
	var count int64
	err := r.db.Model(&mcp.AgentMcpServer{}).
		Where("agent_id = ? AND mcp_server_id = ?", agentID, mcpServerID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.db.Create(&mcp.AgentMcpServer{
		AgentID:     agentID,
		McpServerID: mcpServerID,
	}).Error
}

// RemoveAgentMcpBinding removes the binding between an agent and an MCP server.
func (r *AgentRepository) RemoveAgentMcpBinding(agentID, mcpServerID uint64) error {
	return r.db.Where("agent_id = ? AND mcp_server_id = ?", agentID, mcpServerID).
		Delete(&mcp.AgentMcpServer{}).Error
}

// ListAllForReconcile 列出所有 agent 的 name/状态/runtime_port，供对账使用。
func (r *AgentRepository) ListAllForReconcile() ([]agent.AgentConfig, error) {
	var items []agent.AgentConfig
	err := r.db.Select("id, name, deployment_status, runtime_port").Find(&items).Error
	return items, err
}
