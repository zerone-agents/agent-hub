package repository

import (
	"errors"
	"strings"

	authdom "control-panel/internal/domain/auth"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

// ErrDefaultRequired 表示操作会破坏「表非空 ⇒ 恰好一个 default」的不变式：
// 把 default 行降级、或删除仍有其他行的 default 行。handler 映射 409。
var ErrDefaultRequired = errors.New("default 租户必须存在且唯一：请先转移 default 再操作")

// ErrDefaultConflict 表示并发写入撞上 default_key 唯一约束（哨兵值兜底生效），
// 映射 409，客户端重试即可。
var ErrDefaultConflict = errors.New("并发 default 冲突，请重试")

// isDefaultKeyConflict 判定错误链是否为 default_key 唯一索引冲突。
// sqlite: "UNIQUE constraint failed: tenant_oauth_clients.default_key"；
// MySQL: "Duplicate entry 'default' for key '...uk_default_key'"。
// 两种驱动消息都含 "default_key"；主键（org）冲突消息不含该串，可精确区分。
func isDefaultKeyConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "default_key")
}

// TenantOAuthClientRepository 存取 tenant_oauth_clients（组织 → Casdoor
// Application 凭证映射）。加解密在 handler/service 层做，这里只存取密文。
type TenantOAuthClientRepository struct {
	db *gorm.DB
}

func NewTenantOAuthClientRepository() *TenantOAuthClientRepository {
	return &TenantOAuthClientRepository{db: database.GetDB()}
}

// Upsert 插入或更新（主键 org）。事务性 default 语义：
//   - 空表 + isDefault=false → 事务内自动提升为 default（首行守卫，原 handler
//     层事务外判定移入，收窄并发窗口）；
//   - isDefault=true：先清旧 default 的 default_key，再设本行为 default；
//   - isDefault=false 且目标是当前 default 且表内还有其他行 → ErrDefaultRequired
//     （唯一行保持 default 不变，允许更新非 default 字段）。
//     跨事务的残余竞态由 default_key 哨兵唯一索引兜底，撞索引 → ErrDefaultConflict。
func (r *TenantOAuthClientRepository) Upsert(org, clientID, secretEnc, certEnc string, isDefault bool) error {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 首行自动 default：Count 在事务内做（原 handler 层事务外判定移入，
		// 收窄并发窗口；跨事务残余竞态由 default_key 哨兵唯一索引兜底）。
		if !isDefault {
			var n int64
			if err := tx.Model(&authdom.TenantOAuthClient{}).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				isDefault = true
			}
		}

		var existing authdom.TenantOAuthClient
		err := tx.Where("org = ?", org).First(&existing).Error
		isCurrentDefault := false
		if err == nil {
			isCurrentDefault = existing.DefaultKey != nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if isDefault {
			// 清旧 default（不含本行；本行随后 Save 覆盖）。
			if err := tx.Model(&authdom.TenantOAuthClient{}).
				Where("default_key IS NOT NULL AND org != ?", org).
				Update("default_key", nil).Error; err != nil {
				return err
			}
		} else {
			// 不摘 default：唯一行保持 default；多行时降级当前 default 需先改指派。
			if isCurrentDefault {
				var others int64
				if err := tx.Model(&authdom.TenantOAuthClient{}).
					Where("org != ?", org).Count(&others).Error; err != nil {
					return err
				}
				if others > 0 {
					return ErrDefaultRequired
				}
				isDefault = true // 表内仅本行，维持 default
			}
		}

		row := authdom.TenantOAuthClient{
			Org:             org,
			ClientID:        clientID,
			ClientSecretEnc: secretEnc,
			CertEnc:         certEnc,
		}
		if isDefault {
			key := authdom.DefaultKeySentinel // 哨兵值（原为 org 名，见常量注释）
			row.DefaultKey = &key
		} else {
			row.DefaultKey = nil
		}
		// 已有行时保留原 created_at：Save 对含主键记录做全字段 UPDATE，
		// 零值 CreatedAt 会覆写为空时间（MySQL strict mode 报错、sqlite 丢数据）。
		if err == nil {
			row.CreatedAt = existing.CreatedAt
		}
		return tx.Save(&row).Error
	})
	if err != nil {
		if isDefaultKeyConflict(err) {
			return ErrDefaultConflict
		}
		return err
	}
	return nil
}

// Find 返回指定 org 的行；未注册返回 (nil, nil)。
func (r *TenantOAuthClientRepository) Find(org string) (*authdom.TenantOAuthClient, error) {
	var row authdom.TenantOAuthClient
	err := r.db.Where("org = ?", org).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// FindDefault 返回 default 行（default_key 非空）；无 default 返回 (nil, nil)。
func (r *TenantOAuthClientRepository) FindDefault() (*authdom.TenantOAuthClient, error) {
	var row authdom.TenantOAuthClient
	err := r.db.Where("default_key IS NOT NULL").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// List 返回全部行（isDefault 派生信息：DefaultKey != nil）。
func (r *TenantOAuthClientRepository) List() ([]authdom.TenantOAuthClient, error) {
	var rows []authdom.TenantOAuthClient
	err := r.db.Order("org ASC").Find(&rows).Error
	return rows, err
}

// Count 返回表行数（ops API 判断首行自动 default、delete 守卫用）。
func (r *TenantOAuthClientRepository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&authdom.TenantOAuthClient{}).Count(&n).Error
	return n, err
}

// Delete 删除指定 org。删的是 default 行且删后仍有其他行 → ErrDefaultRequired
// （最后一行即使是 default 也允许删除，删空表无 default 约束）。
func (r *TenantOAuthClientRepository) Delete(org string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var row authdom.TenantOAuthClient
		err := tx.Where("org = ?", org).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 幂等：删不存在的 org 视为成功
		}
		if err != nil {
			return err
		}
		if row.DefaultKey != nil {
			var others int64
			if err := tx.Model(&authdom.TenantOAuthClient{}).
				Where("org != ?", org).Count(&others).Error; err != nil {
				return err
			}
			if others > 0 {
				return ErrDefaultRequired
			}
		}
		return tx.Delete(&row).Error
	})
}
