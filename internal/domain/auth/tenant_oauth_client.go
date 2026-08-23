package auth

import "time"

// TenantOAuthClient 是组织 → Casdoor Application 凭证映射（多组织登录入口）。
// client_secret_enc / cert_enc 为 AES-GCM 密文（provider.Encrypt 产物）；
// cert_enc 为空 = 该组织用全局 CASDOOR_CERTIFICATE 验签。
// default_key 实现 default 租户标记：default 行存哨兵常量 DefaultKeySentinel
// （不存 org 名，理由见 DefaultKeySentinel），其余行 NULL；
// uniqueIndex 保证最多一行非 NULL，即 default 有且唯一（表非空 ⇒ 恰好一个，
// 不变式由 ops API 事务维护，见 Task 2）。
type TenantOAuthClient struct {
	Org             string    `gorm:"primaryKey;type:varchar(64)"                 json:"org"`
	ClientID        string    `gorm:"type:varchar(128);not null"                  json:"clientId"`
	ClientSecretEnc string    `gorm:"column:client_secret_enc;type:text;not null" json:"-"`
	CertEnc         string    `gorm:"column:cert_enc;type:text"                    json:"-"`
	DefaultKey      *string   `gorm:"column:default_key;type:varchar(64);uniqueIndex:uk_default_key" json:"-"`
	CreatedAt       time.Time `gorm:"column:created_at"                           json:"createdAt"`
	UpdatedAt       time.Time `gorm:"column:updated_at"                           json:"updatedAt"`
}

func (TenantOAuthClient) TableName() string { return "tenant_oauth_clients" }

// DefaultKeySentinel 是 default 行 default_key 列的哨兵常量。default_key 只有
// 「非 NULL 即 default」语义（FindDefault/List 均只判 NULL），值从不被解释为
// org。写入常量而非 org 名，使 uk_default_key 唯一索引真正约束「最多一行
// 非 NULL」——存 org 名时不同 org 互不碰撞，索引不起兜底作用（issue #54）。
// 跨事务的并发 default 写入撞索引 → repository.ErrDefaultConflict → 409。
const DefaultKeySentinel = "default"
