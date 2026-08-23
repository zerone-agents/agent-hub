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

// mustOwnAgent 写路径统一入口校验：agent 不属于该租户则返回
// gorm.ErrRecordNotFound（不暴露存在性）。关联表操作前的归属校验也用它。
func (r *AgentRepository) mustOwnAgent(tx *gorm.DB, tenantID string, agentID uint64) error {
	var count int64
	err := TenantOwned(tx.Model(&agent.AgentConfig{}), tenantID).
		Where("id = ?", agentID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AgentRepository) ListAll(tenantID string) ([]*agent.AgentConfig, error) {
	var agents []*agent.AgentConfig
	err := TenantOwned(r.db, tenantID).Order("id ASC").Find(&agents).Error
	return agents, err
}

// ListAllUnscoped 跨租户全量列出 agents。仅限无租户上下文的系统链路使用：
// runtime token 鉴权（token 本身即凭证，命中后再回填租户）和启动时的
// 存量数据回填。业务请求必须走 ListAll(tenantID)。
func (r *AgentRepository) ListAllUnscoped() ([]*agent.AgentConfig, error) {
	var agents []*agent.AgentConfig
	err := r.db.Order("id ASC").Find(&agents).Error
	return agents, err
}

// ListForPlatform returns agents enabled for the given client platform
// ("desktop" or "mobile"). Unknown platforms yield an empty list.
func (r *AgentRepository) ListForPlatform(tenantID, platform string) ([]*agent.AgentConfig, error) {
	var agents []*agent.AgentConfig
	var err error
	scoped := TenantOwned(r.db, tenantID)
	switch platform {
	case agent.PlatformMobile:
		err = scoped.Where("mobile_enabled = ?", true).Order("id ASC").Find(&agents).Error
	default: // agent.PlatformDesktop
		err = scoped.Where("desktop_enabled = ?", true).Order("id ASC").Find(&agents).Error
	}
	return agents, err
}

func (r *AgentRepository) GetByID(tenantID string, id uint64) (*agent.AgentConfig, error) {
	var a agent.AgentConfig
	err := TenantOwned(r.db, tenantID).Where("id = ?", id).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AgentRepository) GetByName(tenantID, name string) (*agent.AgentConfig, error) {
	var a agent.AgentConfig
	err := TenantOwned(r.db, tenantID).Where("name = ?", name).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create 写入前强制盖章 TenantID——调用方传入的 TenantID 不可信。
func (r *AgentRepository) Create(tenantID string, a *agent.AgentConfig) error {
	a.TenantID = tenantID
	return r.db.Create(a).Error
}

// Update 先校验归属（跨租户返回 ErrRecordNotFound，不暴露存在性），
// 再盖章 TenantID 后保存——调用方传入的 TenantID 不可信。
func (r *AgentRepository) Update(tenantID string, a *agent.AgentConfig) error {
	if err := r.mustOwnAgent(r.db, tenantID, a.ID); err != nil {
		return err
	}
	a.TenantID = tenantID
	return r.db.Save(a).Error
}

func (r *AgentRepository) Delete(tenantID string, id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 入口校验：agent 不属于该租户则整体失败（不暴露存在性），
		// 关联表级联删除也不会发生。
		if err := r.mustOwnAgent(tx, tenantID, id); err != nil {
			return err
		}
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
		return tx.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&agent.AgentConfig{}).Error
	})
}

func (r *AgentRepository) Exists(tenantID string, id uint64) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agent.AgentConfig{}), tenantID).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *AgentRepository) ExistsByName(tenantID, name string) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&agent.AgentConfig{}), tenantID).Where("name = ?", name).Count(&count).Error
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

// GetAllSubagents 返回本租户内 agent name -> subagent names 的映射。
// 按 agent_id 过滤的关联表查询不改签名；这个跨 agent 聚合方法以 name 为 key，
// 必须带租户过滤，否则跨租户同名 agent 的绑定会被合并到同一 map 条目。
func (r *AgentRepository) GetAllSubagents(tenantID string) (map[string][]string, error) {
	type row struct {
		AgentName    string
		SubagentName string
	}
	var rows []row
	err := r.db.Table("agent_subagents").
		Select("main.name as agent_name, sub.name as subagent_name").
		Joins("JOIN agents AS main ON agent_subagents.agent_id = main.id").
		Joins("JOIN agents AS sub ON agent_subagents.subagent_id = sub.id").
		Where("main.tenant_id = ?", tenantID).
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

func (r *AgentRepository) ClearDefaultExcept(tenantID string, exceptID uint64) error {
	return TenantOwned(r.db.Model(&agent.AgentConfig{}), tenantID).
		Where("id != ? AND is_default = ?", exceptID, true).
		Update("is_default", false).Error
}

func (r *AgentRepository) ClearAllDefault(tenantID string) error {
	return TenantOwned(r.db.Model(&agent.AgentConfig{}), tenantID).
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

// GetAllAgentKnowledgeDatasetIDs 返回本租户内 agent name -> dataset IDs 的映射。
// 与 GetAllSubagents 同理：以 name 为 key 的跨 agent 聚合必须带租户过滤。
func (r *AgentRepository) GetAllAgentKnowledgeDatasetIDs(tenantID string) (map[string][]string, error) {
	type row struct {
		AgentName string
		DatasetID string
	}
	var rows []row
	err := r.db.Table("agent_knowledge_datasets").
		Select("agents.name as agent_name, agent_knowledge_datasets.dataset_id as dataset_id").
		Joins("JOIN agents ON agent_knowledge_datasets.agent_id = agents.id").
		Where("agents.tenant_id = ?", tenantID).
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

// ListAllForReconcile 列出所有 agent 的 tenant_id/name/状态/runtime_port，供对账使用。
// 后台对账任务无租户上下文，Kong 实体按 DeployKey(TenantID, Name) 全局唯一，显式全量。
func (r *AgentRepository) ListAllForReconcile() ([]agent.AgentConfig, error) {
	var items []agent.AgentConfig
	err := r.db.Select("id, tenant_id, name, deployment_status, runtime_port").Find(&items).Error
	return items, err
}
