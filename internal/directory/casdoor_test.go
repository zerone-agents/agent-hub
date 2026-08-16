package directory

import (
	"errors"
	"sort"
	"testing"
	"time"

	"control-panel/internal/auth"
	authdom "control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// ===================== fakes =====================

type updateCall struct {
	user    *casdoorsdk.User
	columns []string
}

type setRoleCall struct {
	externalID string
	role       string
	status     string
}

// fakeClient 模拟 casdoor Admin API。log 非空时 UpdateUserForColumns 追加
// "casdoor:update" 事件，与 fakeStore 的 "store:setrole" 共享同一日志，
// 用于交叉断言「Casdoor 先行」的写入顺序。
type fakeClient struct {
	users          []*casdoorsdk.User
	getErr         error
	getByIDErr     error // GetUserByUserId 注入错误（优先返回）
	updateRejected bool  // true 时 UpdateUserForColumns 返回 (false, nil)
	updates        []updateCall
	getByIDCalls   int
	log            *[]string
}

func (f *fakeClient) GetUsers() ([]*casdoorsdk.User, error) { return f.users, f.getErr }

func (f *fakeClient) GetUserByUserId(id string) (*casdoorsdk.User, error) {
	f.getByIDCalls++
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	for _, u := range f.users {
		if u.Id == id {
			return u, nil
		}
	}
	return nil, nil // casdoor SDK 对未知用户返回 (nil, nil)
}

func (f *fakeClient) UpdateUserForColumns(u *casdoorsdk.User, cols []string) (bool, error) {
	f.updates = append(f.updates, updateCall{user: u, columns: cols})
	if f.log != nil {
		*f.log = append(*f.log, "casdoor:update")
	}
	if f.updateRejected {
		return false, nil
	}
	return true, nil
}

// fakeStore 是 MembershipStore 的内存实现（directory 用例）。ListByTenant
// 按记录 ID 升序，对齐 gorm 实现；SetRole 记录调用并更新记录。
type fakeStore struct {
	recs     map[string]*authdom.UserIdentity // key: externalID
	nextID   uint64
	listErr  error
	findErr  error
	setErr   error
	setCalls []setRoleCall
	log      *[]string
}

var seedBase = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

func newFakeStore() *fakeStore {
	return &fakeStore{recs: make(map[string]*authdom.UserIdentity)}
}

// seed 预置一条成员记录，ID 自增（决定 ListByTenant 顺序），CreatedAt 固定
// 便于断言透传。
func (f *fakeStore) seed(externalID, tenantID, username, role, status string) *authdom.UserIdentity {
	f.nextID++
	rec := &authdom.UserIdentity{
		ID: f.nextID, Provider: "casdoor", ExternalID: externalID, TenantID: tenantID,
		Username: username, DisplayName: username, Email: username + "@x.com",
		Role: role, Status: status,
		CreatedAt: seedBase.Add(time.Duration(f.nextID) * time.Minute),
	}
	f.recs[externalID] = rec
	return rec
}

func (f *fakeStore) FindByExternalID(provider, externalID string) (*authdom.UserIdentity, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	rec := f.recs[externalID]
	if rec == nil {
		return nil, nil
	}
	cp := *rec
	return &cp, nil
}

func (f *fakeStore) ListByTenant(tenantID string) ([]authdom.UserIdentity, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []authdom.UserIdentity
	for _, rec := range f.recs {
		if rec.TenantID == tenantID {
			out = append(out, *rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeStore) ApplyDecision(provider string, au *auth.AuthUser, d auth.MembershipDecision) error {
	return nil // directory 用例不触达合成路径
}

func (f *fakeStore) SetRole(provider, externalID, role, status string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setCalls = append(f.setCalls, setRoleCall{externalID: externalID, role: role, status: status})
	if f.log != nil {
		*f.log = append(*f.log, "store:setrole")
	}
	if rec := f.recs[externalID]; rec != nil {
		rec.Role, rec.Status = role, status
	}
	return nil
}

func (f *fakeStore) rec(externalID string) *authdom.UserIdentity { return f.recs[externalID] }

// ===================== ListUsers =====================

func TestListUsersFromLocalTable(t *testing.T) {
	// 列表以本地 user_identities 为源：tenant-b 记录不出现；
	// casdoor 全量拉一次 is_forbidden（无 N+1）：bob 显示 disabled；
	// pending 原样、Role "" 保留；CreatedAt 取本地记录。
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleAdmin, authdom.StatusActive)
	fs.seed("2", "tenant-a", "bob", authdom.RoleMember, authdom.StatusActive)
	fs.seed("3", "tenant-a", "carol", "", authdom.StatusPending)
	fs.seed("9", "tenant-b", "dave", authdom.RoleMember, authdom.StatusActive) // 他租户
	fc := &fakeClient{users: []*casdoorsdk.User{
		{Id: "2", Name: "bob", Owner: "tenant-a", IsForbidden: true},
		{Id: "9", Name: "dave", Owner: "tenant-b"}, // 他租户用户仅在映射里，不产生条目
	}}
	d := NewCasdoorDirectory(fc, fs)

	got, err := d.ListUsers("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d users, want 3 (tenant-b excluded)", len(got))
	}
	if got[0].Username != "alice" || got[0].Role != "admin" || got[0].Status != "active" {
		t.Fatalf("alice: %+v", got[0])
	}
	if got[1].Username != "bob" || got[1].Role != "member" || got[1].Status != "disabled" {
		t.Fatalf("bob 应因 casdoor is_forbidden 显示 disabled: %+v", got[1])
	}
	if got[2].Username != "carol" || got[2].Role != "" || got[2].Status != "pending" {
		t.Fatalf("carol pending 原样、Role 空保留: %+v", got[2])
	}
	if want := seedBase.Add(1 * time.Minute).Format(time.RFC3339); got[0].CreatedAt != want {
		t.Fatalf("CreatedAt 应取本地记录: got %q want %q", got[0].CreatedAt, want)
	}
}

func TestListUsersEmptyTenant(t *testing.T) {
	d := NewCasdoorDirectory(&fakeClient{}, newFakeStore())
	got, err := d.ListUsers("tenant-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}
}

func TestListUsersErrorsPropagate(t *testing.T) {
	// store 错误与 casdoor GetUsers 错误都要透传（handler 映射 502）
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
	d := NewCasdoorDirectory(&fakeClient{getErr: errors.New("casdoor boom")}, fs)
	if _, err := d.ListUsers("tenant-a"); err == nil {
		t.Fatal("want error from GetUsers")
	}

	fs2 := newFakeStore()
	fs2.listErr = errors.New("db boom")
	d2 := NewCasdoorDirectory(&fakeClient{}, fs2)
	if _, err := d2.ListUsers("tenant-a"); err == nil {
		t.Fatal("want error from ListByTenant")
	}
}

// ===================== UpdateRole =====================

func TestUpdateRoleMemberIsLocalOnly(t *testing.T) {
	// 非 admin 的角色调整（member→maintainer）纯本地：fakeClient 零调用
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
	fc := &fakeClient{}
	d := NewCasdoorDirectory(fc, fs)

	if err := d.UpdateRole("tenant-a", "1", authdom.RoleMaintainer, "actor-9"); err != nil {
		t.Fatal(err)
	}
	if len(fc.updates) != 0 || fc.getByIDCalls != 0 {
		t.Fatalf("fakeClient 必须零调用, updates=%d getByID=%d", len(fc.updates), fc.getByIDCalls)
	}
	if len(fs.setCalls) != 1 {
		t.Fatalf("store 写入次数 = %d, want 1", len(fs.setCalls))
	}
	if c := fs.setCalls[0]; c.role != authdom.RoleMaintainer || c.status != authdom.StatusActive {
		t.Fatalf("setCall: %+v（active 用户状态保持）", c)
	}
}

func TestUpdateRoleToAdminWritesCasdoorFirst(t *testing.T) {
	t.Run("casdoor 先行成功后写本地", func(t *testing.T) {
		fs := newFakeStore()
		fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
		fc := &fakeClient{users: []*casdoorsdk.User{
			{Id: "1", Name: "alice", Owner: "tenant-a", IsAdmin: false},
		}}
		var events []string
		fc.log, fs.log = &events, &events
		d := NewCasdoorDirectory(fc, fs)

		if err := d.UpdateRole("tenant-a", "1", authdom.RoleAdmin, "actor-9"); err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 || events[0] != "casdoor:update" || events[1] != "store:setrole" {
			t.Fatalf("写入顺序必须 Casdoor 先行: %v", events)
		}
		if len(fc.updates) != 1 {
			t.Fatalf("casdoor 更新次数 = %d, want 1", len(fc.updates))
		}
		if !fc.updates[0].user.IsAdmin || fc.updates[0].columns[0] != "is_admin" {
			t.Fatalf("update call: %+v cols %v", fc.updates[0].user, fc.updates[0].columns)
		}
		rec := fs.rec("1")
		if rec.Role != authdom.RoleAdmin || rec.Status != authdom.StatusActive {
			t.Fatalf("本地记录: %+v", rec)
		}
	})

	t.Run("casdoor 被拒则本地不动", func(t *testing.T) {
		fs := newFakeStore()
		fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
		fc := &fakeClient{
			users:          []*casdoorsdk.User{{Id: "1", Name: "alice", Owner: "tenant-a"}},
			updateRejected: true,
		}
		d := NewCasdoorDirectory(fc, fs)

		if err := d.UpdateRole("tenant-a", "1", authdom.RoleAdmin, "actor-9"); !errors.Is(err, ErrUpdateRejected) {
			t.Fatalf("got %v, want ErrUpdateRejected", err)
		}
		if len(fs.setCalls) != 0 {
			t.Fatalf("casdoor 失败后本地必须零写入, got %d", len(fs.setCalls))
		}
		if rec := fs.rec("1"); rec.Role != authdom.RoleMember || rec.Status != authdom.StatusActive {
			t.Fatalf("本地记录被意外改动: %+v", rec)
		}
	})
}

func TestDemoteAdminWritesCasdoorFirst(t *testing.T) {
	// admin → member：先 UpdateUserForColumns(is_admin=false)，成功才写本地
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleAdmin, authdom.StatusActive)
	fc := &fakeClient{users: []*casdoorsdk.User{
		{Id: "1", Name: "alice", Owner: "tenant-a", IsAdmin: true},
	}}
	var events []string
	fc.log, fs.log = &events, &events
	d := NewCasdoorDirectory(fc, fs)

	if err := d.UpdateRole("tenant-a", "1", authdom.RoleMember, "actor-9"); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "casdoor:update" || events[1] != "store:setrole" {
		t.Fatalf("写入顺序必须 Casdoor 先行: %v", events)
	}
	if fc.updates[0].user.IsAdmin {
		t.Fatalf("降级应写 is_admin=false: %+v", fc.updates[0].user)
	}
	if fc.updates[0].columns[0] != "is_admin" {
		t.Fatalf("columns: %v", fc.updates[0].columns)
	}
	if rec := fs.rec("1"); rec.Role != authdom.RoleMember {
		t.Fatalf("本地角色: %+v", rec)
	}
}

func TestUpdateRolePendingUserBecomesActive(t *testing.T) {
	// 审批动作 = 分配角色：pending 用户被分配 member 时 status 同步置 active
	fs := newFakeStore()
	fs.seed("3", "tenant-a", "carol", "", authdom.StatusPending)
	fc := &fakeClient{}
	d := NewCasdoorDirectory(fc, fs)

	if err := d.UpdateRole("tenant-a", "3", authdom.RoleMember, "actor-9"); err != nil {
		t.Fatal(err)
	}
	if len(fs.setCalls) != 1 {
		t.Fatalf("setCalls = %d, want 1", len(fs.setCalls))
	}
	if c := fs.setCalls[0]; c.role != authdom.RoleMember || c.status != authdom.StatusActive {
		t.Fatalf("setCall: %+v, want member/active", c)
	}
	if rec := fs.rec("3"); rec.Role != authdom.RoleMember || rec.Status != authdom.StatusActive {
		t.Fatalf("本地记录: %+v", rec)
	}
	if len(fc.updates) != 0 || fc.getByIDCalls != 0 {
		t.Fatalf("非 admin 目标不应触达 casdoor: updates=%d getByID=%d", len(fc.updates), fc.getByIDCalls)
	}
}

func TestUpdateRoleRejectsSelfAndInvalid(t *testing.T) {
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
	fs.seed("7", "tenant-b", "zoe", authdom.RoleMember, authdom.StatusActive)
	fc := &fakeClient{}
	d := NewCasdoorDirectory(fc, fs)

	if err := d.UpdateRole("tenant-a", "1", authdom.RoleAdmin, "1"); !errors.Is(err, ErrSelfOperation) {
		t.Fatalf("self: %v", err)
	}
	if err := d.UpdateRole("tenant-a", "1", "superuser", "actor"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("invalid role: %v", err)
	}
	if err := d.UpdateRole("tenant-a", "missing", authdom.RoleAdmin, "actor"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing: %v", err)
	}
	// 本地记录属于 tenant-b，跨租户操作视为不存在
	if err := d.UpdateRole("tenant-a", "7", authdom.RoleAdmin, "actor"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("cross-tenant: %v", err)
	}
	if len(fs.setCalls) != 0 || len(fc.updates) != 0 {
		t.Fatalf("全部拒绝路径必须零写入: setCalls=%d updates=%d", len(fs.setCalls), len(fc.updates))
	}
}

// ===================== SetDisabled / ResetPassword =====================

func TestSetDisabledStillPassthrough(t *testing.T) {
	t.Run("is_forbidden 直通且不落本地", func(t *testing.T) {
		fs := newFakeStore()
		fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
		fc := &fakeClient{users: []*casdoorsdk.User{
			{Id: "1", Name: "alice", Owner: "tenant-a"},
		}}
		d := NewCasdoorDirectory(fc, fs)

		if err := d.SetDisabled("tenant-a", "1", true, "1"); !errors.Is(err, ErrSelfOperation) {
			t.Fatalf("self disable: %v", err)
		}
		if err := d.SetDisabled("tenant-a", "1", true, "actor-9"); err != nil {
			t.Fatal(err)
		}
		if len(fc.updates) != 1 || !fc.updates[0].user.IsForbidden || fc.updates[0].columns[0] != "is_forbidden" {
			t.Fatalf("disable call: %+v cols %v", fc.updates[0].user, fc.updates[0].columns)
		}
		// 禁用状态不落本地表，role/status 保持
		if rec := fs.rec("1"); rec.Role != authdom.RoleMember || rec.Status != authdom.StatusActive {
			t.Fatalf("本地记录不应被禁用操作改动: %+v", rec)
		}
	})

	t.Run("pending 用户也可禁用（无额外防护）", func(t *testing.T) {
		fs := newFakeStore()
		fs.seed("3", "tenant-a", "carol", "", authdom.StatusPending)
		fc := &fakeClient{users: []*casdoorsdk.User{
			{Id: "3", Name: "carol", Owner: "tenant-a"},
		}}
		d := NewCasdoorDirectory(fc, fs)

		if err := d.SetDisabled("tenant-a", "3", true, "actor-9"); err != nil {
			t.Fatalf("pending 用户禁用不应报错: %v", err)
		}
		if rec := fs.rec("3"); rec.Status != authdom.StatusPending {
			t.Fatalf("本地状态不应变化: %+v", rec)
		}
	})

	t.Run("本地无记录返回 ErrUserNotFound", func(t *testing.T) {
		fs := newFakeStore()
		fc := &fakeClient{users: []*casdoorsdk.User{
			{Id: "ghost", Name: "ghost", Owner: "tenant-a"},
		}}
		d := NewCasdoorDirectory(fc, fs)

		if err := d.SetDisabled("tenant-a", "ghost", true, "actor-9"); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("got %v, want ErrUserNotFound", err)
		}
		if len(fc.updates) != 0 || fc.getByIDCalls != 0 {
			t.Fatalf("无本地记录时不应触达 casdoor: updates=%d getByID=%d", len(fc.updates), fc.getByIDCalls)
		}
	})
}

func TestResetPasswordPassthrough(t *testing.T) {
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
	fc := &fakeClient{users: []*casdoorsdk.User{
		{Id: "1", Name: "alice", Owner: "tenant-a"},
	}}
	d := NewCasdoorDirectory(fc, fs)

	if _, err := d.ResetPassword("tenant-a", "1", "1"); !errors.Is(err, ErrSelfOperation) {
		t.Fatalf("self reset: %v", err)
	}
	plain, err := d.ResetPassword("tenant-a", "1", "actor-9")
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) < 16 {
		t.Fatalf("password too short: %q", plain)
	}
	last := fc.updates[len(fc.updates)-1]
	if last.user.Password != plain || last.columns[0] != "password" {
		t.Fatalf("reset call cols %v", last.columns)
	}
	// 二次重置产生不同密码
	plain2, _ := d.ResetPassword("tenant-a", "1", "actor-9")
	if plain == plain2 {
		t.Fatal("passwords must be random")
	}
}

