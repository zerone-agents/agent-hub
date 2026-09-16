// H7.4 扩展身份凭据服务测试（P0）。
//
// 覆盖：安装即签发（明文一次）、明文不落库、校验成功/失败路径、
// 租户回退 default、停用即时失效、轮换旧凭据作废、未安装/不存在报错，
// 以及启动期断言用的 CountGrants。
package services

import (
	"strings"
	"testing"

	"control-panel/internal/domain/extension"

	"github.com/stretchr/testify/require"
)

type identityFixture struct {
	*lifecycleFixture
	identity *ExtensionIdentityService
}

func newIdentityFixture(t *testing.T) *identityFixture {
	t.Helper()
	f := newLifecycleFixture(t)
	return &identityFixture{lifecycleFixture: f, identity: NewExtensionIdentityService(f.db)}
}

// installWithToken 注册并安装扩展，返回扩展行与安装时签发的一次性凭据。
func (f *identityFixture) installWithToken(t *testing.T, tenantID, name string) (*extension.Extension, string) {
	t.Helper()
	res := f.register(t, tenantID, lifecycleManifest(name, "1.0.0", 0, "", ""))
	installed, err := f.lifecycle.Install(tenantID, res.Extension.ID, "1.0.0", "tester")
	require.NoError(t, err)
	require.NotEmpty(t, installed.ExtensionToken, "安装必须签发扩展身份凭据")
	return res.Extension, installed.ExtensionToken
}

func TestExtensionIdentityIssueAndVerify(t *testing.T) {
	f := newIdentityFixture(t)
	ext, token := f.installWithToken(t, "tenant-a", "io.zerone.identity-ok")

	require.Equal(t, []string{"zx1"}, strings.SplitN(token, ".", 2)[:1], "凭据带版本前缀")
	require.Equal(t, 3, len(strings.Split(token, ".")), "凭据形如 zx1.<tokenId>.<secret>")

	ok, err := f.identity.Verify("tenant-a", ext.Name, token)
	require.NoError(t, err)
	require.True(t, ok)

	// 逐项失败路径。
	cases := []struct {
		name string
		tid  string
		ext  string
		tok  string
	}{
		{"凭证被篡改一个字符", "tenant-a", ext.Name, token[:len(token)-1] + "X"},
		{"凭证只剩前缀", "tenant-a", ext.Name, "zx1."},
		{"凭证前缀错误", "tenant-a", ext.Name, "zx2." + strings.SplitN(token, ".", 2)[1]},
		{"凭证完全不是这个格式", "tenant-a", ext.Name, "not-a-token"},
		{"空 token", "tenant-a", ext.Name, ""},
		{"空扩展名", "tenant-a", "", token},
		{"别的扩展名", "tenant-a", "io.zerone.other", token},
		{"别的租户", "tenant-b", ext.Name, token},
	}
	for _, c := range cases {
		got, err := f.identity.Verify(c.tid, c.ext, c.tok)
		require.NoError(t, err, c.name)
		require.False(t, got, c.name)
	}

	// 换掉 tokenId 段（但 secret 正确）也必须失败：tokenId 参与定位。
	parts := strings.Split(token, ".")
	tampered := parts[0] + "." + strings.Repeat("0", len(parts[1])) + "." + parts[2]
	ok, err = f.identity.Verify("tenant-a", ext.Name, tampered)
	require.NoError(t, err)
	require.False(t, ok, "tokenId 不匹配必须失败")
}

