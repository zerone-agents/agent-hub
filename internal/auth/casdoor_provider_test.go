package auth

import (
	"errors"
	"testing"
	"time"

	authdom "control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// fakeMembershipStore 是 MembershipStore 的内存实现，供 provider 测试注入。
// 语义对齐 gorm 实现：OpCreate 建记录、OpUpdate 改 role/status、OpNone 只刷快照。
type fakeMembershipStore struct {
	recs      map[string]*authdom.UserIdentity // key: provider + "\x00" + externalID
	applyN    int                              // ApplyDecision 调用次数
	lastApply MembershipDecision               // 最后一次 ApplyDecision 的 decision
	findErr   error
	applyErr  error
}

func newFakeMembershipStore() *fakeMembershipStore {
	return &fakeMembershipStore{recs: make(map[string]*authdom.UserIdentity)}
}

func storeKey(provider, externalID string) string { return provider + "\x00" + externalID }

// seed 预置一条成员记录。
func (f *fakeMembershipStore) seed(provider, externalID, role, status string) {
	f.recs[storeKey(provider, externalID)] = &authdom.UserIdentity{
		Provider: provider, ExternalID: externalID, Role: role, Status: status,
	}
}

func (f *fakeMembershipStore) get(provider, externalID string) *authdom.UserIdentity {
	return f.recs[storeKey(provider, externalID)]
}

func (f *fakeMembershipStore) FindByExternalID(provider, externalID string) (*authdom.UserIdentity, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	rec := f.recs[storeKey(provider, externalID)]
	if rec == nil {
		return nil, nil
	}
	cp := *rec
	return &cp, nil
}

func (f *fakeMembershipStore) ApplyDecision(provider string, au *AuthUser, d MembershipDecision) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	f.applyN++
	f.lastApply = d
	key := storeKey(provider, au.ID)
	switch d.Op {
	case OpCreate:
		f.recs[key] = &authdom.UserIdentity{
			Provider: provider, ExternalID: au.ID, TenantID: au.TenantID,
			Username: au.Username, DisplayName: au.DisplayName, Email: au.Email,
			Role: d.Role, Status: d.Status, LastLoginAt: time.Now(),
		}
	case OpUpdate:
		if rec := f.recs[key]; rec != nil {
			rec.Role, rec.Status = d.Role, d.Status
			rec.TenantID = au.TenantID
			rec.Username, rec.DisplayName, rec.Email = au.Username, au.DisplayName, au.Email
		}
	case OpNone:
		// 仅刷新快照与 lastLoginAt，role/status 不动；测试无需模拟快照字段。
	}
	return nil
}

// ListByTenant/SetRole 是 MembershipStore 的用户管理扩展（Task 6）；
// provider 路径不触达，这里给出语义对齐的最小实现以维持接口实现。
func (f *fakeMembershipStore) ListByTenant(tenantID string) ([]authdom.UserIdentity, error) {
	var out []authdom.UserIdentity
	for _, rec := range f.recs {
		if rec.TenantID == tenantID {
			out = append(out, *rec)
		}
	}
	return out, nil
}

func (f *fakeMembershipStore) SetRole(provider, externalID, role, status string) error {
	if rec := f.recs[storeKey(provider, externalID)]; rec != nil {
		rec.Role, rec.Status = role, status
	}
	return nil
}

// newTestProvider 构造注入 fake store 的 provider，外部依赖由用例自行替换。
func newTestProvider(store MembershipStore) *CasdoorProvider {
	p := NewCasdoorProvider(store)
	p.parseToken = func(string) (*casdoorsdk.User, error) {
		return nil, errors.New("parseToken 未注入")
	}
	p.fetchUser = func(string, string) (*casdoorsdk.User, error) {
		return nil, errors.New("fetchUser 未注入")
	}
	return p
}

func TestCasdoorProviderMode(t *testing.T) {
	p := newTestProvider(newFakeMembershipStore())
	if p.Mode() != "casdoor" {
		t.Fatalf("mode = %q", p.Mode())
	}
	if _, err := p.RefreshToken(""); err == nil {
		t.Fatal("empty refresh must error")
	}
}

