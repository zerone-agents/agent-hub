package handler

// H4 回归（issue #232 计划）：认证路由下的同步协议四端点。
//
// 覆盖计划清单：401（无凭证）/403（guest）；能力六字段；claim 校验（origin/
// sourceId → 400）；PUT 生命周期（applied/duplicate/superseded=200、
// conflict/deleted=409 且附状态、payload invalid=400）；URL/body id 不同→400
// 且无 DB 变更；>8 MiB → 413；未知键 GET/未 claim PUT → 404；跨用户隔离；
// body 伪造 tenant 字段 → 400（身份只取认证上下文）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/jwtutil"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/chat"
	"control-panel/internal/middleware"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	syncHSrcA = "aaaa0000-0000-4000-8000-00000000a001"
	syncHSrcB = "aaaa0000-0000-4000-8000-00000000b002"
)

// syncStubProvider：按 token 返回固定身份（JWT 分支；CLI 分支不启用）。
type syncStubProvider struct{ users map[string]*auth.AuthUser }

func (p *syncStubProvider) ValidateAccessToken(tok string) (*auth.AuthUser, error) {
	if u, ok := p.users[tok]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("invalid token %q", tok)
}
func (p *syncStubProvider) RefreshToken(string) (*auth.TokenPair, error) {
	return nil, fmt.Errorf("unsupported")
}
func (p *syncStubProvider) GetUserIdentity(string) (*auth.AuthUser, bool) { return nil, false }
func (p *syncStubProvider) Mode() string                                  { return "builtin" }

func setupChatSyncHandlerEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.SessionSyncState{}, &chat.UploadRecord{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	provider := &syncStubProvider{users: map[string]*auth.AuthUser{
		"tok-u1":    {ID: "u1", Username: "user1", TenantID: "t1", Roles: []string{"member"}},
		"tok-u2":    {ID: "u2", Username: "user2", TenantID: "t1", Roles: []string{"member"}},
		"tok-guest": {ID: "g1", Username: "guest1", TenantID: "t1", Roles: []string{authdom.RoleGuest}},
	}}
	h := NewChatSyncHandler(services.NewChatSyncService("https://hub.example.com"))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v2/chat/sync", middleware.JWTAuthWithCLI(nil, provider), jwtutil.GuestGuard())
	g.GET("/capabilities", h.Capabilities)
	g.POST("/sessions/:id/claim", h.Claim)
	g.GET("/sessions/:id", h.GetState)
	g.PUT("/sessions/:id", h.Apply)
	return r, db
}

func syncDo(r *gin.Engine, method, path, token string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func jsonMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m), "body=%s", rec.Body.String())
	return m
}

// syncEnvelopeBody：构造合法 wire envelope JSON（摘要经 DigestEnvelope 正规
// 计算）。alt=true 时改变内容重算摘要（同 version 异摘要用例）。
func syncEnvelopeBody(t *testing.T, sourceID, sessionID string, version int64, kind chat.Kind, msgCount int, alt bool) []byte {
	t.Helper()
	e := chat.Envelope{
		ProtocolVersion:   chat.ProtocolVersion,
		ProjectionVersion: chat.ProjectionVersion,
		SourceID:          sourceID,
		AppSessionID:      sessionID,
		Version:           version,
		SnapshotID:        fmt.Sprintf("%08d-1111-4222-8333-%012d", version, version%1000000000000),
		Kind:              kind,
	}
	payloadVal := any(nil)
	if kind == chat.KindReplace {
		rp := &chat.ReplacePayload{
			Session: chat.SyncSessionPayload{
				ID: sessionID, Title: fmt.Sprintf("t-v%d", version),
				CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
			},
			Messages: []chat.SyncMessagePayload{},
		}
		for i := 0; i < msgCount; i++ {
			content := fmt.Sprintf("c%d@v%d", i, version)
			if alt {
				content = "alt-" + content
			}
			rp.Messages = append(rp.Messages, chat.SyncMessagePayload{
				ID: fmt.Sprintf("%s-m%d", sessionID, i), Role: "user",
				Content: content, CreatedAt: "2026-10-01T00:00:00Z", Ordinal: int64(i),
			})
		}
		e.Payload = rp
		b, err := json.Marshal(rp)
		require.NoError(t, err)
		e.RawPayload = b
		payloadVal = rp
	} else {
		e.RawPayload = json.RawMessage("null")
	}
	d, err := chat.DigestEnvelope(e)
	require.NoError(t, err)

	wire := map[string]any{
		"protocolVersion":   chat.ProtocolVersion,
		"projectionVersion": chat.ProjectionVersion,
		"sourceId":          sourceID,
		"appSessionId":      sessionID,
		"version":           strconv.FormatInt(version, 10),
		"snapshotId":        e.SnapshotID,
		"kind":              string(kind),
		"payload":           payloadVal,
		"digest":            d,
	}
	b, err := json.Marshal(wire)
	require.NoError(t, err)
	return b
}

func syncStateRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&chat.SessionSyncState{}).Count(&n).Error)
	return n
}

func TestChatSyncHandlerAuthAndGuest(t *testing.T) {
	r, _ := setupChatSyncHandlerEnv(t)

	rec := syncDo(r, http.MethodGet, "/api/v2/chat/sync/capabilities", "", nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = syncDo(r, http.MethodGet, "/api/v2/chat/sync/capabilities", "tok-guest", nil)
	require.Equal(t, http.StatusForbidden, rec.Code, "guest 不得访问同步协议，body=%s", rec.Body.String())

	rec = syncDo(r, http.MethodPut, "/api/v2/chat/sync/sessions/s1", "tok-guest", syncEnvelopeBody(t, syncHSrcA, "s1", 1, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestChatSyncHandlerCapabilities(t *testing.T) {
	r, _ := setupChatSyncHandlerEnv(t)

	rec := syncDo(r, http.MethodGet, "/api/v2/chat/sync/capabilities", "tok-u1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	m := jsonMap(t, rec)
	require.Equal(t, []any{float64(2)}, m["protocolVersions"])
	require.Equal(t, []any{float64(1)}, m["projectionVersions"])
	require.Equal(t, "sha256-jcs-rfc8785", m["digestAlgorithm"])
	require.Equal(t, float64(8388608), m["maxRequestBytes"])
	require.Equal(t, true, m["tombstone"])
	require.Equal(t, "https://hub.example.com", m["serverIdentity"])
}

func TestChatSyncHandlerClaimValidation(t *testing.T) {
	r, db := setupChatSyncHandlerEnv(t)

	rec := syncDo(r, http.MethodPost, "/api/v2/chat/sync/sessions/s1/claim", "tok-u1",
		[]byte(`{"sourceId":"`+syncHSrcA+`","origin":"bogus"}`))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

	rec = syncDo(r, http.MethodPost, "/api/v2/chat/sync/sessions/s1/claim", "tok-u1",
		[]byte(`{"sourceId":"not-a-uuid","origin":"imported"}`))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	require.Zero(t, syncStateRows(t, db), "非法 claim 不得留状态行")
}

func TestChatSyncHandlerClaimApplyLifecycle(t *testing.T) {
	r, db := setupChatSyncHandlerEnv(t)
	ctx := "/api/v2/chat/sync/sessions/s1"

	// claim → 200 claimed-empty
	rec := syncDo(r, http.MethodPost, ctx+"/claim", "tok-u1",
		[]byte(`{"sourceId":"`+syncHSrcA+`","origin":"imported"}`))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	st := jsonMap(t, rec)
	require.Equal(t, "claimed-empty", st["status"])
	require.Equal(t, "0", st["version"])
	require.Equal(t, syncHSrcA, st["sourceId"])

	// PUT v1 → 200 applied
	env1 := syncEnvelopeBody(t, syncHSrcA, "s1", 1, chat.KindReplace, 1, false)
	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", env1)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	m := jsonMap(t, rec)
	require.Equal(t, "applied", m["status"])
	state := m["state"].(map[string]any)
	require.Equal(t, "1", state["version"])
	require.Equal(t, "active", state["status"])

	// 重放 → duplicate
	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", env1)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "duplicate", jsonMap(t, rec)["status"])

	// 同 version 异摘要 → 409 conflict 且附当前身份
	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", syncEnvelopeBody(t, syncHSrcA, "s1", 1, chat.KindReplace, 1, true))
	require.Equal(t, http.StatusConflict, rec.Code, "body=%s", rec.Body.String())
	m = jsonMap(t, rec)
	require.Equal(t, "conflict", m["status"])
	require.Equal(t, "1", m["state"].(map[string]any)["version"])

	// GET → active v1
	rec = syncDo(r, http.MethodGet, ctx, "tok-u1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "active", jsonMap(t, rec)["status"])

	// v3 → applied；随后 v2（降版本）→ superseded
	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", syncEnvelopeBody(t, syncHSrcA, "s1", 3, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "applied", jsonMap(t, rec)["status"])
	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", syncEnvelopeBody(t, syncHSrcA, "s1", 2, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "superseded", jsonMap(t, rec)["status"])

	// 正文落库
	var msgs int64
	require.NoError(t, db.Model(&chat.Message{}).Where("user_id = ? AND session_id = ?", "u1", "s1").Count(&msgs).Error)
	require.EqualValues(t, 1, msgs)
}

func TestChatSyncHandlerURLBodyMismatch(t *testing.T) {
	r, db := setupChatSyncHandlerEnv(t)

	rec := syncDo(r, http.MethodPost, "/api/v2/chat/sync/sessions/s1/claim", "tok-u1",
		[]byte(`{"sourceId":"`+syncHSrcA+`","origin":"imported"}`))
	require.Equal(t, http.StatusOK, rec.Code)

	// URL s2、body appSessionId=s1 → 400；s1 状态不变、s2 无行
	rec = syncDo(r, http.MethodPut, "/api/v2/chat/sync/sessions/s2", "tok-u1",
		syncEnvelopeBody(t, syncHSrcA, "s1", 1, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

	rec = syncDo(r, http.MethodGet, "/api/v2/chat/sync/sessions/s1", "tok-u1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "0", jsonMap(t, rec)["version"])
	rec = syncDo(r, http.MethodGet, "/api/v2/chat/sync/sessions/s2", "tok-u1", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.EqualValues(t, 1, syncStateRows(t, db))
}

func TestChatSyncHandlerInvalidPayloads(t *testing.T) {
	r, db := setupChatSyncHandlerEnv(t)
	ctx := "/api/v2/chat/sync/sessions/s1"

	rec := syncDo(r, http.MethodPost, ctx+"/claim", "tok-u1",
		[]byte(`{"sourceId":"`+syncHSrcA+`","origin":"imported"}`))
	require.Equal(t, http.StatusOK, rec.Code)

	badBodies := map[string][]byte{
		"duplicate-key": []byte(`{"protocolVersion":2,"projectionVersion":1,"sourceId":"` + syncHSrcA +
			`","appSessionId":"s1","version":"1","version":"2","snapshotId":"x","kind":"replace","payload":null,"digest":"` + strings.Repeat("0", 64) + `"}`),
		"digest-mismatch": syncEnvelopeWithDigest(t, syncHSrcA, "s1", 1, strings.Repeat("1", 64)),
		"protocol-mismatch": []byte(`{"protocolVersion":3,"projectionVersion":1,"sourceId":"` + syncHSrcA +
			`","appSessionId":"s1","version":"1","snapshotId":"x","kind":"replace","payload":null,"digest":"` + strings.Repeat("0", 64) + `"}`),
		"fabricated-tenant": syncEnvelopeWithExtraTenant(t, syncHSrcA, "s1", 1),
	}
	for name, body := range badBodies {
		rec := syncDo(r, http.MethodPut, ctx, "tok-u1", body)
		require.Equal(t, http.StatusBadRequest, rec.Code, "%s: body=%s", name, rec.Body.String())
	}
	// 全部拒绝后状态仍是 claim 占位
	rec = syncDo(r, http.MethodGet, ctx, "tok-u1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "0", jsonMap(t, rec)["version"], "非法载荷不得推进状态")
	require.EqualValues(t, 1, syncStateRows(t, db))
}

// syncEnvelopeWithDigest：合法形状 + 指定摘要（错误摘要用例）。
func syncEnvelopeWithDigest(t *testing.T, sourceID, sessionID string, version int64, digest string) []byte {
	t.Helper()
	body := syncEnvelopeBody(t, sourceID, sessionID, version, chat.KindReplace, 1, false)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	m["digest"] = digest
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// syncEnvelopeWithExtraTenant：伪造 tenant 字段（身份只取认证上下文）。
func syncEnvelopeWithExtraTenant(t *testing.T, sourceID, sessionID string, version int64) []byte {
	t.Helper()
	body := syncEnvelopeBody(t, sourceID, sessionID, version, chat.KindReplace, 1, false)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	m["tenant"] = "fabricated"
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func TestChatSyncHandlerTooLarge(t *testing.T) {
	r, _ := setupChatSyncHandlerEnv(t)

	huge := []byte(`{"pad":"` + strings.Repeat("x", 8388609) + `"}`)
	rec := syncDo(r, http.MethodPut, "/api/v2/chat/sync/sessions/s1", "tok-u1", huge)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, "body=%.200s", rec.Body.String())
}

func TestChatSyncHandlerUnknownAndUnclaimed(t *testing.T) {
	r, _ := setupChatSyncHandlerEnv(t)

	rec := syncDo(r, http.MethodGet, "/api/v2/chat/sync/sessions/nope", "tok-u1", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = syncDo(r, http.MethodPut, "/api/v2/chat/sync/sessions/sx", "tok-u1",
		syncEnvelopeBody(t, syncHSrcA, "sx", 1, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusNotFound, rec.Code, "未 claim 的键不得自行创建覆盖")
}

func TestChatSyncHandlerDeletedReplace409(t *testing.T) {
	r, _ := setupChatSyncHandlerEnv(t)
	ctx := "/api/v2/chat/sync/sessions/s1"

	rec := syncDo(r, http.MethodPost, ctx+"/claim", "tok-u1",
		[]byte(`{"sourceId":"`+syncHSrcA+`","origin":"imported"}`))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, http.StatusOK, syncDo(r, http.MethodPut, ctx, "tok-u1",
		syncEnvelopeBody(t, syncHSrcA, "s1", 1, chat.KindReplace, 1, false)).Code)

	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", syncEnvelopeBody(t, syncHSrcA, "s1", 2, chat.KindDelete, 0, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "applied", jsonMap(t, rec)["status"])

	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", syncEnvelopeBody(t, syncHSrcA, "s1", 3, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusConflict, rec.Code, "tombstone 拒绝复活")
	m := jsonMap(t, rec)
	require.Equal(t, "deleted", m["status"])
	require.Equal(t, "2", m["state"].(map[string]any)["version"])

	rec = syncDo(r, http.MethodGet, ctx, "tok-u1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "deleted", jsonMap(t, rec)["status"])
}

func TestChatSyncHandlerCrossUserIsolation(t *testing.T) {
	r, _ := setupChatSyncHandlerEnv(t)
	ctx := "/api/v2/chat/sync/sessions/s1"

	rec := syncDo(r, http.MethodPost, ctx+"/claim", "tok-u1",
		[]byte(`{"sourceId":"`+syncHSrcA+`","origin":"imported"}`))
	require.Equal(t, http.StatusOK, rec.Code)

	// u2 视角：同租户不同用户——键不可见（同步键含 user）
	rec = syncDo(r, http.MethodPut, ctx, "tok-u2", syncEnvelopeBody(t, syncHSrcA, "s1", 1, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusNotFound, rec.Code)
	rec = syncDo(r, http.MethodGet, ctx, "tok-u2", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// 异 source 也不得经他人键写入
	rec = syncDo(r, http.MethodPut, ctx, "tok-u1", syncEnvelopeBody(t, syncHSrcB, "s1", 1, chat.KindReplace, 1, false))
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, "conflict", jsonMap(t, rec)["status"])
}
