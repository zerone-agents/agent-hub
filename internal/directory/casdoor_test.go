package directory

import (
	"errors"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type updateCall struct {
	user    *casdoorsdk.User
	columns []string
}

type roleUpdateCall struct {
	role    *casdoorsdk.Role
	columns []string
}

type fakeClient struct {
	users       []*casdoorsdk.User
	roles       []*casdoorsdk.Role
	getErr      error
	getRolesErr error
	getByIDErr  error // injected error for GetUserByUserId (takes precedence)
	// updateRejected makes UpdateUserForColumns/UpdateRoleForColumns return
	// (false, nil), simulating casdoor rejecting the update without an error.
	updateRejected bool
	updates        []updateCall
	roleUpdates    []roleUpdateCall
}

func (f *fakeClient) GetUsers() ([]*casdoorsdk.User, error) { return f.users, f.getErr }
func (f *fakeClient) GetUserByUserId(id string) (*casdoorsdk.User, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	for _, u := range f.users {
		if u.Id == id {
			return u, nil
		}
	}
	return nil, nil // casdoor SDK returns (nil, nil) for an unknown user
}
func (f *fakeClient) UpdateUserForColumns(u *casdoorsdk.User, cols []string) (bool, error) {
	f.updates = append(f.updates, updateCall{user: u, columns: cols})
	if f.updateRejected {
		return false, nil
	}
	return true, nil
}
func (f *fakeClient) GetRoles() ([]*casdoorsdk.Role, error) { return f.roles, f.getRolesErr }
func (f *fakeClient) UpdateRoleForColumns(r *casdoorsdk.Role, cols []string) (bool, error) {
	f.roleUpdates = append(f.roleUpdates, roleUpdateCall{role: r, columns: cols})
	if f.updateRejected {
		return false, nil
	}
	return true, nil
}

var testMapping = map[string]string{
	"admin":      "agent-hub-admin",
	"maintainer": "agent-hub-maintainer",
	"member":     "agent-hub-member",
}

func TestListUsersFiltersByTenantAndNormalizesRoles(t *testing.T) {
	// get-users returns users WITHOUT roles (casdoor/casdoor#3688); role
	// membership comes from the Role objects' Users lists ("owner/name").
	fc := &fakeClient{
		users: []*casdoorsdk.User{
			{Id: "1", Name: "alice", DisplayName: "Alice", Email: "a@x.com", Owner: "tenant-a",
				CreatedTime: "2026-01-01T00:00:00+08:00"},
			{Id: "2", Name: "bob", Owner: "tenant-a"},
			{Id: "3", Name: "carol", Owner: "tenant-a", IsForbidden: true},
			{Id: "4", Name: "dave", Owner: "tenant-b"}, // other tenant -> filtered out
		},
		roles: []*casdoorsdk.Role{
			{Owner: "tenant-a", Name: "agent-hub-admin", Users: []string{"tenant-a/alice", "tenant-b/dave"}},
			{Owner: "tenant-a", Name: "agent-hub-member", Users: []string{"tenant-a/bob"}},
			{Owner: "tenant-a", Name: "other-org-role", Users: []string{"tenant-a/bob"}},
			{Owner: "tenant-a", Name: "unmapped-role", Users: []string{"tenant-a/carol"}},
		},
	}
	d := NewCasdoorDirectory(fc, testMapping, "")
	got, err := d.ListUsers("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d users, want 3 (tenant-b filtered)", len(got))
	}
	if got[0].Role != "admin" || got[0].Status != "active" {
		t.Fatalf("alice: %+v", got[0])
	}
	if got[1].Role != "member" {
		t.Fatalf("bob role: %q", got[1].Role)
	}
	if got[2].Role != "" || got[2].Status != "disabled" {
		t.Fatalf("carol: %+v", got[2])
	}
}

func TestListUsersSDKError(t *testing.T) {
	d := NewCasdoorDirectory(&fakeClient{getErr: errors.New("boom")}, testMapping, "")
	if _, err := d.ListUsers("tenant-a"); err == nil {
		t.Fatal("want error")
	}
	d = NewCasdoorDirectory(&fakeClient{
		users:       []*casdoorsdk.User{{Id: "1", Owner: "tenant-a"}},
		getRolesErr: errors.New("roles boom"),
	}, testMapping, "")
	if _, err := d.ListUsers("tenant-a"); err == nil {
		t.Fatal("want error from GetRoles")
	}
}

