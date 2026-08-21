package repository

import (
	"errors"

	"gorm.io/gorm"
)

// ErrTenantIDRequired 由没有系统写通道的资源（skills/scenes）的写方法在
// tenantID 为空串时返回：这类资源的空租户调用不是合法系统路径，放行会把
// 租户私有行盖章成 tenant_id=” 的全局共享行。tools/mcps 有合法系统通道
// （SeedBuiltins/seedBuiltinMcp 元数据刷新），不适用此错误。
var ErrTenantIDRequired = errors.New("tenantID required")

// TenantOwned 限定查询到单个租户（租户私有资源：agents/providers/chat 等）。
func TenantOwned(db *gorm.DB, tenantID string) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

// TenantWithShared 限定查询到租户 + 全局共享行（tenant_id 为空串的内置数据：
// 内置 tools/mcps/providers 种子）。共享行只读——写路径不走这个 scope。
func TenantWithShared(db *gorm.DB, tenantID string) *gorm.DB {
	return db.Where("tenant_id IN (?, '')", tenantID)
}
