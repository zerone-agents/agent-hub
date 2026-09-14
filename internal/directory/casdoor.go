package directory

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"control-panel/internal/auth"
	authdom "control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// providerCasdoor 是 user_identities 表的 provider 值；casdoor 模式下
// 外部身份只来自 casdoor。
const providerCasdoor = "casdoor"

// NewCasdoorDirectory 构造 CasdoorDirectory。角色与审批状态以本地成员表
// （store，user_identities）为真实源；client 仅用于 casdoor 侧字段的读改
// （is_admin / is_forbidden / password）。
func NewCasdoorDirectory(resolveClient ClientResolver, store auth.MembershipStore) *CasdoorDirectory {
	return &CasdoorDirectory{resolveClient: resolveClient, store: store}
}

// ListUsers 列出租户全部成员（本地 user_identities，按记录 id 升序）。
// casdoor 全量拉一次用户建 is_forbidden 映射（无 N+1）：被禁用成员的
// Status 合成为 disabled，其余按本地审批状态原样展示（pending/active），
// Role 也按本地记录原样透传（空 = 未分配）。
func (d *CasdoorDirectory) ListUsers(tenantID string) ([]ManagedUser, error) {
	recs, err := d.store.ListByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	users, err := d.resolveClient(tenantID).GetUsers()
	if err != nil {
		return nil, err
	}
	forbidden := make(map[string]bool, len(users))
	for _, u := range users {
		if u != nil {
			forbidden[u.Id] = u.IsForbidden
		}
	}
	out := make([]ManagedUser, 0, len(recs))
	for i := range recs {
		rec := &recs[i]
		status := rec.Status
		if forbidden[rec.ExternalID] {
			status = authdom.StatusDisabled
		}
		out = append(out, ManagedUser{
			ID: rec.ExternalID, Username: rec.Username, DisplayName: rec.DisplayName,
			Email: rec.Email, Role: rec.Role, Status: status,
			CreatedAt: rec.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// localTenantRecord 查本地成员记录并校验租户归属；无记录或跨租户返回
// ErrUserNotFound（404），store 故障原样透传（502）。
func (d *CasdoorDirectory) localTenantRecord(tenantID, userID string) (*authdom.UserIdentity, error) {
	rec, err := d.store.FindByExternalID(providerCasdoor, userID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.TenantID != tenantID {
		return nil, ErrUserNotFound
	}
	return rec, nil
}

// getTenantUser 从 casdoor 拉取用户并校验租户归属。SDK 错误原样透传
// （handler 映射为 502）；仅用户不存在或跨租户时返回 ErrUserNotFound（404）。
func (d *CasdoorDirectory) getTenantUser(tenantID, userID string) (*casdoorsdk.User, error) {
	u, err := d.resolveClient(tenantID).GetUserByUserId(userID)
	if err != nil {
		return nil, err
	}
	if u == nil || u.Owner != tenantID {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// UpdateRole 更新成员角色（本地真实源）。admin 的任命/降级需双写 casdoor
// 的 is_admin 标志，且「Casdoor 先行」：casdoor 写入失败或被拒（ok=false）
// 时整体报错、本地不动——避免本地已显示 admin 而 casdoor 侧组织管理员
// 权限未生效的不一致。pending 成员被分配角色视同审批通过（status 同步
// 置 active），其余成员保持原状态。
//
// 回执（spec §3.2）：远端写入前的任何失败返回 (nil, err)（规则 4）；远端
// 成功后本地 SetRole 失败返回 rcpt(RemoteApplied=true, LocalApplied=false)
// + err（partial 信号，规则 3）；RemoteApplied 在无需双写时同样为 true
// （无远端变更视为已生效）。EffectiveStatus 由「casdoor is_forbidden +
// 本地状态」合成，隐藏的 pending 审批不漏审。
func (d *CasdoorDirectory) UpdateRole(tenantID, userID, role, actorID string) (*authdom.MutationReceipt, error) {
	if userID == actorID {
		return nil, ErrSelfOperation
	}
	if !authdom.IsValidRole(role) {
		return nil, ErrInvalidRole
	}
	rec, err := d.localTenantRecord(tenantID, userID)
	if err != nil {
		return nil, err
	}
	// admin 任免先改 casdoor 的 is_admin。判断不能只看本地记录
	// （rec.Role == admin）：若目标用户在 Casdoor 控制台被直接提为组织
	// 管理员（本地记录仍是 member），对其降级时也必须双写，否则下次
	// 登录合成规则会按 IsAdmin=true 自动提回 admin，管理动作静默失效。
	// 因此先拉取 casdoor 侧用户，双写条件补上 u.IsAdmin。
	// 「Casdoor 先行」不变：casdoor 写入失败或被拒（ok=false）时整体报错、
	// 本地不动。
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return nil, err
	}
	statusAfter := rec.Status
	if statusAfter == authdom.StatusPending {
		statusAfter = authdom.StatusActive // 审批动作 = 分配角色
	}
	eff := func(forbidden bool, st string) authdom.UserStatus {
		if forbidden {
			return authdom.UserStatus("disabled")
		}
		return authdom.UserStatus(st)
	}
	rcpt := &authdom.MutationReceipt{
		RoleBefore: authdom.Role(rec.Role), RoleAfter: authdom.Role(role),
		StatusBefore: authdom.UserStatus(rec.Status), StatusAfter: authdom.UserStatus(statusAfter),
		EffectiveStatusBefore: eff(u.IsForbidden, rec.Status),
		EffectiveStatusAfter:  eff(u.IsForbidden, statusAfter),
	}
	if role == authdom.RoleAdmin || rec.Role == authdom.RoleAdmin || u.IsAdmin {
		u.IsAdmin = role == authdom.RoleAdmin
		ok, err := d.resolveClient(tenantID).UpdateUserForColumns(u, []string{"is_admin"})
		if err != nil {
			return nil, err // 远端未生效：receipt=nil（规则 4）
		}
		if !ok {
			return nil, ErrUpdateRejected
		}
	}
	rcpt.RemoteApplied = true
	if err := d.store.SetRole(providerCasdoor, userID, role, statusAfter); err != nil {
		rcpt.LocalApplied = false
		return rcpt, err // partial：远端已生效、本地失败（规则 3）
	}
	rcpt.LocalApplied = true
	return rcpt, nil
}

// SetDisabled 直通设置 casdoor 的 is_forbidden 标志（禁用状态不落本地表，
// ListUsers 时实时合成）。本地成员记录不存在的用户不可操作；pending
// 成员同样可禁用（无额外防护）。
//
// 回执（spec §3.2）：远端单写、无两阶段窗口，成功即 RemoteApplied=
// LocalApplied=true；EffectiveStatus 前后由「casdoor IsForbidden 前后值 +
// 本地 rec.Status」合成，Role/Status 字段取本地 rec 原值不变。
func (d *CasdoorDirectory) SetDisabled(tenantID, userID string, disabled bool, actorID string) (*authdom.MutationReceipt, error) {
	if userID == actorID {
		return nil, ErrSelfOperation
	}
	rec, err := d.localTenantRecord(tenantID, userID)
	if err != nil {
		return nil, err
	}
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return nil, err
	}
	effBefore := authdom.UserStatus(rec.Status)
	if u.IsForbidden {
		effBefore = authdom.UserStatus("disabled")
	}
	u.IsForbidden = disabled
	ok, err := d.resolveClient(tenantID).UpdateUserForColumns(u, []string{"is_forbidden"})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrUpdateRejected
	}
	effAfter := authdom.UserStatus(rec.Status)
	if disabled {
		effAfter = authdom.UserStatus("disabled")
	}
	return &authdom.MutationReceipt{
		RoleBefore: authdom.Role(rec.Role), RoleAfter: authdom.Role(rec.Role),
		StatusBefore: authdom.UserStatus(rec.Status), StatusAfter: authdom.UserStatus(rec.Status),
		EffectiveStatusBefore: effBefore, EffectiveStatusAfter: effAfter,
		RemoteApplied: true, LocalApplied: true, // 远端单写，无两阶段窗口
	}, nil
}

// ResetPassword 直通设置随机密码（casdoor 服务端哈希），明文只返回一次。
func (d *CasdoorDirectory) ResetPassword(tenantID, userID, actorID string) (string, error) {
	if userID == actorID {
		return "", ErrSelfOperation
	}
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return "", err
	}
	b := make([]byte, 12) // 24 个十六进制字符
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	plain := "Reset!" + hex.EncodeToString(b)
	u.Password = plain
	ok, err := d.resolveClient(tenantID).UpdateUserForColumns(u, []string{"password"})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrUpdateRejected
	}
	return plain, nil
}