// token 里带 agent-hub-admin 角色，但本地成员表记录为 member：
// JWT roles claim 必须被完全忽略，角色以本地记录为准。
func TestValidateAccessTokenIgnoresTokenRoles(t *testing.T) {
	store := newFakeMembershipStore()
	store.seed("casdoor", "id1", authdom.RoleMember, authdom.StatusActive)
	p := newTestProvider(store)
	p.parseToken = func(string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{
			Id: "id1", Name: "alice", Owner: "tenant-acme",
			Roles: []*casdoorsdk.Role{{Name: "agent-hub-admin"}},
		}, nil
	}

	au, err := p.ValidateAccessToken("fake-token")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(au.Roles) != 1 || au.Roles[0] != authdom.RoleMember {
		t.Fatalf("roles = %v, want [member]（token 角色应被忽略）", au.Roles)
	}
	if au.TenantID != "tenant-acme" {
		t.Fatalf("tenant = %q", au.TenantID)
	}
	if store.applyN != 0 {
		t.Fatalf("ValidateAccessToken 不得写成员表，applyN = %d", store.applyN)
	}
}

// 本地无记录：返回空 roles 且不报错（待审批由下游拦截），也不得建记录。
func TestValidateAccessTokenUnknownUserGetsEmptyRoles(t *testing.T) {
	store := newFakeMembershipStore()
	p := newTestProvider(store)
	p.parseToken = func(string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: "ghost", Name: "ghost", Owner: "org1"}, nil
	}

	au, err := p.ValidateAccessToken("fake-token")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if au == nil {
		t.Fatal("expected AuthUser")
	}
	if len(au.Roles) != 0 {
		t.Fatalf("roles = %v, want 空", au.Roles)
	}
	if len(store.recs) != 0 || store.applyN != 0 {
		t.Fatalf("只读路径不得建记录: recs=%d applyN=%d", len(store.recs), store.applyN)
	}
}

// 本地记录 Role 为非法值（非 admin/maintainer/member 的非空串）：视为未分配，返回空 roles。
func TestValidateAccessTokenInvalidLocalRoleGetsEmptyRoles(t *testing.T) {
	store := newFakeMembershipStore()
	store.seed("casdoor", "id9", "superuser", authdom.StatusActive)
	p := newTestProvider(store)
	p.parseToken = func(string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: "id9", Name: "mallory", Owner: "org1"}, nil
	}

	au, err := p.ValidateAccessToken("fake-token")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(au.Roles) != 0 {
		t.Fatalf("非法本地角色应视为未分配，roles = %v", au.Roles)
	}
}

