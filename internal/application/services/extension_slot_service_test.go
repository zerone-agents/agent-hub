// ExtensionSlotService 测试：插槽聚合/排序/租户隔离/override 隐藏/代理路由解析。
package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/extensionslot"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newSlotTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&extension.Extension{}, &extension.Version{}, &extension.Install{}, &extensionslot.Override{}))
	return db
}

// slotManifest 生成带富插槽声明与 apiRoutes 的 manifest。
func slotManifest(name, version string, slots []map[string]any, apiRoutes string) json.RawMessage {
	slotJSON, _ := json.Marshal(slots)
	return json.RawMessage(fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "插槽测试扩展",
	  "description": "ExtensionSlotService 测试用",
	  "ui": {"slots": %s},
	  "apiRoutes": %s
	}`, name, version, slotJSON, apiRoutes))
}

func statCardSlot(slot string, order int, title string) map[string]any {
	return map[string]any{
		"slot": slot, "component": "stat-card", "title": title, "order": order,
		"data": map[string]any{"value": 42, "description": "测试"},
	}
}

// 注册并启用一个扩展。
func seedEnabledExtension(t *testing.T, svc *ExtensionService, lifecycle *ExtensionLifecycleService, tenantID string, manifest json.RawMessage) *extension.Extension {
	t.Helper()
	res, err := svc.Register(tenantID, RegisterExtensionInput{Manifest: manifest})
	require.NoError(t, err)
	_, err = lifecycle.Install(tenantID, res.Extension.ID, res.Version.Version, "admin-1")
	require.NoError(t, err)
	return res.Extension
}

func TestSlotServiceAggregationSortAndTenantIsolation(t *testing.T) {
	db := newSlotTestDB(t)
	extSvc := NewExtensionService(db)
	lifecycle := NewExtensionLifecycleService(db)
	slotSvc := NewExtensionSlotService(db)

	// 扩展 A：dashboard.card 两个组件（order 乱序）+ 默认隐藏组件
	seedEnabledExtension(t, extSvc, lifecycle, "tenant-a", slotManifest("io.zerone.alpha", "1.0.0", []map[string]any{
		statCardSlot("dashboard.card", 20, "Z 指标"),
		statCardSlot("dashboard.card", 10, "A 指标"),
		{"slot": "dashboard.card", "component": "markdown", "title": "隐藏文档", "visible": false,
			"data": map[string]any{"content": "默认不可见"}},
	}, `[]`))
	// 扩展 B：同插槽 order=15
	seedEnabledExtension(t, extSvc, lifecycle, "tenant-a", slotManifest("io.zerone.beta", "1.0.0", []map[string]any{
		statCardSlot("dashboard.card", 15, "B 指标"),
	}, `[]`))
	// 租户 B 也装了扩展 A（隔离验证）
	seedEnabledExtension(t, extSvc, lifecycle, "tenant-b", slotManifest("io.zerone.alpha", "1.0.0", []map[string]any{
		statCardSlot("dashboard.card", 5, "租户B指标"),
	}, `[]`))

	// 聚合 + 排序（10 A, 15 B, 20 Z）
	items, err := slotSvc.List("tenant-a", "dashboard.card")
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, []string{"A 指标", "B 指标", "Z 指标"}, []string{items[0].Title, items[1].Title, items[2].Title})
	require.Equal(t, "io.zerone.alpha", items[0].ExtensionName)
	// 默认隐藏组件不出现在结果中
	for _, it := range items {
		require.NotEqual(t, "隐藏文档", it.Title)
		require.True(t, it.Visible)
	}

	// 租户隔离：tenant-b 只看到自己的
	itemsB, err := slotSvc.List("tenant-b", "dashboard.card")
	require.NoError(t, err)
	require.Len(t, itemsB, 1)
	require.Equal(t, "租户B指标", itemsB[0].Title)

	// 其它租户看不到任何东西
	itemsC, err := slotSvc.List("tenant-c", "dashboard.card")
	require.NoError(t, err)
	require.Empty(t, itemsC)

	// 按 slot 过滤：只查 sidebar 时无结果
	sidebar, err := slotSvc.List("tenant-a", "sidebar")
	require.NoError(t, err)
	require.Empty(t, sidebar)
}

func TestSlotServiceOverrideHideAndRestore(t *testing.T) {
	db := newSlotTestDB(t)
	extSvc := NewExtensionService(db)
	lifecycle := NewExtensionLifecycleService(db)
	slotSvc := NewExtensionSlotService(db)

	ext := seedEnabledExtension(t, extSvc, lifecycle, "tenant-a", slotManifest("io.zerone.alpha", "1.0.0", []map[string]any{
		statCardSlot("run.detail.tab", 1, "运行指标"),
	}, `[]`))

	// 初始可见
	items, err := slotSvc.List("tenant-a", "run.detail.tab")
	require.NoError(t, err)
	require.Len(t, items, 1)

	// 临时隐藏
	ov, err := slotSvc.SetVisible("tenant-a", ext.Name, "run.detail.tab", "stat-card", false, "admin-1")
	require.NoError(t, err)
	require.False(t, ov.Visible)
	items, err = slotSvc.List("tenant-a", "run.detail.tab")
	require.NoError(t, err)
	require.Empty(t, items)

	// 覆盖 tenant-b（隔离：tenant-a 仍隐藏，tenant-b 无扩展也不可见）
	_, err = slotSvc.SetVisible("tenant-b", ext.Name, "run.detail.tab", "stat-card", true, "admin-2")
	require.NoError(t, err)

	// 恢复可见
	_, err = slotSvc.SetVisible("tenant-a", ext.Name, "run.detail.tab", "stat-card", true, "admin-1")
	require.NoError(t, err)
	items, err = slotSvc.List("tenant-a", "run.detail.tab")
	require.NoError(t, err)
	require.Len(t, items, 1)

	// 参数校验：非法 slot / component
	_, err = slotSvc.SetVisible("tenant-a", ext.Name, "bogus.slot", "stat-card", false, "")
	require.Error(t, err)
	_, err = slotSvc.SetVisible("tenant-a", ext.Name, "sidebar", "evil-widget", false, "")
	require.Error(t, err)

	// 未启用扩展不参与聚合：停用后列表为空
	_, err = lifecycle.Disable("tenant-a", ext.ID)
	require.NoError(t, err)
	items, err = slotSvc.List("tenant-a", "run.detail.tab")
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestSlotServiceResolveAPIRoute(t *testing.T) {
	db := newSlotTestDB(t)
	extSvc := NewExtensionService(db)
	lifecycle := NewExtensionLifecycleService(db)
	slotSvc := NewExtensionSlotService(db)

	upstream := "https://stats.io.zerone.example/v1/metrics"
	ext := seedEnabledExtension(t, extSvc, lifecycle, "tenant-a", slotManifest("io.zerone.alpha", "1.0.0",
		[]map[string]any{
			{"slot": "dashboard.card", "component": "stat-card", "title": "动态指标", "order": 1,
				"dataSource": map[string]any{"path": "/api/v1/extensions/io.zerone.alpha/metrics"}},
		},
		fmt.Sprintf(`[{"method":"GET","path":"/api/v1/extensions/io.zerone.alpha/metrics","upstream":%q}]`, upstream)))

	// 命中声明
	route, err := slotSvc.ResolveAPIRoute("tenant-a", ext.Name, "GET", "/api/v1/extensions/io.zerone.alpha/metrics")
	require.NoError(t, err)
	require.Equal(t, upstream, route.Upstream)

	// 未声明路径 → 拒绝（handler 映射 403）
	_, err = slotSvc.ResolveAPIRoute("tenant-a", ext.Name, "GET", "/api/v1/extensions/io.zerone.alpha/secrets")
	require.Error(t, err)

	// 非 GET 即使路径声明过也拒绝
	_, err = slotSvc.ResolveAPIRoute("tenant-a", ext.Name, "POST", "/api/v1/extensions/io.zerone.alpha/metrics")
	require.Error(t, err)

	// 路径不属于该扩展名 → 拒绝
	_, err = slotSvc.ResolveAPIRoute("tenant-a", ext.Name, "GET", "/api/v1/extensions/io.zerone.beta/metrics")
	require.Error(t, err)

	// 路径穿越 → 拒绝
	_, err = slotSvc.ResolveAPIRoute("tenant-a", ext.Name, "GET", "/api/v1/extensions/io.zerone.alpha/../x")
	require.Error(t, err)

	// 其它租户 → 拒绝
	_, err = slotSvc.ResolveAPIRoute("tenant-b", ext.Name, "GET", "/api/v1/extensions/io.zerone.alpha/metrics")
	require.Error(t, err)

	// 停用后 → 拒绝
	_, err = lifecycle.Disable("tenant-a", ext.ID)
	require.NoError(t, err)
	_, err = slotSvc.ResolveAPIRoute("tenant-a", ext.Name, "GET", "/api/v1/extensions/io.zerone.alpha/metrics")
	require.Error(t, err)
}

// TestSlotServiceDataSourceRequiresDeclaredRoute 校验 manifest 层：
// dataSource.path 必须已在 apiRoutes 声明。
func TestSlotServiceDataSourceRequiresDeclaredRoute(t *testing.T) {
	db := newSlotTestDB(t)
	extSvc := NewExtensionService(db)
	_, err := extSvc.Register("tenant-a", RegisterExtensionInput{Manifest: json.RawMessage(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": "io.zerone.gamma",
	  "version": "1.0.0",
	  "displayName": "坏扩展",
	  "description": "dataSource 指向未声明路径",
	  "ui": {"slots": [{"slot": "dashboard.card", "component": "stat-card", "title": "x",
	    "dataSource": {"path": "/api/v1/extensions/io.zerone.gamma/nope"}}]}
	}`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "apiRoutes")
}
