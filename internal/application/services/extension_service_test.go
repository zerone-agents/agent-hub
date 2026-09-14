// ExtensionService 应用服务测试：注册→列表→详情→幂等→租户隔离。
package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"control-panel/internal/domain/extension"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newExtensionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&extension.Extension{}, &extension.Version{}))
	return db
}

func testExtensionManifest(name, version string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "测试扩展",
	  "description": "ExtensionService 测试用",
	  "permissions": [{"permission": "state", "scope": "io.zerone.test/*", "actions": ["read"]}],
	  "ui": {"slots": ["sidebar"]},
	  "stateSchemas": [{"name": "profile"}],
	  "events": [{"name": "profile.changed"}]
	}`, name, version))
}

func TestExtensionServiceRegisterListDetailAndIdempotency(t *testing.T) {
	db := newExtensionTestDB(t)
	svc := NewExtensionService(db)

	// 注册
	res, err := svc.Register("tenant-a", RegisterExtensionInput{
		Manifest: testExtensionManifest("io.zerone.alpha", "1.0.0"), Source: "seed", CreatedBy: "admin-1"})
	require.NoError(t, err)
	require.False(t, res.AlreadyExisted)
	require.Equal(t, "io.zerone.alpha", res.Extension.Name)
	require.Equal(t, extension.StatusActive, res.Extension.Status)
	require.Equal(t, "seed", res.Extension.Source)
	require.Len(t, res.Version.ContentHash, 64)

	// 同内容哈希重复注册 → 幂等返回既有版本
	dup, err := svc.Register("tenant-a", RegisterExtensionInput{
		Manifest: testExtensionManifest("io.zerone.alpha", "1.0.0")})
	require.NoError(t, err)
	require.True(t, dup.AlreadyExisted)
	require.Equal(t, res.Version.ID, dup.Version.ID)

	// 同版本不同内容 → 报错
	conflict, err := svc.Register("tenant-a", RegisterExtensionInput{
		Manifest: json.RawMessage(`{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.alpha","version":"1.0.0","displayName":"改","description":"内容不同的同版本"}`)})
	require.Error(t, err)
	require.Nil(t, conflict)
	require.Contains(t, err.Error(), "已存在且内容不一致")

	// 同扩展新版本 → 复用扩展行，新增版本
	v2, err := svc.Register("tenant-a", RegisterExtensionInput{
		Manifest: testExtensionManifest("io.zerone.alpha", "1.1.0"), Changelog: "新增事件"})
	require.NoError(t, err)
	require.Equal(t, res.Extension.ID, v2.Extension.ID)
	require.Equal(t, "1.1.0", v2.Version.Version)
	require.Equal(t, "新增事件", v2.Version.Changelog)

	// 非法 manifest → 中文错误
	_, err = svc.Register("tenant-a", RegisterExtensionInput{Manifest: json.RawMessage(`{"name":"bad"}`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "扩展 manifest 校验失败")

	// 列表 + 过滤 + 分页
	page, err := svc.List("tenant-a", ExtensionListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 2, page.Items[0].VersionCount)
	require.Equal(t, "1.1.0", page.Items[0].LatestVersion)

	filtered, err := svc.List("tenant-a", ExtensionListFilter{Source: "registry"})
	require.NoError(t, err)
	require.EqualValues(t, 0, filtered.Total)

	// 详情：版本列表 + manifest 摘要 + 权限清单
	detail, err := svc.Get("tenant-a", res.Extension.ID)
	require.NoError(t, err)
	require.Len(t, detail.Versions, 2)
	latest := detail.Versions[0]
	require.Equal(t, "1.1.0", latest.Version.Version)
	require.Equal(t, 1, latest.ManifestSummary.StateSchemaCount)
	require.Equal(t, 1, latest.ManifestSummary.EventCount)
	require.Equal(t, []string{"sidebar"}, latest.ManifestSummary.Slots)
	require.Len(t, latest.Permissions, 1)
	require.Equal(t, "state", latest.Permissions[0].Permission)

	// 单版本完整 manifest
	ver, err := svc.GetVersion("tenant-a", res.Extension.ID, "1.0.0")
	require.NoError(t, err)
	require.Contains(t, ver.Manifest, "io.zerone.alpha")
}

func TestExtensionServiceTenantIsolation(t *testing.T) {
	db := newExtensionTestDB(t)
	svc := NewExtensionService(db)

	_, err := svc.Register("tenant-a", RegisterExtensionInput{Manifest: testExtensionManifest("io.zerone.alpha", "1.0.0")})
	require.NoError(t, err)
	_, err = svc.Register("tenant-b", RegisterExtensionInput{Manifest: testExtensionManifest("io.zerone.beta", "2.0.0")})
	require.NoError(t, err)

	pageA, err := svc.List("tenant-a", ExtensionListFilter{})
	require.NoError(t, err)
	require.EqualValues(t, 1, pageA.Total)
	require.Equal(t, "io.zerone.alpha", pageA.Items[0].Name)

	pageB, err := svc.List("tenant-b", ExtensionListFilter{})
	require.NoError(t, err)
	require.Equal(t, "io.zerone.beta", pageB.Items[0].Name)

	// 租户 B 拿不到租户 A 的扩展详情与版本
	_, err = svc.Get("tenant-b", pageA.Items[0].ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = svc.GetVersion("tenant-b", pageA.Items[0].ID, "1.0.0")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// GetEnabled 语义：精确租户 + 名称 + 版本 + active
	enabled, _, err := svc.GetEnabled(db, "tenant-a", "io.zerone.alpha", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "io.zerone.alpha", enabled.Name)
	_, _, err = svc.GetEnabled(db, "tenant-a", "io.zerone.beta", "2.0.0")
	require.Error(t, err)
	_, _, err = svc.GetEnabled(db, "tenant-a", "io.zerone.alpha", "9.9.9")
	require.Error(t, err)
}

func TestExtensionServiceRegisterDefaultsAndValidation(t *testing.T) {
	db := newExtensionTestDB(t)
	svc := NewExtensionService(db)

	// 空 tenant 归一为 default（平台级）
	res, err := svc.Register("", RegisterExtensionInput{Manifest: testExtensionManifest("io.zerone.platform", "1.0.0")})
	require.NoError(t, err)
	require.Equal(t, "default", res.Extension.TenantID)
	require.Equal(t, extension.SourceUpload, res.Extension.Source)

	// 非法 source 报错
	_, err = svc.Register("t", RegisterExtensionInput{Manifest: testExtensionManifest("io.zerone.x", "1.0.0"), Source: "bad"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "source")
}
