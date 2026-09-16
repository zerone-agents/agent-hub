// H7.1 扩展生命周期：extension_installs 模型。
//
// Install 表示某租户内一次扩展安装记录（每租户每扩展至多一行，
// 靠唯一索引 uk_extension_installs 保证；并发安装只有一个写入成功，
// 另一个命中唯一索引冲突后转为幂等返回）。升级/回滚只改本行的
// version 指向；卸载删除本行但保留 extension_versions 与 run_states
// 历史数据（旧状态依旧可读）。
//
// H7.4 扩展身份凭据（P0）：同一行还承载 Hub 签发的扩展身份凭据。
// 扩展调用 Hub API 时必须同时给出 X-Extension-Name 与 X-Extension-Token，
// Hub 用 (tenant, name) 定位本行后校验凭据。只存哈希，明文仅在签发
// 响应里返回一次。凭据随安装行同生共死：卸载（非 purge）后该行被删，
// 凭据立即失效；停用（status=disabled）时校验直接拒绝。
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

	// --- H7.4 扩展身份凭据 ---
	// AuthTokenID 是凭据的公开标识（明文 token 的前半段），用于定位行；
	// 不敏感，可展示。空表示尚未签发凭据。
	AuthTokenID string `gorm:"type:varchar(64);not null;default:'';index" json:"authTokenId,omitempty"`
	// AuthTokenHash 是凭据密钥段的 SHA-256 十六进制摘要。绝不外发。
	AuthTokenHash string `gorm:"type:varchar(64);not null;default:''" json:"-"`
	// AuthTokenIssuedAt 记录最近一次签发/轮换时间，便于运维排查。
	AuthTokenIssuedAt *time.Time `json:"authTokenIssuedAt,omitempty"`
}

func (Install) TableName() string { return "extension_installs" }

// HasCredential 报告该安装行是否已签发扩展身份凭据。
func (i Install) HasCredential() bool {
	return i.AuthTokenID != "" && i.AuthTokenHash != ""
}
