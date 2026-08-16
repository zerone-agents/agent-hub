package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/directory"

	"github.com/gin-gonic/gin"
)

// fakeUserDirectory implements UserDirectory with scripted results and
// captures the arguments of the last call for assertion.
type fakeUserDirectory struct {
	users   []directory.ManagedUser
	listErr error

	updateRoleErr  error
	setDisabledErr error
	resetPassword  string
	resetErr       error

	gotTenant, gotUser, gotActor string
	gotRole                      string
	gotDisabled                  bool
}

func (f *fakeUserDirectory) ListUsers(tenantID string) ([]directory.ManagedUser, error) {
	f.gotTenant = tenantID
	return f.users, f.listErr
}

func (f *fakeUserDirectory) UpdateRole(tenantID, userID, role, actorID string) error {
	f.gotTenant, f.gotUser, f.gotRole, f.gotActor = tenantID, userID, role, actorID
	return f.updateRoleErr
}

func (f *fakeUserDirectory) SetDisabled(tenantID, userID string, disabled bool, actorID string) error {
	f.gotTenant, f.gotUser, f.gotDisabled, f.gotActor = tenantID, userID, disabled, actorID
	return f.setDisabledErr
}

func (f *fakeUserDirectory) ResetPassword(tenantID, userID, actorID string) (string, error) {
	f.gotTenant, f.gotUser, f.gotActor = tenantID, userID, actorID
	return f.resetPassword, f.resetErr
}

const fakeSignupURL = "https://casdoor.example.com/signup/middle-ground"

// setupCasdoorUserRouter wires the casdoor admin user routes with a fake
// directory. The middleware injects the same context the auth middleware
// would set (tenant_id / user_id / roles); RequireAdmin is intentionally not
// mounted here — it is applied at route registration in main.go.
func setupCasdoorUserRouter(dir UserDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-a")
		c.Set("user_id", "actor")
		c.Set("roles", []string{"admin"})
	})
	h := NewCasdoorUserHandler(dir, fakeSignupURL)
	r.GET("/admin/users/signup-url", h.SignupURL)
	r.GET("/admin/users", h.ListUsers)
	r.PATCH("/admin/users/:id", h.UpdateUser)
	r.POST("/admin/users/:id/reset-password", h.ResetUserPassword)
	return r
}

func casdoorDo(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCasdoorListUsersOK(t *testing.T) {
	dir := &fakeUserDirectory{
		users: []directory.ManagedUser{{ID: "u1", Username: "alice", Role: "member", Status: "active"}},
	}
	r := setupCasdoorUserRouter(dir)

	w := casdoorDo(r, "GET", "/admin/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if dir.gotTenant != "tenant-a" {
		t.Fatalf("ListUsers tenant: got %q", dir.gotTenant)
	}
	var env struct {
		Success bool                    `json:"success"`
		Data    []directory.ManagedUser `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Success || len(env.Data) != 1 || env.Data[0].ID != "u1" {
		t.Fatalf("unexpected body: %+v", env)
	}
}

func TestCasdoorListUsersSDKFailure502(t *testing.T) {
	dir := &fakeUserDirectory{listErr: errors.New("sdk unreachable")}
	r := setupCasdoorUserRouter(dir)

	w := casdoorDo(r, "GET", "/admin/users", nil)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCasdoorUpdateUserRoleAndStatus(t *testing.T) {
	dir := &fakeUserDirectory{}
	r := setupCasdoorUserRouter(dir)

	// role update: directory receives tenant, user id, role, actor.
	w := casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"role": "maintainer"})
	if w.Code != http.StatusOK {
		t.Fatalf("role update: %d %s", w.Code, w.Body.String())
	}
	if dir.gotTenant != "tenant-a" || dir.gotUser != "u1" || dir.gotRole != "maintainer" || dir.gotActor != "actor" {
		t.Fatalf("UpdateRole args: tenant=%q user=%q role=%q actor=%q", dir.gotTenant, dir.gotUser, dir.gotRole, dir.gotActor)
	}

	// status update: disabled -> SetDisabled(disabled=true).
	w = casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"status": "disabled"})
	if w.Code != http.StatusOK {
		t.Fatalf("status update: %d %s", w.Code, w.Body.String())
	}
	if dir.gotUser != "u1" || !dir.gotDisabled {
		t.Fatalf("SetDisabled args: user=%q disabled=%v", dir.gotUser, dir.gotDisabled)
	}

	// empty payload -> 400.
	w = casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty payload: %d", w.Code)
	}

	// sentinel error mapping: ErrSelfOperation -> 400, ErrUserNotFound -> 404.
	dir.updateRoleErr = directory.ErrSelfOperation
	w = casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"role": "maintainer"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ErrSelfOperation: %d", w.Code)
	}

	dir.updateRoleErr = directory.ErrUserNotFound
	w = casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"role": "maintainer"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("ErrUserNotFound: %d", w.Code)
	}
	dir.updateRoleErr = nil
}

func TestCasdoorResetPassword(t *testing.T) {
	dir := &fakeUserDirectory{resetPassword: "pw"}
	r := setupCasdoorUserRouter(dir)

	w := casdoorDo(r, "POST", "/admin/users/u1/reset-password", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["password"] != "pw" {
		t.Fatalf("password: got %v", data["password"])
	}
	if dir.gotTenant != "tenant-a" || dir.gotUser != "u1" || dir.gotActor != "actor" {
		t.Fatalf("ResetPassword args: tenant=%q user=%q actor=%q", dir.gotTenant, dir.gotUser, dir.gotActor)
	}

	dir.resetErr = directory.ErrSelfOperation
	w = casdoorDo(r, "POST", "/admin/users/u1/reset-password", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ErrSelfOperation: %d", w.Code)
	}
}

func TestCasdoorSignupURL(t *testing.T) {
	dir := &fakeUserDirectory{}
	r := setupCasdoorUserRouter(dir)

	w := casdoorDo(r, "GET", "/admin/users/signup-url", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("signup-url: %d %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["signupUrl"] != fakeSignupURL {
		t.Fatalf("signupUrl: got %v", data["signupUrl"])
	}
}
