package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupHealthTestDatabase(t *testing.T) {
	t.Helper()
	previous := database.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
}

func TestHealthCheckBuiltinDoesNotRequireCasdoor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHealthTestDatabase(t)

	r := gin.New()
	r.GET("/health", HealthCheckForAuthMode(false))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"status":"healthy","services":{"database":{"status":"healthy"},"casdoor":{"status":"disabled"}}}`, healthStableFields(t, w.Body.Bytes()))
}

func TestServiceHealthCheckBuiltinReportsCasdoorDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health/:service", ServiceHealthCheckForAuthMode(false))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/casdoor", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"success":true,"data":{"service":"casdoor","status":"disabled"}}`, w.Body.String())
}

func healthStableFields(t *testing.T, body []byte) string {
	t.Helper()
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &response))
	services := response["services"].(map[string]interface{})
	databaseStatus := services["database"].(map[string]interface{})
	delete(databaseStatus, "latency")
	stable := map[string]interface{}{"status": response["status"], "services": services}
	encoded, err := json.Marshal(stable)
	require.NoError(t, err)
	return string(encoded)
}
