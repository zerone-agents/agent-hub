package mcp

import (
	"time"

	"control-panel/internal/domain/agent"
)

// Transport 类型常量（仅开放 sse / http，stdio 不再支持）
const (
	TransportSSE  = "sse"
	TransportHTTP = "http"
)

// McpServer 对应 SDK 的 McpServerConfig（stdio 类型已移除）。
// headers 字段以密文形式存储（由 service 层用 provider.Encrypt/Decrypt 处理）。
type McpServer struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	Name          string `gorm:"type:varchar(64);uniqueIndex:uk_tenant_name,priority:2;not null"`
	TenantID      string `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_tenant_name,priority:1;index"`
	Title         string `gorm:"type:varchar(128);not null"`
	Description   string `gorm:"type:text"`
	TransportType string `gorm:"column:transport_type;type:varchar(16);not null"`

	// sse / http 专属
	URL     string `gorm:"column:url;type:varchar(512)"`
	Headers string `gorm:"type:text"` // 加密后的 JSON：{"Authorization":"Bearer ***"}

	// 重试策略（可空，空时由客户端全局默认填充）
	RetryMaxRetries *int `gorm:"column:retry_max_retries"`
	RetryTimeoutMs  *int `gorm:"column:retry_timeout_ms"`

	// 内置 MCP 标记，内置服务不可删除，name/transportType 不可修改
	IsBuiltin bool `gorm:"column:is_builtin;not null;default:false"`

	// MCP 探测结果与状态
	ToolsJSON    string     `gorm:"column:tools;type:text"`
	ProbeStatus  string     `gorm:"column:probe_status;type:varchar(16);not null;default:'pending'"`
	LastProbedAt *time.Time `gorm:"column:last_probed_at"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;index"`
}

func (McpServer) TableName() string {
	return "mcp_servers"
}

// AgentMcpServer 是 Agent 与 McpServer 的多对多绑定关系。
// 不含 enabled 字段——存在即代表启用，删除即代表禁用。
type AgentMcpServer struct {
	AgentID     uint64            `gorm:"primaryKey;index:idx_agent_id"`
	McpServerID uint64            `gorm:"primaryKey;index:idx_mcp_id"`
	Agent       agent.AgentConfig `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`
	McpServer   McpServer         `gorm:"foreignKey:McpServerID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt   time.Time         `gorm:"column:created_at"`
}

func (AgentMcpServer) TableName() string {
	return "agent_mcp_servers"
}
