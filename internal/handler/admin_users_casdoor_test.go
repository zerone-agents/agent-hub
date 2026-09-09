package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

	updateRoleCalls int
}

func (f *fakeUserDirectory) ListUsers(tenantID string) ([]directory.ManagedUser, error) {
	f.gotTenant = tenantID
	return f.users, f.listErr
}

func (f *fakeUserDirectory) UpdateRole(tenantID, userID, role, actorID string) error {
	f.updateRoleCalls++
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

const fakeCasdoorEndpoint = "https://casdoor.example.com"

// trivialLoginURLFn 供不关心链接内容的测试使用：仅要求生成器被调用即可。
var trivialLoginURLFn = func(org string) (string, error) {
	return fakeCasdoorEndpoint + "/login/oauth/authorize", nil
}

// fakeLoginURLBuilder 注入 LoginURLBuilder：返回组织名拼成的固定授权链接，
// 同时记录收到的 org 供断言。
func fakeLoginURLBuilder(t *testing.T) (LoginURLBuilder, *[]string) {
	t.Helper()
	var gotOrgs []string
	return func(org string) (string, error) {
		gotOrgs = append(gotOrgs, org)
		return fakeCasdoorEndpoint + "/login/oauth/authorize?client_id=fake-id&response_type=code&scope=read", nil
	}, &gotOrgs
}

// setupCasdoorUserRouter wires the casdoor admin user routes with a fake
// directory. The middleware injects the same context the auth middleware
// would set (tenant_id / user_id / roles); RequireAdmin is intentionally not
// mounted here — it is applied at route registration in main.go.
func setupCasdoorUserRouter(dir UserDirectory, loginURLFn LoginURLBuilder) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", "tenant-a")
		c.Set("user_id", "actor")
		c.Set("roles", []string{"admin"})
	})
	h := NewCasdoorUserHandler(dir, loginURLFn)
	r.GET("/admin/users/login-url", h.LoginURL)
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
	r := setupCasdoorUserRouter(dir, trivialLoginURLFn)

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
	r := setupCasdoorUserRouter(dir, trivialLoginURLFn)

	w := casdoorDo(r, "GET", "/admin/users", nil)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCasdoorUpdateUserRoleAndStatus(t *testing.T) {
	dir := &fakeUserDirectory{}
	r := setupCasdoorUserRouter(dir, trivialLoginURLFn)

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

func TestCasdoorUpdateUserInvalidStatus(t *testing.T) {
	dir := &fakeUserDirectory{}
	r := setupCasdoorUserRouter(dir, trivialLoginURLFn)

	// Invalid status alone -> 400.
	w := casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"status": "banned"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid status: %d %s", w.Code, w.Body.String())
	}

	// Valid role + invalid status -> 400, and UpdateRole must NOT be called
	// (status is validated before any change is applied).
	w = casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"role": "maintainer", "status": "banned"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("valid role + invalid status: %d %s", w.Code, w.Body.String())
	}
	if dir.updateRoleCalls != 0 {
		t.Fatalf("UpdateRole called %d times, want 0 on invalid status", dir.updateRoleCalls)
	}
}

func TestCasdoorResetPassword(t *testing.T) {
	dir := &fakeUserDirectory{resetPassword: "pw"}
	r := setupCasdoorUserRouter(dir, trivialLoginURLFn)

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

func TestCasdoorLoginURL(t *testing.T) {
	dir := &fakeUserDirectory{}
	loginURLFn, gotOrgs := fakeLoginURLBuilder(t)
	r := setupCasdoorUserRouter(dir, loginURLFn)

	w := casdoorDo(r, "GET", "/admin/users/login-url", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login-url: %d %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	// 组织按请求租户传入生成器（tenant-a 由 setupCasdoorUserRouter 的
	// middleware 注入），返回的是 OAuth 授权链接而非静态注册页。
	want := fakeCasdoorEndpoint + "/login/oauth/authorize?client_id=fake-id&response_type=code&scope=read"
	if data["loginUrl"] != want {
		t.Fatalf("loginUrl: got %v, want %v", data["loginUrl"], want)
	}
	if len(*gotOrgs) != 1 || (*gotOrgs)[0] != "tenant-a" {
		t.Fatalf("builder orgs: got %v, want [tenant-a]", *gotOrgs)
	}
}

func TestCasdoorLoginURLBuilderFailure(t *testing.T) {
	dir := &fakeUserDirectory{}
	loginURLFn := func(org string) (string, error) {
		return "", errors.New("组织未注册")
	}
	r := setupCasdoorUserRouter(dir, loginURLFn)

	w := casdoorDo(r, "GET", "/admin/users/login-url", nil)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("login-url failure: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "生成登录链接失败") {
		t.Fatalf("failure body missing message: %s", w.Body.String())
	}
}
