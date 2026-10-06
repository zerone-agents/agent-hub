package handler

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth/builtin"
	"control-panel/internal/auth/jwtutil"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestKnowledgeImageRoutingWithRealAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&authdom.User{}, &authdom.RefreshToken{}))
	provider := builtin.New(db, builtinTestSecret)
	users := services.NewUserService(db)
	tokens := map[string]string{}
	for _, role := range []string{"admin", "maintainer", "member", "guest"} {
		user, createErr := users.Create("image_"+role, "Passw0rd!", role, role)
		require.NoError(t, createErr)
		pair, issueErr := provider.IssueTokenPair(user)
		require.NoError(t, issueErr)
		tokens[role] = pair.AccessToken
	}
	disabled, err := users.Create("image_disabled", "Passw0rd!", "disabled", "member")
	require.NoError(t, err)
	pair, err := provider.IssueTokenPair(disabled)
	require.NoError(t, err)
	tokens["disabled"] = pair.AccessToken
	require.NoError(t, db.Model(disabled).Update("status", authdom.StatusDisabled).Error)
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}).SignedString([]byte(builtinTestSecret))
	require.NoError(t, err)
	tokens["expired"] = expired
	var binary bytes.Buffer
	require.NoError(t, png.Encode(&binary, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	var received []string
	engine := &handlerFakeKnowledgeEngine{imageFunc: func(_ context.Context, id string) (*knowledge.StreamResult, error) {
		received = append(received, id)
		return &knowledge.StreamResult{Body: io.NopCloser(bytes.NewReader(binary.Bytes())), ContentType: "image/png", ContentLength: int64(binary.Len())}, nil
	}}
	r := gin.New()
	v1 := r.Group("/api/v1", KnowledgeImageHeaders, middleware.JWTAuthWithCLI(nil, provider), jwtutil.GuestGuard())
	read := v1.Group("/admin", middleware.RequireRole("admin", "maintainer", "member"))
	write := v1.Group("/admin", middleware.RequireManager())
	RegisterKnowledgeRoutes(write, read, NewKnowledgeHandler(services.NewKnowledgeService(engine, nil), nil))
	key := "kb1-key-with-more-hyphens/part name%中文.png"
	for _, role := range []string{"admin", "maintainer", "member"} {
		t.Run(role, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/images/"+url.PathEscape(key), nil)
			req.Header.Set("Authorization", "Bearer "+tokens[role])
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, binary.Bytes(), w.Body.Bytes())
			require.Equal(t, key, received[len(received)-1])
			require.Equal(t, "image/png", w.Header().Get("Content-Type"))
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		})
	}
	// Literal percent-encoded text is part of the key, never decoded twice.
	literal := "kb1-literal%2F%25/中文.png"
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/images/"+url.PathEscape(literal), nil)
	req.Header.Set("Authorization", "Bearer "+tokens["admin"])
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, literal, received[len(received)-1])
	cases := []struct {
		name, token, imageID string
		status               int
	}{
		{"missing-auth", "", key, 401}, {"invalid-auth", "untrusted.invalid.jwt", key, 401}, {"expired-auth", tokens["expired"], key, 401}, {"disabled", tokens["disabled"], key, 401}, {"guest", tokens["guest"], key, 403},
		{"wrong-dataset", tokens["admin"], "kb2-key/path.png", 400}, {"approximate-prefix", tokens["admin"], "kb11-key.png", 400}, {"missing-key", tokens["admin"], "kb1-", 400}, {"missing-id", tokens["admin"], "", 400}, {"no-hyphen", tokens["admin"], "kb1", 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count := len(received)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/images/"+url.PathEscape(tc.imageID), nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, count, len(received))
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			require.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "application/json"))
			require.Empty(t, w.Header().Get("Content-Disposition"))
		})
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/health", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, w.Header().Get("Cache-Control"))
	require.Empty(t, w.Header().Get("X-Content-Type-Options"))
}