func TestExtensionIdentityVerifyFallsBackToDefaultTenant(t *testing.T) {
	f := newIdentityFixture(t)
	// 平台级内置扩展装在 default 租户，普通租户的请求也要能认证它。
	ext, token := f.installWithToken(t, BuiltinPersonaPackTenant, "io.zerone.platform-pack")

	ok, err := f.identity.Verify("tenant-x", ext.Name, token)
	require.NoError(t, err)
	require.True(t, ok, "租户内无记录时应回退 default 平台级租户")

	// 租户自己装了同名扩展：以租户自己的凭据为准，平台级凭据不再对本租户生效。
	_, tenantToken := f.installWithToken(t, "tenant-x", ext.Name)
	ok, err = f.identity.Verify("tenant-x", ext.Name, token)
	require.NoError(t, err)
	require.False(t, ok, "租户内已安装该扩展时不得继续回退平台级凭据")
	ok, err = f.identity.Verify("tenant-x", ext.Name, tenantToken)
	require.NoError(t, err)
	require.True(t, ok)

	// default 租户自己的凭据不受其他租户影响。
	ok, err = f.identity.Verify(BuiltinPersonaPackTenant, ext.Name, token)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestExtensionIdentityDisabledRejectedThenRestored(t *testing.T) {
	f := newIdentityFixture(t)
	ext, token := f.installWithToken(t, "tenant-a", "io.zerone.identity-lifecycle")

	_, err := f.lifecycle.Disable("tenant-a", ext.ID)
	require.NoError(t, err)
	ok, err := f.identity.Verify("tenant-a", ext.Name, token)
	require.NoError(t, err)
	require.False(t, ok, "停用扩展后凭据必须立即失效")

	_, err = f.lifecycle.Enable("tenant-a", ext.ID)
	require.NoError(t, err)
	ok, err = f.identity.Verify("tenant-a", ext.Name, token)
	require.NoError(t, err)
	require.True(t, ok, "重新启用后凭据恢复生效（凭据不因启停被清除）")
}

func TestExtensionIdentityRotationInvalidatesOldToken(t *testing.T) {
	f := newIdentityFixture(t)
	ext, token := f.installWithToken(t, "tenant-a", "io.zerone.identity-rotate")

	rotated, err := f.identity.IssueCredential("tenant-a", ext.ID)
	require.NoError(t, err)
	require.NotEqual(t, token, rotated.Token)

	ok, err := f.identity.Verify("tenant-a", ext.Name, token)
	require.NoError(t, err)
	require.False(t, ok, "轮换后旧凭据立即失效")

	ok, err = f.identity.Verify("tenant-a", ext.Name, rotated.Token)
	require.NoError(t, err)
	require.True(t, ok)

	// 描述接口返回最新凭据的公开信息，且不含 secret。
	info, err := f.identity.DescribeCredential("tenant-a", ext.ID)
	require.NoError(t, err)
	require.True(t, info.Issued)
	require.Equal(t, rotated.TokenID, info.TokenID)
	require.NotNil(t, info.IssuedAt)
}

func TestExtensionIdentityStoresOnlyHash(t *testing.T) {
	f := newIdentityFixture(t)
	ext, token := f.installWithToken(t, "tenant-a", "io.zerone.identity-hash")

	var inst extension.Install
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", ext.ID).First(&inst).Error)
	secret := strings.Split(token, ".")[2]
	require.NotEqual(t, secret, inst.AuthTokenHash, "库里不得存明文 secret")
	require.NotEqual(t, token, inst.AuthTokenHash)
	require.Equal(t, hashExtensionSecret(secret), inst.AuthTokenHash)
	require.Contains(t, token, inst.AuthTokenID, "tokenId 是明文 token 的一部分")
	require.True(t, inst.HasCredential())
}

func TestExtensionIdentityIssueRequiresInstalledExtension(t *testing.T) {
	f := newIdentityFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.not-installed", "1.0.0", 0, "", ""))

	_, err := f.identity.IssueCredential("tenant-a", res.Extension.ID)
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 404, code, "未安装不得签发凭据")

	_, err = f.identity.IssueCredential("tenant-a", 999999)
	code, ok = IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 404, code, "扩展不存在")
}

// manifest 里声明的权限必须真正落地 grants —— CountGrants 是启动期
// 断言（main.go）与运维核对共用的口径。
func TestExtensionAuthzCountGrants(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, f.db.AutoMigrate(&extension.Grant{}, &extension.AccessAudit{}))
	authz := NewExtensionAuthzService(f.db)
	f.lifecycle.SetAuthzService(authz)

	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	for _, name := range BuiltinPersonaPackNames() {
		count, err := authz.CountGrants(BuiltinPersonaPackTenant, name)
		require.NoError(t, err)
		require.Greater(t, count, int64(0), "内置扩展 %s 必须有 grants 行", name)
	}
	count, err := authz.CountGrants(BuiltinPersonaPackTenant, "io.zerone.never-seeded")
	require.NoError(t, err)
	require.Zero(t, count)

	_, err = authz.CountGrants(BuiltinPersonaPackTenant, "  ")
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 400, code)
}
