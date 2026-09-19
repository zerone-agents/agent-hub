// P1 回归测试：
//  1. Upgrade 迁移日志记录真实 from/to（From 污染会导致区间筛选越界）；
//  2. 多 Schema 扩展 v1→v2→v3 回滚时按 schema 绑定恢复，每个 schema_json
//     与 content_hash 逐字节一致，且 v3→v2 不把 v1→v2 的迁移逆序掉。
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"github.com/stretchr/testify/require"
)

// multiSchemaManifest 生成带 intensity + threshold 两个状态 Schema 的 manifest。
func multiSchemaManifest(name, version string, minimum, maximum int, migrations string) string {
	mig := ""
	if migrations != "" {
		mig = `,"migrations":` + migrations
	}
	return fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "多 Schema 迁移测试",
	  "description": "multi schema migration test",
	  "stateSchemas": [
	    {"name": "intensity", "payload": {"type": "object", "properties": {"intensity": {"type": "integer", "minimum": %d}}}},
	    {"name": "threshold", "payload": {"type": "object", "properties": {"threshold": {"type": "integer", "maximum": %d}}}}
	  ]%s
	}`, name, version, minimum, maximum, mig)
}

func snapshotSchemas(t *testing.T, f *lifecycleFixture, tenantID, namespace string) map[uint64][2]string {
	t.Helper()
	var schemas []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=?", tenantID, namespace).Find(&schemas).Error)
	snap := map[uint64][2]string{}
	for _, s := range schemas {
		raw, err := json.Marshal(s.Schema)
		require.NoError(t, err)
		sum := sha256.Sum256(raw)
		snap[s.ID] = [2]string{string(raw), hex.EncodeToString(sum[:])}
	}
	return snap
}

func TestLifecycleUpgradeMigrationLogRecordsRealFromVersion(t *testing.T) {
	f := newLifecycleFixture(t)
	res := f.register(t, "tenant-a", multiSchemaManifest("io.zerone.multi", "1.0.0", 0, 100, ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)

	f.register(t, "tenant-a", multiSchemaManifest("io.zerone.multi", "2.0.0", -100, 100,
		`[{"from":"1.0.0","to":"2.0.0","ops":[{"op":"replace","path":"/properties/intensity/minimum","value":-100}]}]`))
	_, err = f.lifecycle.Upgrade("tenant-a", res.Extension.ID, "2.0.0")
	require.NoError(t, err)

	var inst extension.Install
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", res.Extension.ID).First(&inst).Error)
	var records []migrationLogRecord
	require.NoError(t, json.Unmarshal([]byte(inst.MigrationLog), &records))
	require.Len(t, records, 1)
	require.Equal(t, "upgrade", records[0].Direction)
	require.Equal(t, "1.0.0", records[0].From, "From 必须是升级前真实版本")
	require.Equal(t, "2.0.0", records[0].To)
	require.NotEqual(t, records[0].From, records[0].To)
}

func TestLifecycleRollbackMultiSchemaPerSchemaRestore(t *testing.T) {
	f := newLifecycleFixture(t)
	const ns = "io.zerone.multi"
	res := f.register(t, "tenant-a", multiSchemaManifest(ns, "1.0.0", 0, 100, ""))
	_, err := f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-1")
	require.NoError(t, err)
	original := snapshotSchemas(t, f, "tenant-a", ns)
	require.Len(t, original, 2)

	// v2：intensity.minimum 0 → -100
	f.register(t, "tenant-a", multiSchemaManifest(ns, "2.0.0", -100, 100,
		`[{"from":"1.0.0","to":"2.0.0","ops":[{"op":"replace","path":"/properties/intensity/minimum","value":-100}]}]`))
	_, err = f.lifecycle.Upgrade("tenant-a", res.Extension.ID, "2.0.0")
	require.NoError(t, err)

	// v3：threshold.maximum 100 → 999
	f.register(t, "tenant-a", multiSchemaManifest(ns, "3.0.0", -100, 999,
		`[{"from":"2.0.0","to":"3.0.0","ops":[{"op":"replace","path":"/properties/threshold/maximum","value":999}]}]`))
	_, err = f.lifecycle.Upgrade("tenant-a", res.Extension.ID, "3.0.0")
	require.NoError(t, err)

	// 回滚 v3→v2：只逆序 v2→v3，v1→v2 的迁移必须保留
	rb, err := f.lifecycle.Rollback("tenant-a", res.Extension.ID, "2.0.0")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", rb.Install.Version)

	var schemas []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND name=?", "tenant-a", ns, "intensity").Order("version").Find(&schemas).Error)
	for _, s := range schemas {
		intensity := s.Schema["properties"].(map[string]any)["intensity"].(map[string]any)
		require.InDelta(t, -100, intensity["minimum"], 0.01,
			"v3→v2 回滚不得把 v1→v2 的迁移逆序掉（schema %s@%s）", s.Name, s.Version)
	}
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND name=?", "tenant-a", ns, "threshold").Order("version").Find(&schemas).Error)
	for _, s := range schemas {
		threshold := s.Schema["properties"].(map[string]any)["threshold"].(map[string]any)
		if s.Version == "3.0.0" {
			// v3 行的声明 payload 本身就是 maximum=999，回滚只撤销迁移造成的改写
			require.InDelta(t, 999, threshold["maximum"], 0.01, "v3 行保持其声明 payload")
			continue
		}
		require.InDelta(t, 100, threshold["maximum"], 0.01, "v2→v3 的迁移必须被逆序（schema %s@%s）", s.Name, s.Version)
	}

	// 再回滚 v2→v1：每个 v1 schema 行都恢复安装时的逐字节状态
	rb, err = f.lifecycle.Rollback("tenant-a", res.Extension.ID, "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", rb.Install.Version)

	// v1 行必须与安装时快照逐字节一致（schema_json + content_hash）
	var v1Rows []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=? AND version=?", "tenant-a", ns, "1.0.0").Find(&v1Rows).Error)
	require.Len(t, v1Rows, len(original))
	for _, s := range v1Rows {
		want, ok := original[s.ID]
		require.True(t, ok, "v1 schema 行应来自安装时快照")
		raw, err := json.Marshal(s.Schema)
		require.NoError(t, err)
		require.JSONEq(t, want[0], string(raw), "schema %s@%s 的 schema_json 必须逐字段恢复", s.Name, s.Version)
		require.Equal(t, want[1], s.ContentHash, "schema %s@%s 的 content_hash 必须与安装时一致", s.Name, s.Version)
	}

	// namespace 下全部行的 content_hash 都必须与自身 schema_json 一致（无损坏）
	var all []rundomain.StateSchema
	require.NoError(t, f.db.Where("tenant_id=? AND namespace=?", "tenant-a", ns).Find(&all).Error)
	require.Len(t, all, 6, "3 个版本 × 2 个 schema 行")
	for _, s := range all {
		raw, err := json.Marshal(s.Schema)
		require.NoError(t, err)
		sum := sha256.Sum256(raw)
		require.Equal(t, hex.EncodeToString(sum[:]), s.ContentHash, "schema %s@%s 的 content_hash 必须是 schema_json 的真实哈希", s.Name, s.Version)
	}
}
