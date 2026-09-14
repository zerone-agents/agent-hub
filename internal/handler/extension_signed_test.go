// H7.6 签名注册的 handler 端到端测试：签名信封 round-trip、伪造签名 400、
// 裸 manifest 兼容、公钥指纹记入 signed_by。
package handler

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// signedManifest 返回一份已签名的信封请求体与公钥。
func signedManifest(t *testing.T, manifest string, mutate func(pub ed25519.PublicKey, sig []byte) (string, string)) (body string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var canonical any
	if err := json.Unmarshal([]byte(manifest), &canonical); err != nil {
		t.Fatal(err)
	}
	norm, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, norm)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	if mutate != nil {
		sigB64, pubB64 = mutate(pub, sig)
	}
	return fmt.Sprintf(`{"manifest":%s,"signature":%q,"public_key":%q}`, manifest, sigB64, pubB64)
}

func TestRegisterSignedEnvelopeRoundTrip(t *testing.T) {
	r := extensionAdminRouter(t)
	body := signedManifest(t, handlerValidManifest, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// 版本响应携带 signedBy 指纹
	require.Contains(t, w.Body.String(), `"signedBy"`)
}

func TestRegisterForgedSignatureRejected400(t *testing.T) {
	r := extensionAdminRouter(t)
	body := signedManifest(t, handlerValidManifest, func(pub ed25519.PublicKey, sig []byte) (string, string) {
		forged := make([]byte, len(sig))
		copy(forged, sig)
		forged[0] ^= 0xff // 篡改签名
		return base64.StdEncoding.EncodeToString(forged),
			base64.StdEncoding.EncodeToString(pub)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "签名校验失败")
}

func TestRegisterTamperedManifestRejected400(t *testing.T) {
	r := extensionAdminRouter(t)
	// 先对原始 manifest 签名，再改动 manifest 内容
	body := signedManifest(t, handlerValidManifest, nil)
	body = strings.Replace(body, `"version": "1.0.0"`, `"version": "9.9.9"`, 1)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "签名校验失败")
}

func TestRegisterMismatchedKeyPairRejected400(t *testing.T) {
	r := extensionAdminRouter(t)
	_, otherPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body := signedManifest(t, handlerValidManifest, func(pub ed25519.PublicKey, sig []byte) (string, string) {
		return base64.StdEncoding.EncodeToString(sig),
			base64.StdEncoding.EncodeToString(otherPriv.Public().(ed25519.PublicKey))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "签名校验失败")
}

func TestRegisterSignatureWithoutPublicKeyRejected400(t *testing.T) {
	r := extensionAdminRouter(t)
	body := `{"manifest":` + handlerValidManifest + `,"signature":"aGVsbG8="}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "同时提供")
}

func TestRegisterBareManifestStillWorks(t *testing.T) {
	r := extensionAdminRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(handlerValidManifest))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	// 未签名版本 signedBy 为空
	require.Contains(t, w.Body.String(), `"signedBy":""`)
}
