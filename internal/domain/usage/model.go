// Package usage 定义 H7.5 用量与运维的领域模型。
//
// UsageRecord 是一条原子用量事件（模型调用 / 消息 / 工作流步骤 /
// 扩展调用 / 存储等），由调用路径上的埋点异步写入，绝不反向影响主
// 流程。Cost 以整数微分（cost_micros）记账，单价配置化（system_settings
// 的 model_pricing），未配置单价时 cost 记 NULL 而非 0——区分
// "免费"与"未知"。UsageBudget 是软预算：超限不阻断，只在记录上标记
// denied 并触发告警。UsageAlert 是告警规则，UsageAlertEvent 是触发历史。
package usage

import "time"

// Kind 取值：用量事件的种类。
const (
	KindToken         = "token"
	KindModelCall     = "model_call"
	KindMessage       = "message"
	KindWorkflowStep  = "workflow_step"
	KindExtensionCall = "extension_call"
	KindStorage       = "storage"
)

// BudgetPeriod 取值：预算周期。
const (
	PeriodDaily   = "daily"
	PeriodMonthly = "monthly"
)

// BudgetKind 取值：预算维度（模型调用 / 扩展调用 / 存储）。
const (
	BudgetKindModelCall     = "model_call"
	BudgetKindExtensionCall = "extension_call"
	BudgetKindStorage       = "storage"
)

// AlertRule 取值：内置告警规则。
const (
	RuleBudgetPct       = "budget_pct"        // 预算用量达到 threshold%
	RuleErrorRateSpike  = "error_rate_spike"  // 错误率达到 threshold%（0-100）
	RuleHealthRed       = "health_red"        // 系统健康状态转红
)

// UsageRecord 是一条用量事件。Tokens / Cost / Bytes / Latency 均为可空：
// 埋点处拿不到就记 NULL。Denied 标记软预算超限（记录仍保留，供审计）。
type UsageRecord struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string    `gorm:"type:varchar(64);not null;index:idx_usage_records_tenant_kind_created,priority:1;index:idx_usage_records_tenant_ext_created,priority:1" json:"-"`
	Kind          string    `gorm:"type:varchar(32);not null;index:idx_usage_records_tenant_kind_created,priority:2" json:"kind"`
	AgentID       *uint64   `json:"agentId,omitempty"`
	RunID         string    `gorm:"type:varchar(64);not null;default:'';index" json:"runId,omitempty"`
	ExtensionName string    `gorm:"type:varchar(253);not null;default:'';index:idx_usage_records_tenant_ext_created,priority:2" json:"extensionName,omitempty"`
	Model         string    `gorm:"type:varchar(128);not null;default:''" json:"model,omitempty"`
	TokensIn      *int64    `json:"tokensIn,omitempty"`
	TokensOut     *int64    `json:"tokensOut,omitempty"`
	CostMicros    *int64    `json:"costMicros,omitempty"`
	Bytes         *int64    `json:"bytes,omitempty"`
	LatencyMs     *int64    `json:"latencyMs,omitempty"`
	Error         string    `gorm:"type:text;not null" json:"error,omitempty"`
	Denied        bool      `gorm:"not null;default:false" json:"denied"`
	CreatedAt     time.Time `gorm:"index:idx_usage_records_tenant_kind_created,priority:3;index:idx_usage_records_tenant_ext_created,priority:3" json:"createdAt"`
}

func (UsageRecord) TableName() string { return "usage_records" }

// UsageBudget 是租户级软预算。LimitValue 的量纲随 Kind：model_call /
// extension_call 为调用次数，storage 为字节数。AlertThresholdPct 是
// 告警阈值百分比（如 80 = 用到 80% 时触发 budget_pct 告警）。
type UsageBudget struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID          string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_usage_budgets_tenant_kind_period,priority:1;index" json:"-"`
	Kind              string    `gorm:"type:varchar(32);not null;uniqueIndex:uk_usage_budgets_tenant_kind_period,priority:2" json:"kind"`
	LimitValue        int64     `gorm:"not null" json:"limitValue"`
	Period            string    `gorm:"type:varchar(16);not null;uniqueIndex:uk_usage_budgets_tenant_kind_period,priority:3" json:"period"`
	AlertThresholdPct int       `gorm:"not null;default:80" json:"alertThresholdPct"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (UsageBudget) TableName() string { return "usage_budgets" }

// UsageAlert 是租户级告警规则。Channel 当前支持 "log" 与 "webhook"
// （配 WebhookURL 时触发即 POST JSON，3s 超时，失败仅记日志）。
type UsageAlert struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    string     `gorm:"type:varchar(64);not null;index" json:"-"`
	Rule        string     `gorm:"type:varchar(32);not null" json:"rule"`
	Channel     string     `gorm:"type:varchar(16);not null;default:'log'" json:"channel"`
	WebhookURL  string     `gorm:"type:varchar(512);not null;default:''" json:"webhookUrl,omitempty"`
	Threshold   float64    `gorm:"not null" json:"threshold"` // budget_pct: 0-100；error_rate_spike: 0-100；health_red: 未用
	LastFiredAt *time.Time `json:"lastFiredAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (UsageAlert) TableName() string { return "usage_alerts" }

// UsageAlertEvent 是一次告警触发记录（fire-and-forget，绝不回写失败）。
type UsageAlertEvent struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID   string    `gorm:"type:varchar(64);not null;index" json:"-"`
	AlertID    uint64    `gorm:"not null;index" json:"alertId"`
	Rule       string    `gorm:"type:varchar(32);not null" json:"rule"`
	Message    string    `gorm:"type:text;not null" json:"message"`
	WebhookURL string    `gorm:"type:varchar(512);not null;default:''" json:"-"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

func (UsageAlertEvent) TableName() string { return "usage_alert_events" }
