package repository

import (
	"control-panel/internal/domain/mcp"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

type McpRepository struct {
	db *gorm.DB
}

func NewMcpRepository() *McpRepository {
	return &McpRepository{db: database.GetDB()}
}

// mustOwnMcp 写路径统一入口校验（同 mustOwnProvider 模式）：mcp 不属于该
// 租户则返回 gorm.ErrRecordNotFound，不暴露存在性。共享内置行（tenant_id=”
// 的 knowledge 模板）只读——tenantID 为空串的系统路径（SeedBuiltins 刷新
// 内置元数据）例外，允许写共享行。
func (r *McpRepository) mustOwnMcp(tx *gorm.DB, tenantID string, mcpID uint64) error {
	if tenantID == "" {
		return nil // 系统路径（启动 seeding）
	}
	var count int64
	err := tx.Model(&mcp.McpServer{}).Where("tenant_id = ? AND tenant_id != ''", tenantID).
		Where("id = ?", mcpID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListAll 返回本租户可见的 mcp_servers（本租户行 + 共享内置行）。
func (r *McpRepository) ListAll(tenantID string) ([]*mcp.McpServer, error) {
	var items []*mcp.McpServer
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Order("id ASC").Find(&items).Error
	return items, err
}

func (r *McpRepository) GetByName(tenantID, name string) (*mcp.McpServer, error) {
	var m mcp.McpServer
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Where("name = ?", name).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *McpRepository) GetByID(tenantID string, id uint64) (*mcp.McpServer, error) {
	var m mcp.McpServer
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Create 写入前强制盖章 TenantID——调用方传入的 TenantID 不可信。
// tenantID 传空串即系统路径（SeedBuiltins 写共享内置行）。
func (r *McpRepository) Create(tenantID string, m *mcp.McpServer) error {
	m.TenantID = tenantID
	return r.db.Create(m).Error
}

// Update 先校验归属（跨租户/共享内置行返回 ErrRecordNotFound；系统路径
// tenantID=” 例外），再盖章保存。
func (r *McpRepository) Update(tenantID string, m *mcp.McpServer) error {
	if err := r.mustOwnMcp(r.db, tenantID, m.ID); err != nil {
		return err
	}
	m.TenantID = tenantID
	return r.db.Save(m).Error
}

func (r *McpRepository) Delete(tenantID string, id uint64) error {
	if err := r.mustOwnMcp(r.db, tenantID, id); err != nil {
		return err
	}
	return r.db.Where("id = ?", id).Delete(&mcp.McpServer{}).Error
}

func (r *McpRepository) ExistsByName(tenantID, name string) (bool, error) {
	var count int64
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

// GetMcpServersByAgent 返回某 Agent 绑定的所有 McpServer 完整记录（含加密
// 字段）。绑定关系经 agent 主表租户校验，这里再按 TenantWithShared 过滤
// 主表行，防跨租户绑定。
func (r *McpRepository) GetMcpServersByAgent(tenantID string, agentID uint64) ([]*mcp.McpServer, error) {
	var items []*mcp.McpServer
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Joins("JOIN agent_mcp_servers ON agent_mcp_servers.mcp_server_id = mcp_servers.id").
		Where("agent_mcp_servers.agent_id = ?", agentID).
		Order("mcp_servers.id ASC").
		Find(&items).Error
	return items, err
}

// GetMcpNamesByAgent 返回某 Agent 绑定的 McpServer name 列表（轻量查询）。
func (r *McpRepository) GetMcpNamesByAgent(agentID uint64) ([]string, error) {
	var names []string
	err := r.db.Table("agent_mcp_servers").
		Select("mcp_servers.name").
		Joins("JOIN mcp_servers ON agent_mcp_servers.mcp_server_id = mcp_servers.id").
		Where("agent_mcp_servers.agent_id = ?", agentID).
		Pluck("mcp_servers.name", &names).Error
	return names, err
}

// ReplaceAgentMcps 用事务替换 Agent 与 McpServer 的绑定关系。
func (r *McpRepository) ReplaceAgentMcps(agentID uint64, mcpIDs []uint64) error {
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if err := tx.Where("agent_id = ?", agentID).Delete(&mcp.AgentMcpServer{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, mcpID := range mcpIDs {
		if err := tx.Create(&mcp.AgentMcpServer{
			AgentID:     agentID,
			McpServerID: mcpID,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// GetAllAgentMcpNames 返回本租户内 Agent -> McpServerNames 映射（供 manifest 聚合用）。
// 与 GetAllSubagents 同理：以 name 为 key 的跨 agent 聚合必须带租户过滤，
// 否则跨租户同名 agent 的绑定会被合并到同一 map 条目。
func (r *McpRepository) GetAllAgentMcpNames(tenantID string) (map[string][]string, error) {
	type row struct {
		AgentName string
		McpName   string
	}
	var rows []row
	err := r.db.Table("agent_mcp_servers").
		Select("agent.name as agent_name, mcp_servers.name as mcp_name").
		Joins("JOIN agents AS agent ON agent_mcp_servers.agent_id = agent.id").
		Joins("JOIN mcp_servers ON agent_mcp_servers.mcp_server_id = mcp_servers.id").
		Where("agent.tenant_id = ?", tenantID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, r := range rows {
		result[r.AgentName] = append(result[r.AgentName], r.McpName)
	}
	return result, nil
}

// CountByIDs 返回给定 ID 列表中本租户（含共享内置行）实际存在的 McpServer
// 数量（用于绑定前校验，防跨租户绑定）。
func (r *McpRepository) CountByIDs(tenantID string, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var count int64
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Where("id IN ?", ids).Count(&count).Error
	return count, err
}

// GetMcpsByIDs 按 ID 列表批量查询 McpServer（本租户 + 共享内置行）。
func (r *McpRepository) GetMcpsByIDs(tenantID string, ids []uint64) ([]*mcp.McpServer, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var items []*mcp.McpServer
	err := TenantWithShared(r.db.Model(&mcp.McpServer{}), tenantID).
		Where("id IN ?", ids).Find(&items).Error
	return items, err
}
