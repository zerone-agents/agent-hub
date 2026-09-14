// H7.1 扩展生命周期：extension_installs 模型。
//
// Install 表示某租户内一次扩展安装记录（每租户每扩展至多一行，
// 靠唯一索引 uk_extension_installs 保证；并发安装只有一个写入成功，
// 另一个命中唯一索引冲突后转为幂等返回）。升级/回滚只改本行的
// version 指向；卸载删除本行但保留 extension_versions 与 run_states
// 历史数据（旧状态依旧可读）。
package extension

import "time"

// 安装状态取值。
const (
	InstallStatusEnabled  = "enabled"
	InstallStatusDisabled = "disabled"
)

// Install 是租户级的扩展安装记录。
type Install struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    string `gorm:"type:varchar(64);not null;uniqueIndex:uk_extension_installs,priority:1;index" json:"-"`
	ExtensionID uint64 `gorm:"not null;uniqueIndex:uk_extension_installs,priority:2;index" json:"extensionId"`
	Version     string `gorm:"type:varchar(64);not null" json:"version"`
	Status      string `gorm:"type:varchar(24);not null;default:'enabled';index" json:"status"`
	InstalledBy string `gorm:"type:varchar(160);not null;default:''" json:"installedBy"`
	// MigrationLog 记录历次迁移（JSON 数组，含方向、版本区间与
	// 每个 op 的新旧值），供回滚推导 inverse 使用。
	MigrationLog string    `gorm:"type:longtext;not null" json:"migrationLog"`
	InstalledAt  time.Time `json:"installedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (Install) TableName() string { return "extension_installs" }
