package auth

import (
	authdom "control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// NormalizePendingRoles 是一次性数据归一（迁移语义修正）：
// 旧模型（角色快照）升级到本地角色管理模型时，AutoMigrate 给存量行填
// status=pending 的同时保留了旧 role 值（如 "member"），导致
// {role: "member", status: "pending"} 走合成规则的 OpNone 分支原样保留
// ——用户照常有 member 权限但管理页显示「待审批」，UI 与鉴权割裂。
//
// 归一规则：status=pending 的行 role 一律清空（升级后全员待审批，
// 由 admin 重新分配角色）。
//
// 幂等性论证：新模型下 pending 行本就不该携带角色——ApplyDecision 建
// pending 行时 role 恒为 ""；SetRole 分配角色时同步置 active；合成规则里
// 「合法 role + pending」的组合只会在旧存量数据中出现。因此重复执行无副作用。
//
// builtin 模式下 user_identities 表不会被写入，执行本函数无影响（无行可改），
// 为最小侵入，调用方（cmd/server/main.go）在 AutoMigrate 后无条件执行。
func NormalizePendingRoles(db *gorm.DB) error {
	return db.Model(&authdom.UserIdentity{}).
		Where("status = ? AND role <> ?", authdom.StatusPending, "").
		Update("role", "").Error
}
