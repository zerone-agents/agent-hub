package auth

import (
	"errors"
	"fmt"

	authdom "control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// CasdoorProvider 适配 Casdoor OAuth 流程（casdoor.go 中的包级函数）到
// Provider 接口。构造前必须先运行 InitCasdoor 以初始化包级 client。
//
// 角色来源：Casdoor JWT 的 roles claim 完全忽略，角色以本地成员表
// （MembershipStore，user_identities）为准——登录/身份查询时经
// SynthesizeMembership 合成并落库，token 校验路径只读不建记录。
type CasdoorProvider struct {
	store MembershipStore
	// parseToken 解析 access token 为 casdoor 用户（默认 GetUserInfo，
	// 走 JWT 解析）。测试可注入假实现，避免构造真 JWT。
	parseToken func(string) (*casdoorsdk.User, error)
	// fetchUser 按 user Id + 所属组织经 Admin API 拉取权威用户数据（默认
	// defaultFetchUser，经 ClientForOrg(org) 解析 client）。测试可注入假实现。
	fetchUser func(userID, org string) (*casdoorsdk.User, error)
}

// NewCasdoorProvider 构造 CasdoorProvider。store 为本地成员表句柄，
// 角色/审批状态的真实源。
func NewCasdoorProvider(store MembershipStore) *CasdoorProvider {
	return &CasdoorProvider{
		store:      store,
		parseToken: GetUserInfo,
		fetchUser:  defaultFetchUser,
	}
}

// defaultFetchUser 走 Casdoor Admin API 按 user Id 拉取用户。按用户所属组织
// 解析 client（ClientForOrg，org 空串回落全局 client）：否则 SDK 的
// GetUserByUserId 会把 owner 固定为全局组织，跨组织用户查不到。
func defaultFetchUser(userID, org string) (*casdoorsdk.User, error) {
	c := ClientForOrg(org)
	if c == nil {
		return nil, errors.New("casdoor client 未初始化")
	}
	return c.GetUserByUserId(userID)
}

// Mode identifies this provider.
func (p *CasdoorProvider) Mode() string { return "casdoor" }

// toAuthUser 纯字段映射：casdoor User → AuthUser。TenantID 取 Owner；
// **空 Owner 直接拒绝**（issue #78：此前回退 "default"，会把缺组织的
// casdoor 用户静默归入 builtin 租户，该租户还会作为可信 org 下发到回传
// 配置——可信身份边界不允许由坏身份数据推导租户）。roles 由调用方按
// 本地成员表/合成结果填入。
func toAuthUser(u *casdoorsdk.User, roles []string) (*AuthUser, error) {
	if u.Owner == "" {
		return nil, fmt.Errorf("casdoor user %s has no owner (organization)", u.Id)
	}
	return &AuthUser{
		ID:          u.Id,
		Username:    u.Name,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Avatar:      u.Avatar,
		Roles:       roles,
		TenantID:    u.Owner,
	}, nil
}

// rolesFromRecord 从本地成员记录取角色（只读路径用）。无记录或 Role 为
// 非法值（admin/maintainer/member 之外的非空串）一律视为未分配，返回空切片。
func rolesFromRecord(rec *authdom.UserIdentity) []string {
	if rec == nil || !authdom.IsValidRole(rec.Role) {
		return []string{}
	}
	return []string{rec.Role}
}

// rolesFromDecision 从合成结果取角色；空或非法值一律视为未分配。
func rolesFromDecision(d MembershipDecision) []string {
	if !authdom.IsValidRole(d.Role) {
		return []string{}
	}
	return []string{d.Role}
}

// ValidateAccessToken 解析 Casdoor JWT 并返回归一化用户。roles claim 完全
// 忽略，角色从本地成员表只读查询：无记录/非法值 → 空 roles（待审批由下游
// 拦截），本路径绝不建记录。
func (p *CasdoorProvider) ValidateAccessToken(token string) (*AuthUser, error) {
	u, err := p.parseToken(token)
	if err != nil {
		return nil, err
	}
	rec, err := p.store.FindByExternalID("casdoor", u.Id)
	if err != nil {
		return nil, err
	}
	au, err := toAuthUser(u, rolesFromRecord(rec))
	if err != nil {
		return nil, err
	}
	return au, nil
}

// RefreshToken exchanges a Casdoor refresh token for a fresh token pair.
func (p *CasdoorProvider) RefreshToken(refreshToken string) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, errors.New("refresh token is empty")
	}
	resp, err := RefreshAccessToken(refreshToken)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	}, nil
}

