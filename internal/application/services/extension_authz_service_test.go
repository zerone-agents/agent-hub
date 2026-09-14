// H7.4 扩展权限服务测试：授权同步（auto/approval）、Enforce 授予/拒绝/
// 撤销即时生效、停用联动、卸载保留审计、purge 删除、审计行写入。
package services

import (
	"fmt"
	"testing"

	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type authzFixture struct {
	db        *gorm.DB
	registry  *ExtensionService
	lifecycle *ExtensionLifecycleService
	authz     *ExtensionAuthzService
}

func newAuthzFixture(t *testing.T) *authzFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
		&extension.Grant{}, &extension.AccessAudit{},
		&rundomain.StateSchema{},
	))
	f := &authzFixture{
		db:        db,
		registry:  NewExtensionService(db),
		lifecycle: NewExtensionLifecycleService(db),
		authz:     NewExtensionAuthzService(db),
	}
	f.lifecycle.SetAuthzService(f.authz)
	return f
}

// authzManifest 生成带权限声明的 manifest；mode 为空则省略 mode 字段（默认 auto）。
func authzManifest(name, version, permissions string) string {
	return fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "权限测试扩展",
	  "description": "authz test",
	  "permissions": %s
	}`, name, version, permissions)
}

func (f *authzFixture) registerAndInstall(t *testing.T, tenantID, manifest string) *extension.Extension {
	t.Helper()
	res, err := f.registry.Register(tenantID, RegisterExtensionInput{Manifest: []byte(manifest), Source: extension.SourceUpload})
	require.NoError(t, err)
	_, err = f.lifecycle.Install(tenantID, res.Extension.ID, res.Version.Version, "admin-1")
	require.NoError(t, err)
	return res.Extension
}

func TestAuthzInstallSyncsAutoGrants(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.authz", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read","write"]}]`))

	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "state", grants[0].Permission)
	require.Equal(t, "mood", grants[0].Scope)
	require.Equal(t, extension.GrantStatusActive, grants[0].Status)
	require.True(t, grants[0].IsActive)
	require.Contains(t, grants[0].Actions, "read")
	require.Equal(t, "1.0.0", grants[0].SourceVersion)

	// Enforce：声明内的动作放行
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "state", "read", "", "10.0.0.1"))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "state", "write", "", "10.0.0.1"))
	// 未声明动作 / 未声明权限类：拒绝
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "state", "delete", "", "10.0.0.1"))
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "agent", "read", "", "10.0.0.1"))
}

func TestAuthzApprovalModePendingUntilApproved(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.appr", "1.0.0",
		`[{"permission":"network","scope":"*","actions":["fetch"],"mode":"approval"}]`))

	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, extension.GrantStatusPending, grants[0].Status)

	// pending 未批准：拒绝
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "network", "fetch", "", ""))

	// 批准后放行
	require.NoError(t, f.authz.ApproveGrant("tenant-a", ext.ID, grants[0].ID, "admin-2"))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "network", "fetch", "", ""))
}

func TestAuthzRevokeEffectiveImmediately(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.revoke", "1.0.0",
		`[{"permission":"message","scope":"*","actions":["send"]}]`))
	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)

	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "message", "send", "", ""))
	require.NoError(t, f.authz.RevokeGrant("tenant-a", ext.ID, grants[0].ID))
	// 撤销即时生效
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "message", "send", "", ""))
	// 重复撤销 404
	require.Error(t, f.authz.RevokeGrant("tenant-a", ext.ID, grants[0].ID))
}

func TestAuthzDisableDisablesGrantsKeepAudit(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.disable", "1.0.0",
		`[{"permission":"event","scope":"*","actions":["emit"]}]`))

	_, err := f.lifecycle.Disable("tenant-a", ext.ID)
	require.NoError(t, err)
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "event", "emit", "", ""))

	// 行保留（审计可查）
	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.False(t, grants[0].IsActive)
	require.Nil(t, grants[0].RevokedAt)

	// 重新启用恢复
	_, err = f.lifecycle.Enable("tenant-a", ext.ID)
	require.NoError(t, err)
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "event", "emit", "", ""))
}

func TestAuthzUninstallKeepsGrantsPurgeDeletes(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.uninst", "1.0.0",
		`[{"permission":"storage","scope":"*","actions":["read"]}]`))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "storage", "read", "", ""))

	// 非 purge 卸载：授权行保留但失效，审计保留
	_, err := f.lifecycle.Uninstall("tenant-a", ext.ID, false, false)
	require.NoError(t, err)
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "storage", "read", "", ""))
	var grantCount, auditCount int64
	require.NoError(t, f.db.Model(&extension.Grant{}).Where("tenant_id=? AND extension_name=?", "tenant-a", ext.Name).Count(&grantCount).Error)
	require.NoError(t, f.db.Model(&extension.AccessAudit{}).Where("tenant_id=? AND extension_name=?", "tenant-a", ext.Name).Count(&auditCount).Error)
	require.EqualValues(t, 1, grantCount)
	require.Greater(t, auditCount, int64(0))
}

func TestAuthzPurgeDeletesGrantsAndAudit(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.purge", "1.0.0",
		`[{"permission":"storage","scope":"*","actions":["read"]}]`))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "storage", "read", "", ""))

	_, err := f.lifecycle.Uninstall("tenant-a", ext.ID, false, true)
	require.NoError(t, err)
	var grantCount, auditCount int64
	require.NoError(t, f.db.Model(&extension.Grant{}).Count(&grantCount).Error)
	require.NoError(t, f.db.Model(&extension.AccessAudit{}).Count(&auditCount).Error)
	require.Zero(t, grantCount)
	require.Zero(t, auditCount)
}

