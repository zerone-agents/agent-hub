package auth

// 本文件原 UpsertIdentity 已删除：user_identities 影子表的登录时 upsert
// 由 MembershipStore（membership_store.go）取代。角色/状态不再随登录快照
// 刷新，只能经用户管理或 SynthesizeMembership 合成规则改变。
// 调用方接线见 Task 4（internal/handler/auth.go 的 Callback）。