// ===================== 错误透传 =====================

func TestSDKErrorsPropagate(t *testing.T) {
	// 瞬时 SDK 错误必须原样透传（handler 映射 502），不吞成 404
	sentinel := errors.New("casdoor unreachable")

	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
	fc := &fakeClient{getByIDErr: sentinel}
	d := NewCasdoorDirectory(fc, fs)

	// UpdateRole 走 admin 分支（目标 admin）时经 GetUserByUserId
	if err := d.UpdateRole("tenant-a", "1", authdom.RoleAdmin, "actor-9"); !errors.Is(err, sentinel) {
		t.Fatalf("UpdateRole: got %v, want sentinel", err)
	}
	if len(fs.setCalls) != 0 {
		t.Fatalf("SDK 错误后本地必须零写入, got %d", len(fs.setCalls))
	}

	// SetDisabled / ResetPassword 直通路径同样透传
	if err := d.SetDisabled("tenant-a", "1", true, "actor-9"); !errors.Is(err, sentinel) {
		t.Fatalf("SetDisabled: got %v, want sentinel", err)
	}
	if _, err := d.ResetPassword("tenant-a", "1", "actor-9"); !errors.Is(err, sentinel) {
		t.Fatalf("ResetPassword: got %v, want sentinel", err)
	}
}

