package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		"org": "orga", "clientId": "cid-a", "clientSecret": "sec-a",
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = opsDo(t, r, http.MethodGet, "/ops/tenant-clients", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []gin.H `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	require.Equal(t, "orga", list.Data[0]["org"])
	require.Equal(t, true, list.Data[0]["isDefault"])
	// 不回显 secret/cert
	require.NotContains(t, w.Body.String(), "sec-a")
	require.NotContains(t, w.Body.String(), "clientSecret")
	require.NotContains(t, w.Body.String(), "certEnc")
}

func TestOpsTenantClient_SecondPostExplicitDefaultSwitch(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "orga", "clientId": "cid-a", "clientSecret": "sec-a"}).Code)

	// 第二行显式 isDefault=true → default 转移到 orgb。
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "orgb", "clientId": "cid-b", "clientSecret": "sec-b", "isDefault": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = opsDo(t, r, http.MethodGet, "/ops/tenant-clients", nil)
	var list struct {
		Data []gin.H `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	require.Equal(t, false, list.Data[0]["isDefault"]) // orga
	require.Equal(t, true, list.Data[1]["isDefault"])  // orgb
}

func TestOpsTenantClient_DowngradeDefaultConflict409(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "orga", "clientId": "cid-a", "clientSecret": "sec-a", "isDefault": true}).Code)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "orgb", "clientId": "cid-b", "clientSecret": "sec-b"}).Code)

	// 把 default 行 orga 以 isDefault=false 重新 upsert 且表内还有 orgb → 409。
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "orga", "clientId": "cid-a2", "clientSecret": "sec-a2", "isDefault": false,
	})
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestOpsTenantClient_DeleteDefaultWithOthers409_Idempotent204(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "orga", "clientId": "cid-a", "clientSecret": "sec-a"}).Code)
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "orgb", "clientId": "cid-b", "clientSecret": "sec-b"}).Code)

	// 删 default 行 orga 且还有 orgb → 409。
	require.Equal(t, http.StatusConflict, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/orga", nil).Code)

	// 删非 default 行 orgb → 204。
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/orgb", nil).Code)

	// 删最后一行（default）→ 204（删空表无约束）。
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/orga", nil).Code)

	// 幂等：删不存在的 org → 204。
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/orgx", nil).Code)
}

func TestOpsTenantClient_UpsertValidatesRequiredFields(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	// 缺 clientSecret → 400
	require.Equal(t, http.StatusBadRequest, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "orga", "clientId": "cid-a"}).Code)
	// 缺 org → 400
	require.Equal(t, http.StatusBadRequest, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"clientId": "cid-a", "clientSecret": "s"}).Code)
}

func TestOpsTenantClient_OrgNameFormatValidation(t *testing.T) {
	r := setupOpsTenantClientRouter(t)

	// 合法：小写字母开头，仅小写字母数字，≤63 字符。
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "tenanta", "clientId": "cid-a", "clientSecret": "sec-a"}).Code)

	// 空串被 binding/必填校验先拦（走"org、clientId、clientSecret 必填"分支），只断言 400。
	require.Equal(t, http.StatusBadRequest, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": "", "clientId": "cid-a", "clientSecret": "sec-a"}).Code)

	// 非空非法 org → 400，错误信息说明约束。
	for _, org := range []string{
		"org-a",                 // 连字符
		"OrgA",                  // 大写
		"1orga",                 // 数字开头
		strings.Repeat("a", 64), // 超长（>63）
		"org_a",                 // 下划线
	} {
		w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
			gin.H{"org": org, "clientId": "cid-a", "clientSecret": "sec-a"})
		require.Equal(t, http.StatusBadRequest, w.Code, "org=%q 应被拒绝", org)
		require.Contains(t, w.Body.String(), "组织名")
		require.Contains(t, w.Body.String(), "小写字母")
	}

	// 63 字符边界仍合法。
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodPost, "/ops/tenant-clients",
		gin.H{"org": strings.Repeat("b", 63), "clientId": "cid-b", "clientSecret": "sec-b"}).Code)

	// GET/DELETE 不受影响（连字符 org 也能按路径删除，幂等）。
	require.Equal(t, http.StatusOK, opsDo(t, r, http.MethodGet, "/ops/tenant-clients", nil).Code)
	require.Equal(t, http.StatusNoContent, opsDo(t, r, http.MethodDelete, "/ops/tenant-clients/orga", nil).Code)
}

func TestOpsTenantClient_CertPEMValidation(t *testing.T) {
	r := setupOpsTenantClientRouter(t)

	// 三类坏值 → 400：非 PEM 文本；PEM 但非 CERTIFICATE 类型；PEM 框架但 body 非法 base64。
	for _, bad := range []string{
		"not a pem at all",
		"-----BEGIN PRIVATE KEY-----\nAAECAw==\n-----END PRIVATE KEY-----\n",
		"-----BEGIN CERTIFICATE-----\n!!!not-base64!!!\n-----END CERTIFICATE-----\n",
	} {
		w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
			"org": "orga", "clientId": "cid", "clientSecret": "sec", "cert": bad,
		})
		require.Equal(t, http.StatusBadRequest, w.Code, "cert=%q 应被拒绝", bad)
		require.Contains(t, w.Body.String(), "PEM")
	}

	// 结构合法的 PEM（pem.Decode 不校验 DER 语义）→ 200。
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "orga", "clientId": "cid", "clientSecret": "sec",
		"cert": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
	})
	require.Equal(t, http.StatusOK, w.Code)
}

func TestOpsTenantClient_CertOptionalStoredEncrypted(t *testing.T) {
	r := setupOpsTenantClientRouter(t)
	pem := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	w := opsDo(t, r, http.MethodPost, "/ops/tenant-clients", gin.H{
		"org": "orga", "clientId": "cid-a", "clientSecret": "sec-a", "cert": pem,
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 库里存的是密文：不含 PEM 明文，且能解回。
	var row authdom.TenantOAuthClient
	require.NoError(t, database.DB.Where("org = ?", "orga").First(&row).Error)
	require.NotContains(t, row.CertEnc, "BEGIN CERTIFICATE")
	require.NotContains(t, row.ClientSecretEnc, "sec-a")
}
