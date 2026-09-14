// H7.6 扩展签名纯函数测试。
package extension

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

const testManifest = `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.sig","version":"1.0.0","displayName":"签名测试","description":"x"}`

func signForTest(t *testing.T) (ed25519.PublicKey, string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	canonical, err := canonicalizeManifest([]byte(testManifest))
	require.NoError(t, err)
	sig := ed25519.Sign(priv, canonical)
	return pub,
		base64.StdEncoding.EncodeToString(sig),
		base64.StdEncoding.EncodeToString(pub)
}

func TestVerifyExtensionHappyPath(t *testing.T) {
	pub, sigB64, pubB64 := signForTest(t)
	fp, err := VerifyExtension(testManifest, sigB64, pubB64)
	require.NoError(t, err)
	require.Equal(t, PublicKeyFingerprint(pub), fp)
	require.Len(t, fp, 32)
}

func TestVerifyExtensionHexKeys(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	canonical, err := canonicalizeManifest([]byte(testManifest))
	require.NoError(t, err)
	sig := ed25519.Sign(priv, canonical)
	fp, err := VerifyExtension(testManifest,
		hex.EncodeToString(sig), hex.EncodeToString(pub))
	require.NoError(t, err)
	require.Equal(t, PublicKeyFingerprint(pub), fp)
}

func TestVerifyExtensionFieldOrderInsensitive(t *testing.T) {
	// 语义相同、字段顺序不同的 manifest 应与规范 JSON 验签结果一致
	reordered := `{"version":"1.0.0","name":"io.zerone.sig","description":"x","displayName":"签名测试","apiVersion":"agenthub.extension/v1alpha1"}`
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	canonical, err := canonicalizeManifest([]byte(reordered))
	require.NoError(t, err)
	sig := ed25519.Sign(priv, canonical)
	fp, err := VerifyExtension(reordered,
		base64.StdEncoding.EncodeToString(sig),
		base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	require.NoError(t, err)
	require.NotEmpty(t, fp)
}

func TestVerifyExtensionForgedSignature(t *testing.T) {
	_, _, pubB64 := signForTest(t)
	forged := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	_, err := VerifyExtension(testManifest, forged, pubB64)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSignatureInvalid))
}

func TestVerifyExtensionTamperedManifest(t *testing.T) {
	_, sigB64, pubB64 := signForTest(t)
	tampered := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.sig","version":"2.0.0","displayName":"签名测试","description":"x"}`
	_, err := VerifyExtension(tampered, sigB64, pubB64)
	require.True(t, errors.Is(err, ErrSignatureInvalid))
}

func TestVerifyExtensionMissingOrBadKeys(t *testing.T) {
	_, err := VerifyExtension(testManifest, "", "")
	require.True(t, errors.Is(err, ErrSignatureInvalid))

	_, err = VerifyExtension(testManifest, "bm90LWEta2V5", base64.StdEncoding.EncodeToString([]byte("short")))
	require.Error(t, err)
}
