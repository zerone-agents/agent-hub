package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// newAdminTestEnv wires an admin router with a stubbed actor (user_id=1,
// matching the first-created admin). Tests cover List/Update/Reset/Create
// invite/List invite/Revoke invite.
func newAdminTestEnv(t *testing.T) (*gin.Engine, *services.UserService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&authdom.User{}, &authdom.Invite{}, &authdom.RefreshToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := services.NewUserService(db)
	invites := services.NewInviteService(db)
	p := builtin.New(db, builtinTestSecret)
	h := NewAdminUserHandler(users, invites, p)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// The first user created (admin via CreateInitialAdmin) gets id=1.
	withActor := func(c *gin.Context) {
		c.Set("user_id", "1")
		c.Next()
	}
	g := r.Group("/api/v1/admin", withActor)
	g.GET("/users", h.ListUsers)
	g.PATCH("/users/:id", h.UpdateUser)
	g.POST("/users/:id/reset-password", h.ResetUserPassword)
	g.POST("/invites", h.CreateInvite)
	g.GET("/invites", h.ListInvites)
	g.DELETE("/invites/:id", h.RevokeInvite)
	return r, users
}

func TestAdminUserOps(t *testing.T) {
	r, users := newAdminTestEnv(t)
	admin, _ := users.CreateInitialAdmin("Passw0rd!")
	member, _ := users.Create("member1", "abcd1234", "", authdom.RoleMember)

	// list — two users, no passwordHash leaked
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list code=%d", w.Code)
	}
	var env struct {
		Success bool `json:"success"`
		Data    []struct {
			Username     string `json:"username"`
			Role         string `json:"role"`
			PasswordHash string `json:"passwordHash"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &env)
	if len(env.Data) != 2 {
		t.Fatalf("users = %d", len(env.Data))
	}
	for _, u := range env.Data {
		if u.PasswordHash != "" {
			t.Fatal("passwordHash must never be serialized")
		}
	}

	// promote member → maintainer
	patch := func(id uint64, body map[string]string) *httptest.ResponseRecorder {
		buf, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%d", id), bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := patch(member.ID, map[string]string{"role": "maintainer"}); w.Code != 200 {
		t.Fatalf("update role code=%d body=%s", w.Code, w.Body.String())
	}
	updated, _ := users.GetByID(member.ID)
	if updated.Role != authdom.RoleMaintainer {
		t.Fatalf("role = %s", updated.Role)
	}

	// disable member
	if w := patch(member.ID, map[string]string{"status": "disabled"}); w.Code != 200 {
		t.Fatalf("disable code=%d body=%s", w.Code, w.Body.String())
	}

	// self-disable → 400
	if w := patch(admin.ID, map[string]string{"status": "disabled"}); w.Code != http.StatusBadRequest {
		t.Fatalf("self-disable code=%d, want 400", w.Code)
	}

	// reset password returns plaintext once
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%d/reset-password", member.ID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	data := parseData(t, w)
	pw, ok := data["password"].(string)
	if !ok || len(pw) < 12 {
		t.Fatalf("reset resp: %v", data)
	}
}

func TestAdminInviteOps(t *testing.T) {
	r, _ := newAdminTestEnv(t)

	// create invite
	body, _ := json.Marshal(map[string]any{"role": "member", "note": "测试", "expiresInDays": 3})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create invite code=%d body=%s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["token"] == nil {
		t.Fatal("create must return plaintext token once")
	}

	// list — no plaintext token exposed
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/invites", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env struct {
		Data []struct {
			ID     uint64 `json:"id"`
			Status string `json:"status"`
			Token  string `json:"token"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &env)
	if len(env.Data) != 1 || env.Data[0].Status != "pending" {
		t.Fatalf("invites: %+v", env.Data)
	}
	if env.Data[0].Token != "" {
		t.Fatal("list must not expose token")
	}

	// revoke
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/admin/invites/%d", env.Data[0].ID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("revoke code=%d", w.Code)
	}
	// revoke again → 400
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/admin/invites/%d", env.Data[0].ID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("double revoke code=%d", w.Code)
	}
}

func TestAdminRejectsBadID(t *testing.T) {
	r, _ := newAdminTestEnv(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/abc", bytes.NewReader([]byte(`{"role":"member"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad id code=%d", w.Code)
	}
}

// keep strconv referenced even if a future refactor removes the only use
var _ = strconv.Itoa
