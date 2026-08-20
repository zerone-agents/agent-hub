package repository

import "gorm.io/gorm"

// TenantOwned 限定查询到单个租户（租户私有资源：agents/providers/chat 等）。
func TenantOwned(db *gorm.DB, tenantID string) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

// TenantWithShared 限定查询到租户 + 全局共享行（tenant_id 为空串的内置数据：
// 内置 tools/mcps/providers 种子）。共享行只读——写路径不走这个 scope。
func TenantWithShared(db *gorm.DB, tenantID string) *gorm.DB {
	return db.Where("tenant_id IN (?, '')", tenantID)
}
