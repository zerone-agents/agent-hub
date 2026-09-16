// 内置人物能力扩展种子与 PersonaCapabilityGate 的测试（H7 P1）。
package services

import (
	"testing"

	"control-panel/internal/domain/extension"
	"control-panel/internal/extensionmanifest"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestEnsureBuiltinPersonaPacks_Idempotent：重复种子只产生一套记录，
// manifest 全部通过严格校验，默认状态 = 已安装 + 已启用。
func TestEnsureBuiltinPersonaPacks_Idempotent(t *testing.T) {
	f := newLifecycleFixture(t)

	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())

	var exts []extension.Extension
	require.NoError(t, f.db.Where("tenant_id=?", "default").Find(&exts).Error)
	require.Len(t, exts, len(builtinPersonaPacks))
	for _, ext := range exts {
		require.Equal(t, extension.SourceSeed, ext.Source)
		require.Equal(t, extension.StatusActive, ext.Status)
		require.NotContains(t, ext.DisplayName, "Speeding")

		var versions []extension.Version
		require.NoError(t, f.db.Where("extension_id=?", ext.ID).Find(&versions).Error)
		require.Len(t, versions, 1)
		// manifest 必须通过 extensionmanifest 严格校验。
		manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(versions[0].Manifest))
		require.Empty(t, errs)
		require.Equal(t, ext.Name, manifest.Name)

		var installs []extension.Install
		require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "default", ext.ID).Find(&installs).Error)
		require.Len(t, installs, 1)
		require.Equal(t, extension.InstallStatusEnabled, installs[0].Status)
		require.Equal(t, builtinPersonaPackExtensionVersion, installs[0].Version)
	}

	// 管理员的启停状态不被重复种子覆盖：停用后再种子仍保持停用。
	emotionExt := findBuiltinExtension(t, f, "io.zerone.emotion")
	_, err := f.lifecycle.Disable("default", emotionExt.ID)
	require.NoError(t, err)
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	inst, err := findInstall(f.db, "default", emotionExt.ID)
	require.NoError(t, err)
	require.NotNil(t, inst)
	require.Equal(t, extension.InstallStatusDisabled, inst.Status)
}

// TestBuiltinPersonaPackUninstallDoesNotResurrect 回归 P1：内置扩展的"卸载"
// 必须落成"停用"而非物理删除安装行。
//
// 若物理删除，启动期 EnsureBuiltinPersonaPacks 会发现安装行不存在并按
// InstallStatusEnabled 重建，于是管理员的卸载动作在下次重启后被静默撤销
// （页面显示已卸载、重启后又变回启用态），与 H7"可停用/卸载"的目标矛盾。
func TestBuiltinPersonaPackUninstallDoesNotResurrect(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())

	ext := findBuiltinExtension(t, f, "io.zerone.emotion")

	// 卸载内置扩展：转为停用，安装行保留。
	res, err := f.lifecycle.Uninstall("default", ext.ID, false, false)
	require.NoError(t, err)
	require.True(t, res.BuiltinDisableOnly, "内置扩展卸载应标记为仅停用")
	inst, err := findInstall(f.db, "default", ext.ID)
	require.NoError(t, err)
	require.NotNil(t, inst, "安装行必须保留，否则启动期种子会把它重新装回来")
	require.Equal(t, extension.InstallStatusDisabled, inst.Status)

	// 重启（再次种子）：仍保持停用，不被静默恢复。
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	inst, err = findInstall(f.db, "default", ext.ID)
	require.NoError(t, err)
	require.NotNil(t, inst)
	require.Equal(t, extension.InstallStatusDisabled, inst.Status, "重启后不得自动恢复启用")

	// purge（彻底清除）对内置扩展应被拒绝：否则扩展行被删掉后种子会重建全套。
	_, err = f.lifecycle.Uninstall("default", ext.ID, false, true)
	require.Error(t, err, "内置扩展不支持 purge 彻底卸载")
}

func findBuiltinExtension(t *testing.T, f *lifecycleFixture, name string) *extension.Extension {
	t.Helper()
	var ext extension.Extension
	require.NoError(t, f.db.Where("tenant_id=? AND name=?", "default", name).First(&ext).Error)
	return &ext
}

// TestEnsureBuiltinPersonaPacks_SyncsGrants 回归 P0：种子必须把四个内置扩展
// manifest 声明的权限真正同步成 grants 行，并且 Enforce 能据此放行。
//
// 这条路径此前在生产接线里是失效的：main.go 中种子跑在 SetAuthzService 之前，
// 而 syncGrantsWithTx 遇到 authz==nil 直接返回 nil（静默 no-op），于是四个内置
// 扩展永远没有 grants 行。旧测试基座本身不接 authz，所以"授权同步与种子同事务"
// 这句话从未被任何用例验证过——本用例把 authz 接上再断言，才能证伪。
func TestEnsureBuiltinPersonaPacks_SyncsGrants(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, f.db.AutoMigrate(&extension.Grant{}, &extension.AccessAudit{}))
	authz := NewExtensionAuthzService(f.db)
	f.lifecycle.SetAuthzService(authz)

	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())

	for _, pack := range builtinPersonaPacks {
		ext := findBuiltinExtension(t, f, pack.Name)
		grants, err := authz.ListGrants("default", ext.ID)
		require.NoError(t, err)
		require.NotEmpty(t, grants, "内置扩展 %s 必须有 grants 行", pack.Name)
		require.True(t, authz.Enforce("default", pack.Name, "state", "read", "", "127.0.0.1"),
			"%s 的 state/read 应被授权放行", pack.Name)
	}

	// 幂等：重复种子不产生重复 grants。
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	ext := findBuiltinExtension(t, f, "io.zerone.emotion")
	grants, err := authz.ListGrants("default", ext.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)

	// 停用后 Enforce 立即拒绝（默认拒绝语义，与中间件一致）。
	_, err = f.lifecycle.Disable("default", ext.ID)
	require.NoError(t, err)
	require.False(t, authz.Enforce("default", ext.Name, "state", "read", "", "127.0.0.1"),
		"停用扩展后不应再放行")
}

