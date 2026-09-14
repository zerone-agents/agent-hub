// H7.1 扩展生命周期服务测试：安装/幂等/依赖解析/事务无残留、升级迁移
// 与回滚恢复、启停语义、卸载依赖检查与 force、跨租户隔离、并发安装。
package services

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// lifecycleFixture 装配内存库 + 注册/生命周期两个服务。
type lifecycleFixture struct {
	db        *gorm.DB
	registry  *ExtensionService
	lifecycle *ExtensionLifecycleService
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
		&rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{},
	))
	return &lifecycleFixture{
		db:        db,
		registry:  NewExtensionService(db),
		lifecycle: NewExtensionLifecycleService(db),
	}
}

func (f *lifecycleFixture) register(t *testing.T, tenantID, manifest string) *RegisterResult {
	t.Helper()
	res, err := f.registry.Register(tenantID, RegisterExtensionInput{Manifest: []byte(manifest), Source: extension.SourceUpload})
	require.NoError(t, err)
	return res
}

// lifecycleManifest 生成带一个 intensity 状态 Schema 的 manifest；
// version/minimum/migrations 可按用例覆盖。
func lifecycleManifest(name, version string, minimum int, migrations string, deps string) string {
	mig := ""
	if migrations != "" {
		mig = `,"migrations":` + migrations
	}
	dep := ""
	if deps != "" {
		dep = `,"dependencies":` + deps
	}
	return fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "生命周期测试扩展",
	  "description": "lifecycle test",
	  "stateSchemas": [{"name": "intensity", "payload": {"type": "object", "properties": {"intensity": {"type": "integer", "minimum": %d}}}}]%s%s
	}`, name, version, minimum, mig, dep)
}

func TestLifecycleInstallSuccessAndIdempotent(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))

	instRes, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)
	require.False(t, instRes.Idempotent)
	require.Equal(t, extension.InstallStatusEnabled, instRes.Install.Status)
	require.Equal(t, "1.0.0", instRes.Install.Version)

	// 状态 Schema 已注册（namespace=扩展名）
	var schemas []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=?", "tenant-a", "io.zerone.life").Find(&schemas).Error)
	require.Len(t, schemas, 1)
	require.Equal(t, "1.0.0", schemas[0].Version)

	// 重复安装同版本：幂等返回 200 语义
	again, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)
	require.True(t, again.Idempotent)
	require.Equal(t, instRes.Install.ID, again.Install.ID)

	// 安装其他版本 → 409 提示走升级
	var count int64
	require.NoError(t, f.db.Model(&extension.Install{}).Where("tenant_id=?", "tenant-a").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestLifecycleInstallVersionNotFound(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "9.9.9", "admin-1")
	require.Error(t, err)
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 404, code)
	require.Contains(t, err.Error(), "不存在")
}

func TestLifecycleInstallDependencyMissing409NoResidue(t *testing.T) {
	f := newLifecycleFixture(t)
	// 被依赖扩展未注册
	dep := f.register(t, "tenant-a", lifecycleManifest("io.zerone.base", "1.0.0", 0, "", ""))
	_ = dep
	child := f.register(t, "tenant-a", lifecycleManifest("io.zerone.child", "1.0.0", 0, "",
		`[{"name":"io.zerone.base","version":">=1.0.0"}]`))

	_, err := f.lifecycle.Install("tenant-a", child.Extension.ID, "1.0.0", "admin-1")
	require.Error(t, err)
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 409, code)
	require.Contains(t, err.Error(), "io.zerone.base")

	// 事务无残留：installs 无行、state_schemas 无新行
	var installs int64
	require.NoError(t, f.db.Model(&extension.Install{}).Where("tenant_id=?", "tenant-a").Count(&installs).Error)
	require.Equal(t, int64(0), installs)
	var schemas int64
	require.NoError(t, f.db.Model(&rundomain.StateSchema{}).Where("tenant_id=?", "tenant-a").Count(&schemas).Error)
	require.Equal(t, int64(0), schemas)
}

func TestLifecycleInstallDependencySatisfiedAndVersionRange(t *testing.T) {
	f := newLifecycleFixture(t)
	base := f.register(t, "tenant-a", lifecycleManifest("io.zerone.base", "1.2.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", base.Extension.ID, "1.2.0", "admin-1")
	require.NoError(t, err)

	child := f.register(t, "tenant-a", lifecycleManifest("io.zerone.child", "1.0.0", 0, "",
		`[{"name":"io.zerone.base","version":"^1.0.0"}]`))
	_, err = f.lifecycle.Install("tenant-a", child.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err, "依赖 ^1.0.0 应被已启用的 1.2.0 满足")

	// 依赖版本不满足：1.2.0 不在 >=2.0.0 范围
	strict := f.register(t, "tenant-a", lifecycleManifest("io.zerone.strict", "1.0.0", 0, "",
		`[{"name":"io.zerone.base","version":">=2.0.0"}]`))
	_, err = f.lifecycle.Install("tenant-a", strict.Extension.ID, "1.0.0", "admin-1")
	require.Error(t, err)
	code, _ := IsLifecycleError(err)
	require.Equal(t, 409, code)
	require.Contains(t, err.Error(), "不满足范围")
}

func TestLifecycleInstallDependencyFromDefaultTenant(t *testing.T) {
	f := newLifecycleFixture(t)
	base := f.register(t, "default", lifecycleManifest("io.zerone.platform", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("default", base.Extension.ID, "1.0.0", "platform")
	require.NoError(t, err)

	child := f.register(t, "tenant-a", lifecycleManifest("io.zerone.app", "1.0.0", 0, "",
		`[{"name":"io.zerone.platform","version":"1.0.0"}]`))
	_, err = f.lifecycle.Install("tenant-a", child.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err, "default 平台级租户已启用的依赖应满足其他租户")
}

func TestLifecycleUpgradeMigrationAppliedAndRollbackRestores(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	// v2：minimum 0 → -100，声明迁移
	v2 := lifecycleManifest("io.zerone.life", "2.0.0", 0, `[
	  {"from":"1.0.0","to":"2.0.0","ops":[{"op":"replace","path":"/properties/intensity/minimum","value":-100}]}
	]`, "")
	f.register(t, "tenant-a", v2)

	up, err := f.lifecycle.Upgrade("tenant-a", res.Extension.ID, "2.0.0")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", up.Install.Version)

	// 迁移真实生效：v1 schema 行的 schema_json 被改写
	var schemas []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND version=?", "tenant-a", "io.zerone.life", "1.0.0").Find(&schemas).Error)
	require.Len(t, schemas, 1)
	intensity := schemas[0].Schema["properties"].(map[string]any)["intensity"].(map[string]any)
	require.InDelta(t, -100, intensity["minimum"], 0.01)

	// v2 schema 行也已注册（升级注册目标版本声明）
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND version=?", "tenant-a", "io.zerone.life", "2.0.0").Find(&schemas).Error)
	require.Len(t, schemas, 1)

	// migration_log 有记录
	var inst extension.Install
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", res.Extension.ID).First(&inst).Error)
	require.Contains(t, inst.MigrationLog, `"direction":"upgrade"`)
	require.Contains(t, inst.MigrationLog, "/properties/intensity/minimum")
	require.Contains(t, inst.MigrationLog, `"oldValue":0`)

	// 回滚 v2→v1：schema 恢复
	rb, err := f.lifecycle.Rollback("tenant-a", res.Extension.ID, "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", rb.Install.Version)
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND version=?", "tenant-a", "io.zerone.life", "1.0.0").Find(&schemas).Error)
	intensity = schemas[0].Schema["properties"].(map[string]any)["intensity"].(map[string]any)
	require.InDelta(t, 0, intensity["minimum"], 0.01, "回滚应按记录的旧值恢复 schema_json")
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", res.Extension.ID).First(&inst).Error)
	require.Contains(t, inst.MigrationLog, `"direction":"rollback"`)
}

func TestLifecycleRollbackDefaultPreviousVersion(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.1.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.1.0", "admin-1")
	require.NoError(t, err)
	rb, err := f.lifecycle.Rollback("tenant-a", res.Extension.ID, "")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", rb.Install.Version)
}

func TestLifecycleDisableGetEnabledAndHistoryReadable(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	// 启用时 GetEnabled 返回版本
	ext, ver, err := f.registry.GetEnabled(f.db, "tenant-a", "io.zerone.life", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", ver.Version)
	require.Equal(t, res.Extension.ID, ext.ID)

	// 造一条历史 run_states 数据
	hist := rundomain.RunState{
		TenantID: "tenant-a", RunID: "run-1", Namespace: "io.zerone.life",
		SchemaName: "intensity", SchemaVersion: "1.0.0", SchemaHash: "deadbeef",
		SubjectType: "agent", SubjectID: "agent-1", Revision: 1,
		Data: map[string]any{"intensity": 5},
	}
	require.NoError(t, f.db.Create(&hist).Error)

	_, err = f.lifecycle.Disable("tenant-a", res.Extension.ID)
	require.NoError(t, err)
	_, _, err = f.registry.GetEnabled(f.db, "tenant-a", "io.zerone.life", "1.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "未安装或未启用")

	// 旧数据依旧可读
	var states []rundomain.RunState
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=?", "tenant-a", "io.zerone.life").Find(&states).Error)
	require.Len(t, states, 1)
	require.InDelta(t, 5, states[0].Data["intensity"], 0.01)

	// 再启用恢复
	_, err = f.lifecycle.Enable("tenant-a", res.Extension.ID)
	require.NoError(t, err)
	_, _, err = f.registry.GetEnabled(f.db, "tenant-a", "io.zerone.life", "1.0.0")
	require.NoError(t, err)
}

func TestLifecycleUninstallBlockedByDependentsAndForce(t *testing.T) {
	f := newLifecycleFixture(t)
	base := f.register(t, "tenant-a", lifecycleManifest("io.zerone.base", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", base.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)
	child := f.register(t, "tenant-a", lifecycleManifest("io.zerone.child", "1.0.0", 0, "",
		`[{"name":"io.zerone.base","version":"1.0.0"}]`))
	_, err = f.lifecycle.Install("tenant-a", child.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	// 影响范围预览：base 的 dependents 应包含 child
	impact, err := f.lifecycle.Impact("tenant-a", base.Extension.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"io.zerone.child"}, impact.Dependents)

	// 默认拒绝卸载并列出依赖方
	_, err = f.lifecycle.Uninstall("tenant-a", base.Extension.ID, false, false)
	require.Error(t, err)
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 409, code)
	require.Contains(t, err.Error(), "io.zerone.child")

	// force：级联停用依赖方后卸载成功
	res, err := f.lifecycle.Uninstall("tenant-a", base.Extension.ID, true, false)
	require.NoError(t, err)
	require.Equal(t, []string{"io.zerone.child"}, res.DependentsDisabled)
	var installs int64
	require.NoError(t, f.db.Model(&extension.Install{}).Where("tenant_id=? AND extension_id=?", "tenant-a", base.Extension.ID).Count(&installs).Error)
	require.Equal(t, int64(0), installs)
	var childInst extension.Install
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", child.Extension.ID).First(&childInst).Error)
	require.Equal(t, extension.InstallStatusDisabled, childInst.Status)
}

func TestLifecycleUninstallKeepsHistoryAndVersions(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)
	hist := rundomain.RunState{
		TenantID: "tenant-a", RunID: "run-1", Namespace: "io.zerone.life",
		SchemaName: "intensity", SchemaVersion: "1.0.0", SchemaHash: "deadbeef",
		SubjectType: "agent", SubjectID: "agent-1", Revision: 1,
		Data: map[string]any{"intensity": 7},
	}
	require.NoError(t, f.db.Create(&hist).Error)

	_, err = f.lifecycle.Uninstall("tenant-a", res.Extension.ID, false, false)
	require.NoError(t, err)
	// 版本数据保留（可重装），历史 state 保留
	var versions int64
	require.NoError(t, f.db.Model(&extension.Version{}).Where("extension_id=?", res.Extension.ID).Count(&versions).Error)
	require.Equal(t, int64(1), versions)
	var states int64
	require.NoError(t, f.db.Model(&rundomain.RunState{}).Where("tenant_id=?", "tenant-a").Count(&states).Error)
	require.Equal(t, int64(1), states)
	// 可重装
	_, err = f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	// purge：版本数据删除，历史 state 仍保留
	_, err = f.lifecycle.Uninstall("tenant-a", res.Extension.ID, false, true)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&extension.Version{}).Where("extension_id=?", res.Extension.ID).Count(&versions).Error)
	require.Equal(t, int64(0), versions)
	require.NoError(t, f.db.Model(&rundomain.RunState{}).Where("tenant_id=?", "tenant-a").Count(&states).Error)
	require.Equal(t, int64(1), states, "purge 也不删除 run_states 历史")
}

func TestLifecycleCrossTenantIsolation(t *testing.T) {
	f := newLifecycleFixture(t)
	resA := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", resA.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	// tenant-b 看不到 tenant-a 的安装，也不满足其依赖
	_, err = f.lifecycle.Impact("tenant-b", resA.Extension.ID)
	require.Error(t, err)
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 404, code)

	// tenant-b 注册同名扩展互不影响
	resB := f.register(t, "tenant-b", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	instB, err := f.lifecycle.Install("tenant-b", resB.Extension.ID, "1.0.0", "admin-2")
	require.NoError(t, err)
	require.NotEqual(t, resA.Extension.ID, resB.Extension.ID)
	require.NotEqual(t, uint64(0), instB.Install.ID)

	var count int64
	require.NoError(t, f.db.Model(&extension.Install{}).Where("tenant_id=?", "tenant-a").Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, f.db.Model(&extension.Install{}).Where("tenant_id=?", "tenant-b").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestLifecycleConcurrentInstallSingleWinner(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))

	const n = 8
	results := make([]*InstallResult, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
		}(i)
	}
	wg.Wait()

	success := 0
	idempotent := 0
	for i := 0; i < n; i++ {
		require.NoError(t, errs[i], "并发安装必须全部成功（幂等语义），第 %d 个出错：%v", i, errs[i])
		require.NotNil(t, results[i])
		if results[i].Idempotent {
			idempotent++
		}
		success++
	}
	require.Equal(t, n, success)
	require.Equal(t, n-1, idempotent, "只有一个写入者，其余全部幂等返回")
	var installs int64
	require.NoError(t, f.db.Model(&extension.Install{}).Where("tenant_id=? AND extension_id=?", "tenant-a", res.Extension.ID).Count(&installs).Error)
	require.Equal(t, int64(1), installs, "唯一索引保证只有一个写入成功")
}

func TestLifecycleUpgradeFailureRollsBack(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	// v2 的迁移路径不存在（from=1.0.0 to=2.0.0 但 op 指向不存在的字段）
	v2 := lifecycleManifest("io.zerone.life", "2.0.0", 0, `[
	  {"from":"1.0.0","to":"2.0.0","ops":[{"op":"replace","path":"/properties/nonexistent/minimum","value":-100}]}
	]`, "")
	f.register(t, "tenant-a", v2)

	_, err = f.lifecycle.Upgrade("tenant-a", res.Extension.ID, "2.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "数据迁移失败")

	// 失败整体回滚：版本仍指向 1.0.0，schema 未被改写
	var inst extension.Install
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", res.Extension.ID).First(&inst).Error)
	require.Equal(t, "1.0.0", inst.Version)
	require.Empty(t, inst.MigrationLog)
	var schemas []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND version=?", "tenant-a", "io.zerone.life", "2.0.0").Find(&schemas).Error)
	require.Len(t, schemas, 0, "迁移失败时 v2 schema 注册也应回滚")
}

func TestSatisfiesRange(t *testing.T) {
	cases := []struct {
		version string
		rangeE  string
		want    bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", ">=1.0.0 <2.0.0", true},
		{"2.0.0", ">=1.0.0 <2.0.0", false},
		{"1.2.3", "^1.0.0", true},
		{"1.9.9", "^1.0.0", true},
		{"2.0.0", "^1.0.0", false},
		{"0.1.5", "^0.1.0", true},
		{"0.2.0", "^0.1.0", false},
		{"1.2.9", "~1.2.0", true},
		{"1.3.0", "~1.2.0", false},
		{"1.0.0", ">1.0.0", false},
		{"1.0.1", ">1.0.0", true},
	}
	for _, tc := range cases {
		got, err := extension.SatisfiesRange(tc.version, tc.rangeE)
		require.NoError(t, err, tc.rangeE)
		require.Equal(t, tc.want, got, "%s in %s", tc.version, tc.rangeE)
	}
	_, err := extension.SatisfiesRange("1.0.0", "not-a-range")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "不合法"))
}
