package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/auth/builtin"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestFullAuthFlow wires the real middleware stack (JWT + RequireAdmin) and
// exercises the end-to-end builtin lifecycle:
//
//	setup → admin login → create invite → register → member blocked from
//	admin route → admin disables user → user's refresh + access both fail.
func TestFullAuthFlow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&authdom.User{}, &authdom.Invite{}, &authdom.RefreshToken{}, &authdom.CLIToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	p := builtin.New(db, builtinTestSecret)
	users := services.NewUserService(db)
	invites := services.NewInviteService(db)
	cliSvc := services.NewCLITokenService(db)
	authH := NewBuiltinAuthHandler(p, users, invites)
	adminH := NewAdminUserHandler(users, invites, p)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/auth/setup", authH.Setup)
	r.POST("/auth/login", authH.Login)
	r.POST("/auth/refresh", authH.Refresh)
	r.POST("/auth/register", authH.Register)
	authed := r.Group("/api", middleware.JWTAuthWithCLI(cliSvc, p))
	adm := authed.Group("/admin", middleware.RequireAdmin())
	adm.GET("/users", adminH.ListUsers)
	adm.POST("/invites", adminH.CreateInvite)
	adm.PATCH("/users/:id", adminH.UpdateUser)

	do := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// 1. setup
	w := do("POST", "/auth/setup", map[string]string{"password": "Passw0rd!", "confirmPassword": "Passw0rd!"}, "")
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	adminTokens := parseData(t, w)
	adminAccess := adminTokens["accessToken"].(string)

	// 2. admin creates an invite
	w = do("POST", "/api/admin/invites", map[string]any{"role": "member", "note": "flow"}, adminAccess)
	if w.Code != 200 {
		t.Fatalf("create invite: %d %s", w.Code, w.Body.String())
	}
	inviteToken := parseData(t, w)["token"].(string)

	// 3. invitee registers (auto-login)
	w = do("POST", "/auth/register", map[string]string{"inviteToken": inviteToken, "username": "flowuser", "password": "abcd1234"}, "")
	if w.Code != 200 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	userTokens := parseData(t, w)
	userAccess := userTokens["accessToken"].(string)
	userRefresh := userTokens["refreshToken"].(string)

	// 4. member hitting an admin route → 403
	w = do("GET", "/api/admin/users", nil, userAccess)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member on admin route: %d", w.Code)
	}

	// 5. admin disables the user
	var listEnv struct {
		Data []struct {
			ID       uint64 `json:"id"`
			Username string `json:"username"`
		} `json:"data"`
	}
	w = do("GET", "/api/admin/users", nil, adminAccess)
	json.Unmarshal(w.Body.Bytes(), &listEnv)
	var targetID uint64
	for _, u := range listEnv.Data {
		if u.Username == "flowuser" {
			targetID = u.ID
		}
	}
	if targetID == 0 {
		t.Fatal("flowuser not found in user list")
	}
	w = do("PATCH", fmt.Sprintf("/api/admin/users/%d", targetID), map[string]string{"status": "disabled"}, adminAccess)
	if w.Code != 200 {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}

	// 6. disabled user's refresh → 401 (sessions revoked)
	w = do("POST", "/auth/refresh", map[string]string{"refreshToken": userRefresh}, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user refresh: %d", w.Code)
	}

	// 7. disabled user's access token is immediately invalid (builtin re-reads
	//    user status on every validation — no 2h window).
	w = do("GET", "/api/admin/users", nil, userAccess)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("disabled user access: %d", w.Code)
	}
}