// RevokeToken revokes a Casdoor access or refresh token. The package-level
// RevokeToken is called by method dispatch (receiver-bound), so it does not
// shadow itself here.
func (p *CasdoorProvider) RevokeToken(token string) error {
	return RevokeToken(token)
}

// SyncMembership 登录回调专用：经 Admin API 拉取权威 IsAdmin/IsForbidden，
// 与本地成员记录合成角色后落库，返回带合成角色的 AuthUser。
// IsForbidden 用户返回 error；合成/落库失败同样返回 error（调用方记日志，
// 不阻断登录）。
func (p *CasdoorProvider) SyncMembership(u *casdoorsdk.User) (*AuthUser, error) {
	if u == nil {
		return nil, errors.New("casdoor user 为空")
	}
	// 按用户所属组织解析 client（否则 SDK 的 owner 参数固定为全局组织，
	// 跨组织用户查不到）。
	fresh, err := p.fetchUser(u.Id, u.Owner)
	if err != nil {
		return nil, fmt.Errorf("拉取 casdoor 用户失败: %w", err)
	}
	if fresh == nil {
		return nil, fmt.Errorf("casdoor 用户不存在: %s", u.Id)
	}
	if fresh.IsForbidden {
		return nil, fmt.Errorf("casdoor 用户已被禁用: %s", u.Id)
	}
	rec, err := p.store.FindByExternalID("casdoor", fresh.Id)
	if err != nil {
		return nil, err
	}
	d := SynthesizeMembership(fresh.IsAdmin, rec)
	au, err := toAuthUser(fresh, rolesFromDecision(d))
	if err != nil {
		return nil, err
	}
	if err := p.store.ApplyDecision("casdoor", au, d); err != nil {
		return nil, err
	}
	return au, nil
}

// GetUserIdentity 从 Casdoor 查询用户当前归一化身份（CLI-token 中间件路径；
// 缓存机制在 jwtutil，不在此层）。规则与登录路径一致：IsForbidden 拒绝；
// 合成结果有变更（Op≠OpNone）时落库——双向同步（casdoor 组织管理员任免
// 反映到本地角色）在此生效。bool 为 false 表示用户未知、被禁用或查询失败。
func (p *CasdoorProvider) GetUserIdentity(userID string) (*AuthUser, bool) {
	// 先取本地记录，用其 TenantID 按组织解析 client（否则 SDK 的 owner 参数
	// 固定为全局组织，跨组织用户查不到）；无记录时空串走全局 client，
	// 保持存量行为。
	rec, err := p.store.FindByExternalID("casdoor", userID)
	if err != nil {
		return nil, false
	}
	org := ""
	if rec != nil {
		org = rec.TenantID
	}
	u, err := p.fetchUser(userID, org)
	if err != nil || u == nil {
		return nil, false
	}
	if u.IsForbidden {
		return nil, false
	}
	rec, err = p.store.FindByExternalID("casdoor", u.Id)
	if err != nil {
		return nil, false
	}
	d := SynthesizeMembership(u.IsAdmin, rec)
	au, err := toAuthUser(u, rolesFromDecision(d))
	if err != nil {
		return nil, false
	}
	if d.Op != OpNone {
		if err := p.store.ApplyDecision("casdoor", au, d); err != nil {
			return nil, false
		}
	}
	return au, true
}
