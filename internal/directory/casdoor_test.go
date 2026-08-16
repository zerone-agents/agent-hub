package directory

import (
	"errors"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type fakeClient struct {
	users  []*casdoorsdk.User
	getErr error
}

func (f *fakeClient) GetUsers() ([]*casdoorsdk.User, error) { return f.users, f.getErr }
func (f *fakeClient) GetUserByUserId(id string) (*casdoorsdk.User, error) {
	for _, u := range f.users {
		if u.Id == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakeClient) UpdateUserForColumns(u *casdoorsdk.User, cols []string) (bool, error) {
	return true, nil
}

var testMapping = map[string]string{
	"admin":      "agent-hub-admin",
	"maintainer": "agent-hub-maintainer",
	"member":     "agent-hub-member",
}

func TestListUsersFiltersByTenantAndNormalizesRoles(t *testing.T) {
	fc := &fakeClient{users: []*casdoorsdk.User{
		{Id: "1", Name: "alice", DisplayName: "Alice", Email: "a@x.com", Owner: "tenant-a",
			CreatedTime: "2026-01-01T00:00:00+08:00",
			Roles:       []*casdoorsdk.Role{{Name: "agent-hub-admin"}}},
		{Id: "2", Name: "bob", Owner: "tenant-a",
			Roles: []*casdoorsdk.Role{{Name: "agent-hub-member"}, {Name: "other-org-role"}}},
		{Id: "3", Name: "carol", Owner: "tenant-a", IsForbidden: true,
			Roles: []*casdoorsdk.Role{{Name: "unmapped-role"}}}, // no mapped role -> Role ""
		{Id: "4", Name: "dave", Owner: "tenant-b", // other tenant -> filtered out
			Roles: []*casdoorsdk.Role{{Name: "agent-hub-admin"}}},
	}}
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
}
