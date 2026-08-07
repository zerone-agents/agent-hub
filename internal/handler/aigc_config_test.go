package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/aigc"
	"control-panel/internal/domain/provider"
	"control-panel/internal/middleware"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const aigcHandlerTestEncKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setupAigcConfigRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&aigc.Config{}))

	gin.SetMode(gin.TestMode)
	h := NewAigcConfigHandler(services.NewAigcConfigService(db, aigcHandlerTestEncKey, fakeModelSource{}))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") == "true" {
			c.Set("roles", []*casdoorsdk.Role{{Name: "agents-admin"}})
			c.Set("user_id", "user-1")
		}
	})
	g := router.Group("/api/v1/admin/aigc", middleware.RequireAdmin())
	g.GET("/config", h.Get)
	g.PUT("/config", h.Save)
	g.POST("/config/rotate-key", h.RotateKey)
	g.DELETE("/config", h.Delete)
	return router
}

func doReq(router *gin.Engine, method, path, body string, admin bool) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if admin {
		req.Header.Set("X-Test-Admin", "true")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAigcConfig_RequiresAdmin(t *testing.T) {
	router := setupAigcConfigRouter(t)
	rec := doReq(router, http.MethodGet, "/api/v1/admin/aigc/config", "", false)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAigcConfig_GetNotConfigured(t *testing.T) {
	router := setupAigcConfigRouter(t)
	rec := doReq(router, http.MethodGet, "/api/v1/admin/aigc/config", "", true)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"configured":false`)
}

func TestAigcConfig_SaveValidatesUSCC(t *testing.T) {
	router := setupAigcConfigRouter(t)
	rec := doReq(router, http.MethodPut, "/api/v1/admin/aigc/config",
		`{"uscc":"short","companyName":"公司"}`, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAigcConfig_SaveAndGetNeverLeaksSigningKey(t *testing.T) {
	router := setupAigcConfigRouter(t)
	rec := doReq(router, http.MethodPut, "/api/v1/admin/aigc/config",
		`{"uscc":"91320118MAK93FC72D","companyName":"南京测试科技有限公司"}`, true)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"contentProducer":"001191320118MAK93FC72D10000"`)
	require.NotContains(t, rec.Body.String(), "signingKey\"")
	require.NotContains(t, rec.Body.String(), "enc:")

	rec = doReq(router, http.MethodGet, "/api/v1/admin/aigc/config", "", true)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"signingKeyConfigured":true`)
	require.NotContains(t, rec.Body.String(), "enc:")

	rec = doReq(router, http.MethodPost, "/api/v1/admin/aigc/config/rotate-key", "", true)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "enc:")

	rec = doReq(router, http.MethodDelete, "/api/v1/admin/aigc/config", "", true)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(router, http.MethodGet, "/api/v1/admin/aigc/config", "", true)
	require.Contains(t, rec.Body.String(), `"configured":false`)
}

// fakeModelSource satisfies services.providerModelCodeSource for handler tests
// that don't exercise model-code assignment.
type fakeModelSource struct{}

func (fakeModelSource) ListAllModels() ([]provider.ProviderModel, error) {
	return nil, nil
}
