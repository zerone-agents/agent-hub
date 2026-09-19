// P1 回归测试：授权同步（syncGrants / SetGrantsActive / purge 删除）纳入
// 生命周期事务后，同步失败必须整体回滚——接口返回错误、installs 状态
// 保持操作前、grants 无变化（杜绝"页面说失败，数据库已经成功"）。
package services

import (
	"errors"
	"testing"

	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"github.com/stretchr/testify/require"
)

var errSyncGrantsInjected = errors.New("注入的授权同步失败")

func grantCountFor(t *testing.T, f *authzFixture, extName string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, f.db.Model(&extension.Grant{}).
		Where("tenant_id=? AND extension_name=?", "tenant-a", extName).Count(&n).Error)
	return n
}

func installRow(t *testing.T, f *authzFixture, extID uint64) *extension.Install {
	t.Helper()
	inst, err := findInstall(f.db, "tenant-a", extID)
	require.NoError(t, err)
	return inst
}

func TestLifecycleInstallSyncGrantsFailureRollsBack(t *testing.T) {
	f := newAuthzFixture(t)
	res, err := f.registry.Register("tenant-a", RegisterExtensionInput{Manifest: []byte(authzManifest("io.zerone.tx-install", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`)), Source: extension.SourceUpload})
	require.NoError(t, err)

	f.lifecycle.syncGrantsHook = func() error { return errSyncGrantsInjected }
	_, err = f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.ErrorIs(t, err, errSyncGrantsInjected)

	// 原子性：installs 无记录、grants 无残留、状态 Schema 一并回滚。
	require.Nil(t, installRow(t, f, res.Extension.ID))
	require.Zero(t, grantCountFor(t, f, res.Extension.Name))
	var schemaCount int64
	require.NoError(t, f.db.Model(&rundomain.StateSchema{}).
		Where("tenant_id=? AND namespace=?", "tenant-a", res.Extension.Name).Count(&schemaCount).Error)
	require.Zero(t, schemaCount)

	// 故障解除后重试应成功（事务未留下半完成状态阻塞重试）。
	f.lifecycle.syncGrantsHook = nil
	instRes, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)
	require.False(t, instRes.Idempotent)
	require.Equal(t, int64(1), grantCountFor(t, f, res.Extension.Name))
}

func TestLifecycleDisableSyncGrantsFailureRollsBack(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.tx-disable", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	require.Equal(t, extension.InstallStatusEnabled, installRow(t, f, ext.ID).Status)

	f.lifecycle.syncGrantsHook = func() error { return errSyncGrantsInjected }
	_, err := f.lifecycle.Disable("tenant-a", ext.ID)
	require.ErrorIs(t, err, errSyncGrantsInjected)

	// installs 仍是启用态，grants 仍生效（未出现"状态已改、接口报失败"）。
	require.Equal(t, extension.InstallStatusEnabled, installRow(t, f, ext.ID).Status)
	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.NotEmpty(t, grants)
	for _, g := range grants {
		require.True(t, g.IsActive)
	}
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "state", "read", "", ""))
}

func TestLifecycleUpgradeSyncGrantsFailureRollsBack(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.tx-upgrade", "1.0.0",
		`[{"permission":"tool","scope":"*","actions":["call"]}]`))
	_, err := f.registry.Register("tenant-a", RegisterExtensionInput{Manifest: []byte(authzManifest("io.zerone.tx-upgrade", "2.0.0",
		`[{"permission":"workflow","scope":"*","actions":["run"]}]`)), Source: extension.SourceUpload})
	require.NoError(t, err)

	f.lifecycle.syncGrantsHook = func() error { return errSyncGrantsInjected }
	_, err = f.lifecycle.Upgrade("tenant-a", ext.ID, "2.0.0")
	require.ErrorIs(t, err, errSyncGrantsInjected)

	// 版本仍指向 1.0.0，旧权限未被动过（无新 grants、无撤销）。
	require.Equal(t, "1.0.0", installRow(t, f, ext.ID).Version)
	grants, err := f.authz.ListGrants("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "tool", grants[0].Permission)
	require.True(t, grants[0].IsActive)
}

func TestLifecycleRollbackSyncGrantsFailureRollsBack(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.tx-rollback", "1.0.0",
		`[{"permission":"tool","scope":"*","actions":["call"]}]`))
	_, err := f.registry.Register("tenant-a", RegisterExtensionInput{Manifest: []byte(authzManifest("io.zerone.tx-rollback", "2.0.0",
		`[{"permission":"workflow","scope":"*","actions":["run"]}]`)), Source: extension.SourceUpload})
	require.NoError(t, err)
	_, err = f.lifecycle.Upgrade("tenant-a", ext.ID, "2.0.0")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", installRow(t, f, ext.ID).Version)

	f.lifecycle.syncGrantsHook = func() error { return errSyncGrantsInjected }
	_, err = f.lifecycle.Rollback("tenant-a", ext.ID, "")
	require.ErrorIs(t, err, errSyncGrantsInjected)

	// 版本仍是 2.0.0，grants 保持 2.0.0 同步后的形态（tool 已撤销、workflow 生效）。
	require.Equal(t, "2.0.0", installRow(t, f, ext.ID).Version)
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "workflow", "run", "", ""))
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "tool", "call", "", ""))
}

func TestLifecycleUninstallSyncGrantsFailureRollsBack(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.tx-uninstall", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))

	f.lifecycle.syncGrantsHook = func() error { return errSyncGrantsInjected }
	_, err := f.lifecycle.Uninstall("tenant-a", ext.ID, false, false)
	require.ErrorIs(t, err, errSyncGrantsInjected)

	// installs 行仍在且启用，grants 全部保持生效。
	require.NotNil(t, installRow(t, f, ext.ID))
	require.Equal(t, extension.InstallStatusEnabled, installRow(t, f, ext.ID).Status)
	require.True(t, f.authz.Enforce("tenant-a", ext.Name, "state", "read", "", ""))
}

func TestLifecyclePurgeSyncGrantsFailureRollsBack(t *testing.T) {
	f := newAuthzFixture(t)
	ext := f.registerAndInstall(t, "tenant-a", authzManifest("io.zerone.tx-purge", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))

	f.lifecycle.syncGrantsHook = func() error { return errSyncGrantsInjected }
	_, err := f.lifecycle.Uninstall("tenant-a", ext.ID, false, true)
	require.ErrorIs(t, err, errSyncGrantsInjected)

	// 扩展行、版本行、installs、grants 全部原样保留。
	require.NotNil(t, installRow(t, f, ext.ID))
	require.Equal(t, int64(1), grantCountFor(t, f, ext.Name))
	var extCount, verCount int64
	require.NoError(t, f.db.Model(&extension.Extension{}).Where("id=?", ext.ID).Count(&extCount).Error)
	require.NoError(t, f.db.Model(&extension.Version{}).Where("extension_id=?", ext.ID).Count(&verCount).Error)
	require.Equal(t, int64(1), extCount)
	require.Equal(t, int64(1), verCount)
}