func TestWriteOpsUpdateRejected(t *testing.T) {
	// casdoor 返回 ok=false 无错误 → ErrUpdateRejected（handler 502）
	fs := newFakeStore()
	fs.seed("1", "tenant-a", "alice", authdom.RoleMember, authdom.StatusActive)
	fc := &fakeClient{
		users:          []*casdoorsdk.User{{Id: "1", Name: "alice", Owner: "tenant-a"}},
		updateRejected: true,
	}
	d := NewCasdoorDirectory(fc, fs)

	if err := d.UpdateRole("tenant-a", "1", authdom.RoleAdmin, "actor-9"); !errors.Is(err, ErrUpdateRejected) {
		t.Fatalf("UpdateRole: got %v, want ErrUpdateRejected", err)
	}
	if len(fs.setCalls) != 0 {
		t.Fatalf("被拒后本地必须零写入, got %d", len(fs.setCalls))
	}
	if err := d.SetDisabled("tenant-a", "1", true, "actor-9"); !errors.Is(err, ErrUpdateRejected) {
		t.Fatalf("SetDisabled: got %v, want ErrUpdateRejected", err)
	}
	if _, err := d.ResetPassword("tenant-a", "1", "actor-9"); !errors.Is(err, ErrUpdateRejected) {
		t.Fatalf("ResetPassword: got %v, want ErrUpdateRejected", err)
	}
}
