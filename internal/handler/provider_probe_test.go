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
// pointing at a recording httptest upstream) and registers both probe
// endpoints. The edit form is supposed to test unsaved values, so its
// overrides must win over the stored config; without overrides the stored
// config wins.
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
	router.POST("/api/v1/admin/providers/probe", h.ProbeConfig)
	router.POST("/api/v1/admin/providers/:id/probe", h.Probe)
	return router, calls, upstream.URL
}

func postProbePath(t *testing.T, router *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(http.MethodPost, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func postProbe(t *testing.T, router *gin.Engine, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	return postProbePath(t, router, "/api/v1/admin/providers/1/probe", body)
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

func expectNoProbeCall(t *testing.T, calls chan probeUpstreamCall) {
	t.Helper()
	select {
	case call := <-calls:
		t.Fatalf("rejected probe must not reach the upstream, got %+v", call)
	case <-time.After(200 * time.Millisecond):
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

// TestProviderHandler_Probe_PartialOverrideFallsBackPerField 钉住稀疏覆盖语义：
// 只覆盖 protocol 时 authStyle 回落库值；只覆盖 authStyle 时 protocol 回落库值。
func TestProviderHandler_Probe_PartialOverrideFallsBackPerField(t *testing.T) {
	t.Run("protocol override only keeps stored auth style", func(t *testing.T) {
		router, calls, upstreamURL := setupProviderProbeRouter(t)

		rec := postProbe(t, router, map[string]interface{}{
			"baseUrl":  upstreamURL,
			"protocol": string(provider.ProtocolOpenAI),
		})
		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

		call := expectProbeCall(t, calls)
		require.Equal(t, http.MethodGet, call.Method)
		require.Equal(t, "/models", call.Path)
		require.Equal(t, "sk-stored-secret", call.APIKey, "authStyle 未覆盖时应回落库中 api_key")
		require.Empty(t, call.Authz)
	})

	t.Run("auth style override only keeps stored protocol", func(t *testing.T) {
		router, calls, _ := setupProviderProbeRouter(t)

		rec := postProbe(t, router, map[string]interface{}{
			"authStyle": string(provider.AuthStyleAuthToken),
		})
		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

		call := expectProbeCall(t, calls)
		require.Equal(t, http.MethodPost, call.Method, "protocol 未覆盖时应回落库中 anthropic")
		require.Equal(t, "/v1/messages", call.Path)
		require.Equal(t, "Bearer sk-stored-secret", call.Authz)
		require.Empty(t, call.APIKey)
	})
}

// TestProviderHandler_Probe_RejectsUnknownOverrideEnums 钉住枚举校验：未知
// protocol/authStyle 覆盖直接 400、不打上游 —— 不再由 doProbe 的默认分支静默
// 回退 anthropic 产生「按所选 protocol 探测成功」的假象（评审反例 protocol=azure）。
func TestProviderHandler_Probe_RejectsUnknownOverrideEnums(t *testing.T) {
	router, calls, _ := setupProviderProbeRouter(t)

	rec := postProbe(t, router, map[string]interface{}{"protocol": "azure"})
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "azure")

	rec = postProbe(t, router, map[string]interface{}{"authStyle": "bearer_typo"})
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "bearer_typo")

	expectNoProbeCall(t, calls)
}

// TestProviderHandler_ProbeConfig_RejectsUnknownEnums 无 id 探测通道与 by-id
// 通道一致拒绝未知枚举，合法值不误伤。
func TestProviderHandler_ProbeConfig_RejectsUnknownEnums(t *testing.T) {
	router, calls, upstreamURL := setupProviderProbeRouter(t)

	bad := map[string]interface{}{
		"baseUrl":   upstreamURL,
		"apiKey":    "sk-x",
		"protocol":  "azure",
		"authStyle": string(provider.AuthStyleAPIKey),
	}
	rec := postProbePath(t, router, "/api/v1/admin/providers/probe", bad)
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "azure")

	bad = map[string]interface{}{
		"baseUrl":   upstreamURL,
		"apiKey":    "sk-x",
		"protocol":  string(provider.ProtocolOpenAI),
		"authStyle": "bearer_typo",
	}
	rec = postProbePath(t, router, "/api/v1/admin/providers/probe", bad)
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "bearer_typo")

	valid := map[string]interface{}{
		"baseUrl":   upstreamURL,
		"apiKey":    "sk-x",
		"protocol":  string(provider.ProtocolOpenAI),
		"authStyle": string(provider.AuthStyleAPIKey),
	}
	rec = postProbePath(t, router, "/api/v1/admin/providers/probe", valid)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	expectProbeCall(t, calls)
}