func TestUpdateRoleSwapsOnlyMappedRoles(t *testing.T) {
	fc := &fakeClient{
		users: []*casdoorsdk.User{
			{Id: "1", Name: "alice", Owner: "tenant-a"},
		},
		roles: []*casdoorsdk.Role{
			{Owner: "tenant-a", Name: "agent-hub-member", Users: []string{"tenant-a/alice", "tenant-a/other"}}, // mapped -> alice removed
			{Owner: "tenant-a", Name: "agent-hub-admin", Users: []string{"tenant-a/other"}},                    // mapped -> alice added
			{Owner: "tenant-a", Name: "external-billing-admin", Users: []string{"tenant-a/alice"}},             // not mapped -> untouched
		},
	}
	d := NewCasdoorDirectory(fc, testMapping, "")
	if err := d.UpdateRole("tenant-a", "1", "admin", "actor-9"); err != nil {
		t.Fatal(err)
	}
	if len(fc.roleUpdates) != 2 {
		t.Fatalf("role updates = %d, want 2 (member+admin)", len(fc.roleUpdates))
	}
	if len(fc.updates) != 0 {
		t.Fatalf("update-user must not be used for roles, got %d calls", len(fc.updates))
	}
	for _, call := range fc.roleUpdates {
		if len(call.columns) != 1 || call.columns[0] != "users" {
			t.Fatalf("columns: %v", call.columns)
		}
		switch call.role.Name {
		case "agent-hub-member":
			for _, uid := range call.role.Users {
				if uid == "tenant-a/alice" {
					t.Fatalf("alice must be removed from agent-hub-member: %v", call.role.Users)
				}
			}
		case "agent-hub-admin":
			found := false
			for _, uid := range call.role.Users {
				if uid == "tenant-a/alice" {
					found = true
				}
			}
			if !found {
				t.Fatalf("alice must be added to agent-hub-admin: %v", call.role.Users)
			}
		default:
			t.Fatalf("unexpected role touched: %s", call.role.Name)
		}
	}
	// external role untouched
	if fc.roles[2].Users[0] != "tenant-a/alice" {
		t.Fatalf("external role must stay untouched: %v", fc.roles[2].Users)
	}
}

func TestUpdateRoleRejectsSelfAndInvalid(t *testing.T) {
	fc := &fakeClient{users: []*casdoorsdk.User{
		{Id: "1", Name: "alice", Owner: "tenant-a", Roles: nil},
	}}
	d := NewCasdoorDirectory(fc, testMapping, "")
	if err := d.UpdateRole("tenant-a", "1", "admin", "1"); !errors.Is(err, ErrSelfOperation) {
		t.Fatalf("self: %v", err)
	}
	if err := d.UpdateRole("tenant-a", "1", "superuser", "actor"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("invalid: %v", err)
	}
	if err := d.UpdateRole("tenant-a", "missing", "admin", "actor"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := d.UpdateRole("tenant-b", "1", "admin", "actor"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("cross-tenant: %v", err)
	}
}

func TestGetTenantUserSDKErrorPropagates(t *testing.T) {
	sentinel := errors.New("casdoor unreachable")
	fc := &fakeClient{
		users:      []*casdoorsdk.User{{Id: "1", Name: "alice", Owner: "tenant-a"}},
		getByIDErr: sentinel,
	}
	d := NewCasdoorDirectory(fc, testMapping, "")

	// Transient SDK errors must surface as-is (handler maps them to 502),
	// not be collapsed into ErrUserNotFound (404).
	if err := d.UpdateRole("tenant-a", "1", "admin", "actor"); !errors.Is(err, sentinel) {
		t.Fatalf("UpdateRole: got %v, want sentinel", err)
	}
	if err := d.SetDisabled("tenant-a", "1", true, "actor"); !errors.Is(err, sentinel) {
		t.Fatalf("SetDisabled: got %v, want sentinel", err)
	}
	if _, err := d.ResetPassword("tenant-a", "1", "actor"); !errors.Is(err, sentinel) {
		t.Fatalf("ResetPassword: got %v, want sentinel", err)
	}
}

func TestWriteOpsUpdateRejected(t *testing.T) {
	// casdoor answering ok=false without an error must surface ErrUpdateRejected
	// (handler maps it to 502 via the default branch).
	fc := &fakeClient{
		users: []*casdoorsdk.User{{Id: "1", Name: "alice", Owner: "tenant-a"}},
		roles: []*casdoorsdk.Role{
			{Owner: "tenant-a", Name: "agent-hub-admin", Users: []string{}},
		},
		updateRejected: true,
	}
	d := NewCasdoorDirectory(fc, testMapping, "")

	if err := d.UpdateRole("tenant-a", "1", "admin", "actor"); !errors.Is(err, ErrUpdateRejected) {
		t.Fatalf("UpdateRole: got %v, want ErrUpdateRejected", err)
	}
	if err := d.SetDisabled("tenant-a", "1", true, "actor"); !errors.Is(err, ErrUpdateRejected) {
		t.Fatalf("SetDisabled: got %v, want ErrUpdateRejected", err)
	}
	if _, err := d.ResetPassword("tenant-a", "1", "actor"); !errors.Is(err, ErrUpdateRejected) {
		t.Fatalf("ResetPassword: got %v, want ErrUpdateRejected", err)
	}
}

func TestSetDisabledAndResetPassword(t *testing.T) {
	fc := &fakeClient{users: []*casdoorsdk.User{
		{Id: "1", Name: "alice", Owner: "tenant-a"},
	}}
	d := NewCasdoorDirectory(fc, testMapping, "")

	if err := d.SetDisabled("tenant-a", "1", true, "1"); !errors.Is(err, ErrSelfOperation) {
		t.Fatalf("self disable: %v", err)
	}
	if err := d.SetDisabled("tenant-a", "1", true, "actor"); err != nil {
		t.Fatal(err)
	}
	if !fc.updates[0].user.IsForbidden || fc.updates[0].columns[0] != "is_forbidden" {
		t.Fatalf("disable call: %+v cols %v", fc.updates[0].user, fc.updates[0].columns)
	}

	if _, err := d.ResetPassword("tenant-a", "1", "1"); !errors.Is(err, ErrSelfOperation) {
		t.Fatalf("self reset: %v", err)
	}
	plain, err := d.ResetPassword("tenant-a", "1", "actor")
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
	// second reset produces a different password
	plain2, _ := d.ResetPassword("tenant-a", "1", "actor")
	if plain == plain2 {
		t.Fatal("passwords must be random")
	}
}
