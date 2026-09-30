package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// probeUpstreamCall records what the probe actually asked the upstream to do.
type probeUpstreamCall struct {
	Method string
	Path   string
	APIKey string // x-api-key header
	Authz  string // Authorization header
}

// setupProviderProbeRouter seeds one stored provider (anthropic + api_key,
// pointing at a recording httptest upstream) and registers the by-id probe
// endpoint. The edit form is supposed to test unsaved values, so its overrides
// must win over the stored config; without overrides the stored config wins.
func setupProviderProbeRouter(t *testing.T) (*gin.Engine, chan probeUpstreamCall, string) {
	t.Helper()

	calls := make(chan probeUpstreamCall, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- probeUpstreamCall{
			Method: r.Method,
			Path:   r.URL.Path,
			APIKey: r.Header.Get("x-api-key"),
			Authz:  r.Header.Get("Authorization"),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(upstream.Close)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	encrypted, err := provider.Encrypt("sk-stored-secret", providerModelsTestKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:           1,
		Key:          "probe-provider",
		Name:         "Probe Provider",
		Protocol:     string(provider.ProtocolAnthropic),
		AuthStyle:    string(provider.AuthStyleAPIKey),
		BaseURL:      upstream.URL,
		LockedAPIKey: encrypted,
	}).Error)

	gin.SetMode(gin.TestMode)
	h := NewProviderHandler(services.NewProviderService(providerModelsTestKey), nil, newHandlerTestAuditRecorder(t))
	router := gin.New()
	router.POST("/api/v1/admin/providers/:id/probe", h.Probe)
	return router, calls, upstream.URL
}

func postProbe(t *testing.T, router *gin.Engine, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/probe", reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func expectProbeCall(t *testing.T, calls chan probeUpstreamCall) probeUpstreamCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(3 * time.Second):
		t.Fatal("probe never reached the upstream")
		return probeUpstreamCall{}
	}
}

// TestProviderHandler_Probe_HonorsProtocolAndAuthStyleOverrides 锁定：编辑弹窗
// 改了 protocol/authStyle（未保存）点「测试连接」必须按表单值探测，不得回落到
// 库中旧值 —— 否则用户改了 protocol 测试结果仍来自旧 protocol，保存后才生效。
func TestProviderHandler_Probe_HonorsProtocolAndAuthStyleOverrides(t *testing.T) {
	router, calls, upstreamURL := setupProviderProbeRouter(t)

	rec := postProbe(t, router, map[string]interface{}{
		"baseUrl":   upstreamURL,
		"protocol":  string(provider.ProtocolOpenAI),
		"authStyle": string(provider.AuthStyleAuthToken),
	})
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Success bool `json:"success"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.True(t, resp.Data.Success, "probe must succeed: %s", rec.Body.String())

	call := expectProbeCall(t, calls)
	require.Equal(t, http.MethodGet, call.Method,
		"openai override must probe GET /models (stored anthropic would POST /v1/messages)")
	require.Equal(t, "/models", call.Path)
	require.Equal(t, "Bearer sk-stored-secret", call.Authz,
		"auth_token override must send Bearer with the stored key")
	require.Empty(t, call.APIKey, "x-api-key must not be set under auth_token")
}

// TestProviderHandler_Probe_WithoutOverridesUsesStoredConfig 锁定不带覆盖的既有
// 调用方（列表页探测 / CLI `provider test --id`）：仍以库中 protocol/authStyle 探测。
func TestProviderHandler_Probe_WithoutOverridesUsesStoredConfig(t *testing.T) {
	router, calls, _ := setupProviderProbeRouter(t)

	rec := postProbe(t, router, nil)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	call := expectProbeCall(t, calls)
	require.Equal(t, http.MethodPost, call.Method)
	require.Equal(t, "/v1/messages", call.Path)
	require.Equal(t, "sk-stored-secret", call.APIKey)
	require.Empty(t, call.Authz)
}
