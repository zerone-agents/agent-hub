package auth

// Role / UserStatus：命名类型（spec §3.2）。现有 RoleAdmin/StatusActive 等
// 无类型字符串常量可直接赋值，其他调用点零改动。
type Role string
type UserStatus string

// MutationReceipt 是用户变更的权威回执：before/after 与两阶段写入的实际生效情况。
type MutationReceipt struct {
	RoleBefore            Role
	RoleAfter             Role
	StatusBefore          UserStatus // 本地权威状态（隐式审批判定依据；非 ListUsers 投影）
	StatusAfter           UserStatus
	EffectiveStatusBefore UserStatus // 管理视图合成状态（远端 forbidden → disabled，否则本地）
	EffectiveStatusAfter  UserStatus // builtin 无远端：Effective == Status
	RemoteApplied         bool       // casdoor 两阶段写：远端是否已生效；builtin 恒 true
	LocalApplied          bool       // 必须由 RowsAffected 判定，仅 .Error==nil 不得视为已生效
}