// TestPersonaCapabilityGate_EnableDisable：停用/启用经生命周期 API 立即
// 反映到 gate；未知 pack 与未种子场景返回 false。
func TestPersonaCapabilityGate_EnableDisable(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	gate := NewPersonaCapabilityGate(f.db)

	for _, pack := range []string{PersonaPackEmotion, PersonaPackBelief, PersonaPackMemory, PersonaPackRelationshipDynamics} {
		require.True(t, gate.Enabled("default", pack), "pack %s 默认应启用", pack)
	}

	// 停用 emotion：gate 立即关闭，其余能力不受影响。
	emotionExt := findBuiltinExtension(t, f, "io.zerone.emotion")
	_, err := f.lifecycle.Disable("default", emotionExt.ID)
	require.NoError(t, err)
	require.False(t, gate.Enabled("default", PersonaPackEmotion))
	require.True(t, gate.Enabled("default", PersonaPackMemory))

	// 重新启用：gate 恢复。
	_, err = f.lifecycle.Enable("default", emotionExt.ID)
	require.NoError(t, err)
	require.True(t, gate.Enabled("default", PersonaPackEmotion))

	// 未知 pack 恒 false。
	require.False(t, gate.Enabled("default", "no-such-pack"))
}

// TestPersonaCapabilityGate_TenantFallback：运行时容器租户没有自己的扩展
// 记录时回退到 default 平台级租户；租户自己有记录时以租户为准。
func TestPersonaCapabilityGate_TenantFallback(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, f.lifecycle.EnsureBuiltinPersonaPacks())
	gate := NewPersonaCapabilityGate(f.db)

	// 租户自己没有记录：回退 default，命中平台级的启用状态。
	require.True(t, gate.Enabled("tenant-x", PersonaPackMemory))

	// 平台级停用后，普通租户调用同样被门控（回退 default）。
	memoryExt := findBuiltinExtension(t, f, "io.zerone.memory")
	_, err := f.lifecycle.Disable("default", memoryExt.ID)
	require.NoError(t, err)
	require.False(t, gate.Enabled("tenant-x", PersonaPackMemory), "租户无记录时应回退 default 并命中停用")
	require.True(t, gate.Enabled("tenant-x", PersonaPackEmotion))

	// 租户自己安装了 memory（经注册表 + 生命周期），以租户状态为准。
	res := f.register(t, "tenant-x", lifecycleManifest("io.zerone.memory", "1.0.0", 0, "", ""))
	_, err = f.lifecycle.Install("tenant-x", res.Extension.ID, "1.0.0", "tester")
	require.NoError(t, err)
	require.True(t, gate.Enabled("tenant-x", PersonaPackMemory), "租户自己的启用安装应覆盖 default 停用")
}

// TestPromptComposerRecentMemoryGatedByExtension：主观记忆扩展停用后，
// recent_memory 阶段拿不到记忆（provider 被 gate 短路，合成器跳过阶段）；
// 启用时行为与现状一致。
func TestPromptComposerRecentMemoryGatedByExtension(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	require.NoError(t, db.AutoMigrate(&extension.Extension{}, &extension.Version{}, &extension.Install{}))
	lifecycle := NewExtensionLifecycleService(db)
	require.NoError(t, lifecycle.EnsureBuiltinPersonaPacks())
	gate := NewPersonaCapabilityGate(db)

	called := 0
	provider := gate.GateRecentMemoryProvider(func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		called++
		return []RecentMemoryItem{{Label: "近期记忆", Text: "你记得昨天核验过数据。"}}, nil
	})
	svc.SetRecentMemoryProvider(provider)

	// 默认启用：记忆进入合成结果。
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.NotEqual(t, -1, stageIndex(t, provenanceStages(snapshot), "recent_memory"))
	require.Contains(t, snapshot.RenderedText, "近期记忆")
	require.Equal(t, 1, called)

	// 停用主观记忆扩展：provider 不再被调用，阶段缺席。
	memoryExt := findSeededExtension(t, db, "io.zerone.memory")
	_, err = lifecycle.Disable("default", memoryExt.ID)
	require.NoError(t, err)
	snapshot, err = svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Equal(t, -1, stageIndex(t, provenanceStages(snapshot), "recent_memory"))
	require.NotContains(t, snapshot.RenderedText, "近期记忆")
	require.Equal(t, 1, called, "停用后 provider 不应再被调用")

	// 重新启用：恢复注入。
	_, err = lifecycle.Enable("default", memoryExt.ID)
	require.NoError(t, err)
	snapshot, err = svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.NotEqual(t, -1, stageIndex(t, provenanceStages(snapshot), "recent_memory"))
	require.Equal(t, 2, called)
}

func findSeededExtension(t *testing.T, db *gorm.DB, name string) *extension.Extension {
	t.Helper()
	var ext extension.Extension
	require.NoError(t, db.Where("tenant_id=? AND name=?", "default", name).First(&ext).Error)
	return &ext
}
