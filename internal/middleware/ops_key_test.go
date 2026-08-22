package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupOpsKeyRouter(key string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequireOpsKey(key))
	r.GET("/ops/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestRequireOpsKey_EmptyKeyReturns404(t *testing.T) {
	r := setupOpsKeyRouter("")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/ping", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRequireOpsKey_MissingOrWrongKeyReturns401(t *testing.T) {
	r := setupOpsKeyRouter("secret-ops-key")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/ping", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)

	req := httptest.NewRequest(http.MethodGet, "/ops/ping", nil)
	req.Header.Set("X-Ops-Key", "wrong")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireOpsKey_CorrectKeyPasses(t *testing.T) {
	r := setupOpsKeyRouter("secret-ops-key")

	req := httptest.NewRequest(http.MethodGet, "/ops/ping", nil)
	req.Header.Set("X-Ops-Key", "secret-ops-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"ok":true`)
}
