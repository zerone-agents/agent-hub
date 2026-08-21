package auth

import "time"

// TenantOAuthClient 是组织 → Casdoor Application 凭证映射（多组织登录入口）。
// client_secret_enc / cert_enc 为 AES-GCM 密文（provider.Encrypt 产物）；
// cert_enc 为空 = 该组织用全局 CASDOOR_CERTIFICATE 验签。
// default_key 实现 default 租户标记：default 行存 org 名，其余行 NULL；
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
