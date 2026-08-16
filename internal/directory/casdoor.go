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
func NewCasdoorDirectory(client UserClient, store auth.MembershipStore) *CasdoorDirectory {
	return &CasdoorDirectory{client: client, store: store}
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
	users, err := d.client.GetUsers()
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
	u, err := d.client.GetUserByUserId(userID)
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
func (d *CasdoorDirectory) UpdateRole(tenantID, userID, role, actorID string) error {
	if userID == actorID {
		return ErrSelfOperation
	}
	if !authdom.IsValidRole(role) {
		return ErrInvalidRole
	}
	rec, err := d.localTenantRecord(tenantID, userID)
	if err != nil {
		return err
	}
	// admin 任免（升为 admin，或从 admin 降级）先改 casdoor 的 is_admin
	if role == authdom.RoleAdmin || rec.Role == authdom.RoleAdmin {
		u, err := d.getTenantUser(tenantID, userID)
		if err != nil {
			return err
		}
		u.IsAdmin = role == authdom.RoleAdmin
		ok, err := d.client.UpdateUserForColumns(u, []string{"is_admin"})
		if err != nil {
			return err
		}
		if !ok {
			return ErrUpdateRejected
		}
	}
	status := rec.Status
	if status == authdom.StatusPending {
		status = authdom.StatusActive // 审批动作 = 分配角色
	}
	return d.store.SetRole(providerCasdoor, userID, role, status)
}

// SetDisabled 直通设置 casdoor 的 is_forbidden 标志（禁用状态不落本地表，
// ListUsers 时实时合成）。本地成员记录不存在的用户不可操作；pending
// 成员同样可禁用（无额外防护）。
func (d *CasdoorDirectory) SetDisabled(tenantID, userID string, disabled bool, actorID string) error {
	if userID == actorID {
		return ErrSelfOperation
	}
	if _, err := d.localTenantRecord(tenantID, userID); err != nil {
		return err
	}
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return err
	}
	u.IsForbidden = disabled
	ok, err := d.client.UpdateUserForColumns(u, []string{"is_forbidden"})
	if err != nil {
		return err
	}
	if !ok {
		return ErrUpdateRejected
	}
	return nil
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
	ok, err := d.client.UpdateUserForColumns(u, []string{"password"})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrUpdateRejected
	}
	return plain, nil
}
