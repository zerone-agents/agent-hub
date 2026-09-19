// H7.6 CLI 集成测试：create→validate、pack 产物完整性、migrate-manifest 升级。
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"control-panel/internal/domain/extension"
	"control-panel/internal/extensionmanifest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestCreateThenValidate：create 生成的骨架必须通过严格校验（验收硬要求）。
func TestCreateThenValidate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, cmdCreate(globalFlags{}, []string{"io.zerone.itest", "--dir", dir}))

	root := filepath.Join(dir, "io.zerone.itest")
	require.FileExists(t, filepath.Join(root, "extension.yaml"))
	require.FileExists(t, filepath.Join(root, "README.md"))
	require.FileExists(t, filepath.Join(root, "ui/card.yaml"))

	require.NoError(t, cmdValidate([]string{root}))
}

// TestValidateRejectsBadManifest：非法 manifest 输出中文错误并非零退出。
func TestValidateRejectsBadManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, cmdCreate(globalFlags{}, []string{"io.zerone.badtest", "--dir", dir}))
	root := filepath.Join(dir, "io.zerone.badtest")
	require.NoError(t, os.WriteFile(filepath.Join(root, "extension.yaml"),
		[]byte("apiVersion: agenthub.extension/v1alpha1\nname: not-dns-name\nversion: 1.0.0\ndisplayName: x\ndescription: y\n"), 0o644))
	require.Error(t, cmdValidate([]string{root}))
}

// TestPackRoundTrip：pack 产物可解包，manifest.json 与文件名中的 name-version
// 一致，且 sha256 文件与包内容一致；签名文件可被 VerifyExtension 接受。
func TestPackRoundTrip(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, cmdCreate(globalFlags{}, []string{"io.zerone.packtest", "--dir", dir}))
	root := filepath.Join(dir, "io.zerone.packtest")

	// 生成签名密钥并写入临时私钥文件
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyFile := filepath.Join(dir, "ed25519.key")
	require.NoError(t, os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600))

	out := filepath.Join(dir, "dist")
	require.NoError(t, cmdPack(globalFlags{}, []string{root, "--sign", "--key", keyFile, "--output", out}))

	tgzPath := filepath.Join(out, "io.zerone.packtest-0.1.0.tgz")
	require.FileExists(t, tgzPath)

	// sha256 文件与包内容一致
	sum, err := sha256File(tgzPath)
	require.NoError(t, err)
	shaFile, err := os.ReadFile(tgzPath + ".sha256")
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(sum), string(shaFile[:64]))

	// 解包：首条目 manifest.json 通过严格校验且 name/version 与文件名一致
	f, err := os.Open(tgzPath)
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	first := true
	var sawExtYaml, sawReadme bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if first {
			first = false
			require.Equal(t, "manifest.json", hdr.Name)
			raw, err := io.ReadAll(tr)
			require.NoError(t, err)
			m, errs := extensionmanifest.ValidateExtensionManifest(raw)
			require.Empty(t, errs)
			require.Equal(t, "io.zerone.packtest", m.Name)
			require.Equal(t, "0.1.0", m.Version)
			continue
		}
		switch hdr.Name {
		case "extension.yaml":
			sawExtYaml = true
		case "README.md":
			sawReadme = true
		}
	}
	require.True(t, sawExtYaml, "包内应包含 extension.yaml")
	require.True(t, sawReadme, "包内应包含 README.md")

	// 签名 round-trip：.sig 的签名与包内 manifest 的规范 JSON 通过验签
	sigRaw, err := os.ReadFile(tgzPath + ".sig")
	require.NoError(t, err)
	var sigEnv struct {
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
	}
	require.NoError(t, json.Unmarshal(sigRaw, &sigEnv))
	require.Equal(t, base64.StdEncoding.EncodeToString(pub), sigEnv.PublicKey)

	m, err := loadManifestYAML(filepath.Join(root, "extension.yaml"))
	require.NoError(t, err)
	canonical, err := manifestToCanonicalJSON(m)
	require.NoError(t, err)
	fingerprint, err := extension.VerifyExtension(string(canonical), sigEnv.Signature, sigEnv.PublicKey)
	require.NoError(t, err)
	require.Equal(t, publicKeyFingerprint(pub), fingerprint)
}

// TestMigrateManifest：旧宽松格式升级为严格 v1 并通过校验。
func TestMigrateManifest(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.yaml")
	require.NoError(t, os.WriteFile(old, []byte(`apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata:
  name: emotion-state
  namespace: io.zerone.emotion
  version: 1.2.0
  displayName: 情绪状态
  description: 为运行中的 Agent 提供短期心理状态
  publisher: zerone
compatibility:
  hub: ">=0.9.0 <2.0.0"
  runtimeProtocol: ">=1.0.0 <2.0.0"
dependencies:
  - package: io.zerone.organization.relations
    version: "^1.0.0"
    optional: true
permissions:
  state:
    read: ["io.zerone.emotion/*"]
    write: ["io.zerone.emotion/*"]
  events:
    consume: ["agenthub.task.*"]
contributes:
  stateSchemas:
    - id: emotion-state
      version: 1.0.0
      file: schemas/emotion-state.schema.json
`), 0o644))

	out := filepath.Join(dir, "upgraded.yaml")
	require.NoError(t, cmdMigrateManifest([]string{old, "--out", out}))

	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	var upgraded map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &upgraded))
	canonical, err := manifestToCanonicalJSON(normalizeYAMLValue(upgraded).(map[string]any))
	require.NoError(t, err)
	m, errs := extensionmanifest.ValidateExtensionManifest(canonical)
	require.Empty(t, errs)
	require.Equal(t, "io.zerone.emotion", m.Name)
	require.Equal(t, "1.2.0", m.Version)
	require.NotEmpty(t, m.Permissions)
	require.NotEmpty(t, m.StateSchemas)
	require.NotEmpty(t, m.Dependencies)
}
