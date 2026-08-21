package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authdom "control-panel/internal/domain/auth"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// opsTestEncryptionKey 是 32 字节 hex key（provider.Encrypt 要求）。
const opsTestEncryptionKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func setupOpsTenantClientRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authdom.TenantOAuthClient{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewOpsTenantClientHandler(repository.NewTenantOAuthClientRepository(), opsTestEncryptionKey)
	r.POST("/ops/tenant-clients", h.Upsert)
	r.GET("/ops/tenant-clients", h.List)
	r.DELETE("/ops/tenant-clients/:org", h.Delete)
	return r
}

func opsDo(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestOpsTenantClient_FirstPostAutoDefault(t *testing.T) {
	r := setupOpsTenantClientRouter(t)

	// isDefault 省略（false）且表空 → 首行自动提升为 default。
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "org-a", "clientId": "cid-a", "clientSecret": "sec-a",
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = opsDo(t, r, http.MethodGet, "/ops/tenant-clients", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []gin.H `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	require.Equal(t, "org-a", list.Data[0]["org"])
	require.Equal(t, true, list.Data[0]["isDefault"])
	// 不回显 secret/cert
	require.NotContains(t, w.Body.String(), "sec-a")
	require.NotContains(t, w.Body.String(), "clientSecret")
	require.NotContains(t, w.Body.String(), "certEnc")
}

func TestOpsTenantClient_SecondPostExplicitDefaultSwitch(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "org-a", "clientId": "cid-a", "clientSecret": "sec-a"}).Code)

	// 第二行显式 isDefault=true → default 转移到 org-b。
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "org-b", "clientId": "cid-b", "clientSecret": "sec-b", "isDefault": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = opsDo(t, r, http.MethodGet, "/ops/tenant-clients", nil)
	var list struct {
		Data []gin.H `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	require.Equal(t, false, list.Data[0]["isDefault"]) // org-a
	require.Equal(t, true, list.Data[1]["isDefault"])  // org-b
}

func TestOpsTenantClient_DowngradeDefaultConflict409(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "org-a", "clientId": "cid-a", "clientSecret": "sec-a", "isDefault": true}).Code)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "org-b", "clientId": "cid-b", "clientSecret": "sec-b"}).Code)

	// 把 default 行 org-a 以 isDefault=false 重新 upsert 且表内还有 org-b → 409。
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "org-a", "clientId": "cid-a2", "clientSecret": "sec-a2", "isDefault": false,
	})
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestOpsTenantClient_DeleteDefaultWithOthers409_Idempotent204(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "org-a", "clientId": "cid-a", "clientSecret": "sec-a"}).Code)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "org-b", "clientId": "cid-b", "clientSecret": "sec-b"}).Code)

	// 删 default 行 org-a 且还有 org-b → 409。
	require.Equal(t, http.StatusConflict, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/org-a", nil).Code)

	// 删非 default 行 org-b → 204。
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/org-b", nil).Code)

	// 删最后一行（default）→ 204（删空表无约束）。
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/org-a", nil).Code)

	// 幂等：删不存在的 org → 204。
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/org-x", nil).Code)
}

func TestOpsTenantClient_UpsertValidatesRequiredFields(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	// 缺 clientSecret → 400
	require.Equal(t, http.StatusBadRequest, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "org-a", "clientId": "cid-a"}).Code)
	// 缺 org → 400
	require.Equal(t, http.StatusBadRequest, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"clientId": "cid-a", "clientSecret": "s"}).Code)
}

func TestOpsTenantClient_CertOptionalStoredEncrypted(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	pem := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "org-a", "clientId": "cid-a", "clientSecret": "sec-a", "cert": pem,
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 库里存的是密文：不含 PEM 明文，且能解回。
	var row authdom.TenantOAuthClient
	require.NoError(t, database.DB.Where("org = ?", "org-a").First(&row).Error)
	require.NotContains(t, row.CertEnc, "BEGIN CERTIFICATE")
	require.NotContains(t, row.ClientSecretEnc, "sec-a")
}