// IsAdmin=false 的新用户登录：合成 pending 记录并落库，返回空 roles。
// fetchUser 必须收到用户的 Owner 作为 org（跨组织解析 client 的关键参数）。
func TestSyncMembershipCreatesPendingForNewUser(t *testing.T) {
	store := newFakeMembershipStore()
	p := newTestProvider(store)
	var gotOrg string
	p.fetchUser = func(id, org string) (*casdoorsdk.User, error) {
		gotOrg = org
		return &casdoorsdk.User{Id: id, Name: "alice", Owner: "org1", IsAdmin: false}, nil
	}

	au, err := p.SyncMembership(&casdoorsdk.User{Id: "id1", Owner: "org1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(au.Roles) != 0 {
		t.Fatalf("roles = %v, want 空（pending）", au.Roles)
	}
	if store.lastApply.Op != OpCreate {
		t.Fatalf("op = %v, want OpCreate", store.lastApply.Op)
	}
	rec := store.get("casdoor", "id1")
	if rec == nil {
		t.Fatal("成员记录未落库")
	}
	if rec.Role != "" || rec.Status != authdom.StatusPending {
		t.Fatalf("rec = role %q status %q, want 空/pending", rec.Role, rec.Status)
	}
	if rec.Username != "alice" || rec.TenantID != "org1" {
		t.Fatalf("快照字段未写入: %+v", rec)
	}
	if gotOrg != "org1" {
		t.Fatalf("fetchUser 收到的 org = %q, want %q（须按用户 Owner 透传）", gotOrg, "org1")
	}
}

// IsAdmin=true：合成为 admin/active；再次登录 Op=None（不重复改写角色）。
func TestSyncMembershipOrgAdminBecomesAdmin(t *testing.T) {
	store := newFakeMembershipStore()
	p := newTestProvider(store)
	p.fetchUser = func(id, org string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: id, Name: "root", Owner: org, IsAdmin: true}, nil
	}

	au, err := p.SyncMembership(&casdoorsdk.User{Id: "id1", Owner: "org1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(au.Roles) != 1 || au.Roles[0] != authdom.RoleAdmin {
		t.Fatalf("roles = %v, want [admin]", au.Roles)
	}
	if store.lastApply.Op != OpCreate {
		t.Fatalf("首次 op = %v, want OpCreate", store.lastApply.Op)
	}
	rec := store.get("casdoor", "id1")
	if rec == nil || rec.Role != authdom.RoleAdmin || rec.Status != authdom.StatusActive {
		t.Fatalf("rec = %+v, want admin/active", rec)
	}

	// 再次登录：角色不变，decision Op=None（ApplyDecision 仍被调用以刷新快照）。
	if _, err := p.SyncMembership(&casdoorsdk.User{Id: "id1"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.lastApply.Op != OpNone {
		t.Fatalf("再次登录 op = %v, want OpNone（不重复写角色）", store.lastApply.Op)
	}
	rec = store.get("casdoor", "id1")
	if rec.Role != authdom.RoleAdmin || rec.Status != authdom.StatusActive {
		t.Fatalf("再次登录后 rec = role %q status %q, want admin/active 不变", rec.Role, rec.Status)
	}
}

// IsForbidden 用户登录回调：返回 error（不建记录）。
func TestSyncMembershipForbiddenUserErrors(t *testing.T) {
	store := newFakeMembershipStore()
	p := newTestProvider(store)
	p.fetchUser = func(id, org string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: id, Name: "banned", Owner: "org1", IsForbidden: true}, nil
	}

	if _, err := p.SyncMembership(&casdoorsdk.User{Id: "id1", Owner: "org1"}); err == nil {
		t.Fatal("IsForbidden 用户必须返回 error")
	}
	if len(store.recs) != 0 || store.applyN != 0 {
		t.Fatalf("禁用用户不得落库: recs=%d applyN=%d", len(store.recs), store.applyN)
	}
}

// CLI 身份查询：IsForbidden → (nil, false)（沿用既有行为）。
func TestGetUserIdentityDisabledUserRejected(t *testing.T) {
	store := newFakeMembershipStore()
	p := newTestProvider(store)
	p.fetchUser = func(id, org string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: id, Name: "banned", Owner: "org1", IsForbidden: true}, nil
	}

	if au, ok := p.GetUserIdentity("id1"); ok || au != nil {
		t.Fatalf("disabled (IsForbidden) casdoor user must be rejected, got %+v ok=%v", au, ok)
	}
}

// 双向同步：本地记录是 admin，但 casdoor 组织管理员已撤销（IsAdmin=false）
// → 返回空 roles，且本地记录被降级为 pending。
func TestGetUserIdentityBidirectionalSync(t *testing.T) {
	store := newFakeMembershipStore()
	store.seed("casdoor", "id1", authdom.RoleAdmin, authdom.StatusActive)
	store.recs[storeKey("casdoor", "id1")].TenantID = "org1"
	p := newTestProvider(store)
	var gotOrg string
	p.fetchUser = func(id, org string) (*casdoorsdk.User, error) {
		gotOrg = org
		return &casdoorsdk.User{Id: id, Name: "alice", Owner: "org1", IsAdmin: false}, nil
	}

	au, ok := p.GetUserIdentity("id1")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(au.Roles) != 0 {
		t.Fatalf("roles = %v, want 空（admin 已被双向降级）", au.Roles)
	}
	if store.lastApply.Op != OpUpdate {
		t.Fatalf("op = %v, want OpUpdate", store.lastApply.Op)
	}
	rec := store.get("casdoor", "id1")
	if rec.Role != "" || rec.Status != authdom.StatusPending {
		t.Fatalf("rec = role %q status %q, want 空/pending（降级落库）", rec.Role, rec.Status)
	}
	if gotOrg != "org1" {
		t.Fatalf("fetchUser 收到的 org = %q, want %q（应取本地记录的 TenantID）", gotOrg, "org1")
	}
}

// CLI 身份查询：casdoor 侧查无此人 → (nil, false)。无本地记录时 fetchUser
// 收到空串 org（走全局 client，保持存量行为）。
func TestGetUserIdentityUnknownUser(t *testing.T) {
	store := newFakeMembershipStore()
	p := newTestProvider(store)
	var gotOrg = "sentinel"
	p.fetchUser = func(id, org string) (*casdoorsdk.User, error) {
		gotOrg = org
		return nil, nil
	}

	if au, ok := p.GetUserIdentity("ghost"); ok || au != nil {
		t.Fatalf("unknown user must be rejected, got %+v ok=%v", au, ok)
	}
	if gotOrg != "" {
		t.Fatalf("无本地记录时 fetchUser org = %q, want 空串", gotOrg)
	}
}
