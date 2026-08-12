package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/auth/builtin"
	authdom "control-panel/internal/domain/auth"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const builtinTestSecret = "test-secret-test-secret-test-secret!!"

func newBuiltinTestEnv(t *testing.T) (*gin.Engine, *services.UserService, *services.InviteService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&authdom.User{}, &authdom.Invite{}, &authdom.RefreshToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	p := builtin.New(db, builtinTestSecret)
	users := services.NewUserService(db)
	invites := services.NewInviteService(db)
	h := NewBuiltinAuthHandler(p, users, invites)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/mode", h.GetMode)
	r.POST("/auth/setup", h.Setup)
	r.POST("/auth/login", h.Login)
	r.POST("/auth/refresh", h.Refresh)
	r.POST("/auth/logout", h.Logout)
	r.POST("/auth/register", h.Register)
	r.GET("/auth/invite/:token", h.InvitePrecheck)
	r.POST("/auth/change-password", func(c *gin.Context) {
		c.Set("user_id", c.GetHeader("X-Test-User-ID"))
		c.Next()
	}, h.ChangePassword)
	return r, users, invites
}

func postJSON(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("parse: %v body=%s", err, w.Body.String())
	}
	if !env.Success {
		t.Fatalf("success=false body=%s", w.Body.String())
	}
	return env.Data
}

func TestSetupAndLoginFlow(t *testing.T) {
	r, _, _ := newBuiltinTestEnv(t)

	// mode: uninitialized
	req := httptest.NewRequest(http.MethodGet, "/auth/mode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	data := parseData(t, w)
	if data["mode"] != "builtin" || data["initialized"] != false {
		t.Fatalf("mode resp: %v", data)
	}

	// setup
	w = postJSON(t, r, "/auth/setup", map[string]string{"password": "Passw0rd!", "confirmPassword": "Passw0rd!"})
	if w.Code != 200 {
		t.Fatalf("setup code=%d body=%s", w.Code, w.Body.String())
	}
	setupData := parseData(t, w)
	if setupData["accessToken"] == "" || setupData["refreshToken"] == "" {
		t.Fatalf("setup must return tokens: %v", setupData)
	}

	// setup again → 409
	w = postJSON(t, r, "/auth/setup", map[string]string{"password": "Passw0rd!", "confirmPassword": "Passw0rd!"})
	if w.Code != http.StatusConflict {
		t.Fatalf("second setup code=%d", w.Code)
	}

	// login wrong password → 401
	w = postJSON(t, r, "/auth/login", map[string]string{"username": "admin", "password": "bad"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad login code=%d", w.Code)
	}

	// login ok
	w = postJSON(t, r, "/auth/login", map[string]string{"username": "admin", "password": "Passw0rd!"})
	if w.Code != 200 {
		t.Fatalf("login code=%d body=%s", w.Code, w.Body.String())
	}
	loginData := parseData(t, w)

	// refresh rotation
	w = postJSON(t, r, "/auth/refresh", map[string]string{"refreshToken": loginData["refreshToken"].(string)})
	if w.Code != 200 {
		t.Fatalf("refresh code=%d body=%s", w.Code, w.Body.String())
	}
	// old refresh reused → 401
	w = postJSON(t, r, "/auth/refresh", map[string]string{"refreshToken": loginData["refreshToken"].(string)})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh code=%d", w.Code)
	}
}

func TestRefreshAcceptsSnakeCase(t *testing.T) {
	r, _, _ := newBuiltinTestEnv(t)
	postJSON(t, r, "/auth/setup", map[string]string{"password": "Passw0rd!", "confirmPassword": "Passw0rd!"})
	w := postJSON(t, r, "/auth/login", map[string]string{"username": "admin", "password": "Passw0rd!"})
	rt := parseData(t, w)["refreshToken"].(string)
	// snake_case key must also work (frontend interceptor compat).
	w = postJSON(t, r, "/auth/refresh", map[string]string{"refresh_token": rt})
	if w.Code != 200 {
		t.Fatalf("snake_case refresh code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRegisterWithInvite(t *testing.T) {
	r, users, invites := newBuiltinTestEnv(t)
	admin, _ := users.CreateInitialAdmin("Passw0rd!")
	res, _ := invites.Create(authdom.RoleMember, "note-x", admin.ID, 0)

	// precheck
	req := httptest.NewRequest(http.MethodGet, "/auth/invite/"+res.Token, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	data := parseData(t, w)
	if data["valid"] != true || data["note"] != "note-x" {
		t.Fatalf("precheck: %v", data)
	}

	// register
	w = postJSON(t, r, "/auth/register", map[string]string{
		"inviteToken": res.Token, "username": "newbie", "password": "abcd1234", "displayName": "新人",
	})
	if w.Code != 200 {
		t.Fatalf("register code=%d body=%s", w.Code, w.Body.String())
	}
	regData := parseData(t, w)
	if regData["accessToken"] == "" {
		t.Fatal("register must auto-login")
	}

	// invite reused → 410
	w = postJSON(t, r, "/auth/register", map[string]string{
		"inviteToken": res.Token, "username": "other", "password": "abcd1234",
	})
	if w.Code != http.StatusGone {
		t.Fatalf("reused invite code=%d", w.Code)
	}

	// duplicate username with fresh invite → 409
	res2, _ := invites.Create(authdom.RoleMember, "", admin.ID, 0)
	w = postJSON(t, r, "/auth/register", map[string]string{
		"inviteToken": res2.Token, "username": "newbie", "password": "abcd1234",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("dup username code=%d", w.Code)
	}
}

func TestInvitePrecheckInvalid(t *testing.T) {
	r, _, _ := newBuiltinTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/invite/inv_bogus", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("invalid invite precheck code=%d", w.Code)
	}
}

func TestChangePasswordRevokesSessions(t *testing.T) {
	r, users, _ := newBuiltinTestEnv(t)
	u, _ := users.CreateInitialAdmin("Passw0rd!")
	w := postJSON(t, r, "/auth/login", map[string]string{"username": "admin", "password": "Passw0rd!"})
	oldTokens := parseData(t, w)

	req := httptest.NewRequest(http.MethodPost, "/auth/change-password",
		bytes.NewReader([]byte(`{"oldPassword":"Passw0rd!","newPassword":"Newpass123"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-ID", strconv.FormatUint(u.ID, 10))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("change pwd code=%d body=%s", w.Code, w.Body.String())
	}
	newTokens := parseData(t, w)
	if newTokens["accessToken"] == "" {
		t.Fatal("change password must return fresh tokens")
	}
	// old refresh dead
	w = postJSON(t, r, "/auth/refresh", map[string]string{"refreshToken": oldTokens["refreshToken"].(string)})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("old refresh after pwd change code=%d", w.Code)
	}
	// new password logs in
	w = postJSON(t, r, "/auth/login", map[string]string{"username": "admin", "password": "Newpass123"})
	if w.Code != 200 {
		t.Fatalf("login with new pwd code=%d", w.Code)
	}
}