func TestAuthzUpgradeResyncsGrants(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.upgrade", "1.0.0",
		`[{"permission":"tool","scope":"*","actions":["call"]}]`))
	// 注册 2.0.0：把 tool/call 换成 workflow/run
	_, err := f.registry.Register("tenant-a", RegisterExtensionInput{Manifest: []byte(authzManifest("io.zerone.upgrade", "2.0.0",
		`[{"permission":"workflow","scope":"*","actions":["run"]}]`)), Source: extension.SourceUpload})
	require.NoError(t, err)
	_, err = f.lifecycle.Upgrade("tenant-a", ext.ID, "2.0.0")
	require.NoError(t, err)

	// 旧权限撤销、新权限生效
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "tool", "call", "", ""))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "workflow", "run", "", ""))

	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 2) // 一行已撤销 + 一行生效
}

func TestAuthzAuditWritesAllowedAndDenied(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.audit", "1.0.0",
		`[{"permission":"model","scope":"*","actions":["invoke"]}]`))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "model", "invoke", "gpt-x", "10.1.2.3"))
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "model", "delete", "", "10.1.2.3"))

	page, err := f.authz.ListAudit("tenant-a", ext.ID, AuditFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Total)
	// 倒序：最近一条在前
	require.False(t, page.Items[0].Allowed)
	require.Equal(t, "delete", page.Items[0].Action)
	require.Equal(t, "10.1.2.3", page.Items[0].IP)
	require.NotEmpty(t, page.Items[0].DeniedReason)
	require.True(t, page.Items[1].Allowed)
	require.Equal(t, "invoke", page.Items[1].Action)
	require.Equal(t, "gpt-x", page.Items[1].Resource)
	require.Empty(t, page.Items[1].DeniedReason)

	// allowed 过滤
	onlyAllowed := true
	allowedPage, err := f.authz.ListAudit("tenant-a", ext.ID, AuditFilter{Page: 1, PageSize: 10, Allowed: &onlyAllowed})
	require.NoError(t, err)
	require.EqualValues(t, 1, allowedPage.Total)
	require.True(t, allowedPage.Items[0].Allowed)
}

func TestAuthzCrossTenantIsolation(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.iso", "1.0.0",
		`[{"permission":"state","scope":"*","actions":["read"]}]`))

	// tenant-b 的扩展同名注册：授权完全隔离
	resB, err := f.registry.Register("tenant-b", RegisterExtensionInput{Manifest: []byte(authzManifest("io.zerone.iso", "1.0.0", `[]`)), Source: extension.SourceUpload})
	require.NoError(t, err)
	_, err = f.lifecycle.Install("tenant-b", resB.Extension.ID, "1.0.0", "admin-b")
	require.NoError(t, err)

	// tenant-a 授权对 tenant-b 不可见：tenant-b 下同名扩展无授权
	require.False(t, f.authz.Enforce("tenant-b", ext.Name, "state", "read", "", ""))
	// tenant-a 自身仍生效
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "state", "read", "", ""))

	// grants 列表租户隔离
	grantsA, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grantsA, 1)
	_, err = f.authz.ListGrants("tenant-b", ext.ID) // ext.ID 属于 tenant-a → 404
	require.Error(t, err)
}

func TestAuthzResourceScopedGrant(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.rsrc", "1.0.0",
		`[{"permission":"agent","scope":"*","actions":["read"],"resource":"agent-a"}]`))
	// 资源匹配放行
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "agent", "read", "agent-a", ""))
	// 资源不匹配拒绝
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "agent", "read", "agent-b", ""))
	// 资源留空 = 未指定资源：grant 有 resource 时仍要求匹配
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "agent", "read", "", ""))
}

func TestAuthzWildcardAction(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.wild", "1.0.0",
		`[{"permission":"ui","scope":"*","actions":["*"]}]`))
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "ui", "anything", "", ""))
}

func TestAuthzSyncIdempotent(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.idem", "1.0.0",
		`[{"permission":"state","scope":"s","actions":["read"]}]`))
	// 重复同步同 manifest：行数不变
	require.NoError(t, f.authz.SyncGrantsFromManifest("tenant-a", ext.Name,
		authzManifest("io.zerone.idem", "1.0.0", `[{"permission":"state","scope":"s","actions":["read"]}]`), "1.0.0", "admin"))
	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
}

func TestAuthzAuditPagination(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.page", "1.0.0",
		`[{"permission":"state","scope":"*","actions":["read"]}]`))
	for i := 0; i < 25; i++ {
		f.authz.Enforce("tenant-a", ext.Name, "state", "read", "", "")
	}
	page1, err := f.authz.ListAudit("tenant-a", ext.ID, AuditFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 25, page1.Total)
	require.Len(t, page1.Items, 10)
	page3, err := f.authz.ListAudit("tenant-a", ext.ID, AuditFilter{Page: 3, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, page3.Items, 5)
	// 倒序：page1 首条比 page3 末条新
	require.False(t, page1.Items[0].CreatedAt.Before(page3.Items[len(page3.Items)-1].CreatedAt))
	// 时间接近时用 ID  tiebreak：每行 CreatedAt 单调
	require.True(t, page1.Items[0].ID > page3.Items[len(page3.Items)-1].ID || !page1.Items[0].CreatedAt.Before(page3.Items[len(page3.Items)-1].CreatedAt))
}
