// H7.1 扩展数据迁移引擎：manifest.migrations 的 JSON Patch 子集实现。
//
// 支持 replace / add / remove 三种 op，路径为 JSON Pointer 风格
// （/properties/intensity/minimum，不含数组下标与转义，扩展 Schema
// 的变换场景足够且行为可预期）。迁移作用于本扩展 namespace 下的全部
// state_schemas.schema_json；正向执行时逐 op 记录旧值（OldValue），
// 回滚时无需扩展方手写 inverse——按记录的旧值逆序执行 replace 即可恢复。
// 扩展方也可在迁移里显式声明 rollback 段，优先于推导的 inverse。
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/extensionmanifest"
	"gorm.io/gorm"
)

// migrationLogEntry 是 migration_log 中单条 op 的执行记录。
// SchemaID 绑定该 op 实际作用的状态 Schema 行：正向迁移逐 schema 记录，
// 回滚时按 schema 各恢复各的旧值，而不是把 inverse 套到 namespace 下
// 全部 Schema（多 Schema 扩展会导致 schema_json/content_hash 损坏）。
type migrationLogEntry struct {
	SchemaID uint64 `json:"schemaId,omitempty"`
	Path     string `json:"path"`
	Op       string `json:"op"`
	OldValue any    `json:"oldValue,omitempty"`
	NewValue any    `json:"newValue,omitempty"`
}

// migrationLogRecord 是 migration_log 中的一条迁移批次记录。
type migrationLogRecord struct {
	Direction string              `json:"direction"` // upgrade / rollback
	From      string              `json:"from"`
	To        string              `json:"to"`
	At        string              `json:"at"`
	Ops       []migrationLogEntry `json:"ops"`
}

// appendMigrationLog 把一次迁移批次追加到 installs.MigrationLog（JSON 数组）。
func appendMigrationLog(inst *extension.Install, direction, from, to string, entries []migrationLogEntry) error {
	var records []migrationLogRecord
	if strings.TrimSpace(inst.MigrationLog) != "" {
		if err := json.Unmarshal([]byte(inst.MigrationLog), &records); err != nil {
			// 历史日志损坏不阻断操作：从空开始重新记录
			records = nil
		}
	}
	records = append(records, migrationLogRecord{
		Direction: direction, From: from, To: to,
		At:  time.Now().UTC().Format(time.RFC3339),
		Ops: entries,
	})
	raw, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("序列化迁移日志失败：%w", err)
	}
	inst.MigrationLog = string(raw)
	return nil
}

// selectMigrations 从 manifest 中选出 from→to 区间的迁移声明；
// 无直接声明时沿 from 出发按 semver 升序做链式拼接（如 1.0.0→1.1.0→2.0.0）。
func selectMigrations(manifest *extensionmanifest.Manifest, from, to string) []extensionmanifest.ManifestMigration {
	var direct []extensionmanifest.ManifestMigration
	var chain []extensionmanifest.ManifestMigration
	current := from
	for current != to {
		var candidates []extensionmanifest.ManifestMigration
		for _, mig := range manifest.Migrations {
			if mig.From != current {
				continue
			}
			// 只允许向目标方向前进的链
			if c, err := compareSemver(mig.To, current); err != nil || c <= 0 {
				continue
			}
			if c, err := compareSemver(mig.To, to); err != nil || c > 0 {
				continue
			}
			candidates = append(candidates, mig)
		}
		if len(candidates) == 0 {
			chain = nil
			break
		}
		sort.Slice(candidates, func(i, j int) bool {
			c, _ := compareSemver(candidates[i].To, candidates[j].To)
			return c < 0
		})
		next := candidates[0]
		chain = append(chain, next)
		current = next.To
	}
	if len(chain) > 0 && current == to {
		return chain
	}
	for _, mig := range manifest.Migrations {
		if mig.From == from && mig.To == to {
			direct = append(direct, mig)
		}
	}
	return direct
}

func compareSemver(a, b string) (int, error) {
	return extension.CompareVersions(a, b)
}

// applyMigration 把一条迁移声明正向应用到本扩展 namespace 的全部状态
// Schema，返回逐 op 的旧值记录（供回滚推导 inverse）。
func applyMigration(tx *gorm.DB, tenantID, namespace string, mig extensionmanifest.ManifestMigration) ([]migrationLogEntry, error) {
	return applyMigrationOps(tx, tenantID, namespace, mig.Ops)
}

// applyMigrationOps 把一组 op 应用到 tenant+namespace 下的全部状态 Schema。
// 逐行、逐 op 执行；任何一行执行失败即返回中文错误（外层事务回滚）。
// 每条记录都带 SchemaID，供回滚时按 schema 精确恢复。
func applyMigrationOps(tx *gorm.DB, tenantID, namespace string, ops []extensionmanifest.ManifestMigrationOp) ([]migrationLogEntry, error) {
	var schemas []rundomain.StateSchema
	if err := tx.Where("tenant_id=? AND namespace=?", tenantID, namespace).Find(&schemas).Error; err != nil {
		return nil, err
	}
	var entries []migrationLogEntry
	hit := make([]int, len(ops))
	for i := range schemas {
		applied, err := applyOpsToSchema(tx, &schemas[i], ops)
		if err != nil {
			return nil, fmt.Errorf("状态 Schema %s@%s 应用 op 失败：%v",
				schemas[i].Name, schemas[i].Version, err)
		}
		for _, e := range applied {
			e.SchemaID = schemas[i].ID
			entries = append(entries, e)
			for k := range ops {
				if ops[k].Op == e.Op && ops[k].Path == e.Path {
					hit[k]++
				}
			}
		}
	}
	for k := range ops {
		if hit[k] == 0 {
			return nil, fmt.Errorf("迁移 op %s %s 未命中 namespace %s 下任何状态 Schema，请检查路径是否拼写正确",
				ops[k].Op, ops[k].Path, namespace)
		}
	}
	return entries, nil
}

// applyOpsToSchema 把一组 op 应用到单个状态 Schema 行（schema_json 原地
// 修改并更新 content_hash），返回逐 op 的旧值记录（不含 SchemaID）。
func applyOpsToSchema(tx *gorm.DB, schema *rundomain.StateSchema, ops []extensionmanifest.ManifestMigrationOp) ([]migrationLogEntry, error) {
	var entries []migrationLogEntry
	changed := false
	for _, op := range ops {
		oldValue, err := applySchemaOp(schema.Schema, op)
		if errors.Is(err, errOpNotApplicable) {
			// 多 Schema 扩展：该 op 的目标路径不在本 schema 中，跳过
			continue
		}
		if err != nil {
			return nil, err
		}
		changed = true
		entries = append(entries, migrationLogEntry{Path: op.Path, Op: op.Op, OldValue: oldValue, NewValue: op.Value})
	}
	if changed {
		raw, err := json.Marshal(schema.Schema)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		// 直接以 JSON 文本更新（与 StateSchema 的 serializer:json 落库格式一致）
		if err := tx.Model(&rundomain.StateSchema{}).Where("id=?", schema.ID).
			Updates(map[string]any{"schema_json": string(raw), "content_hash": hex.EncodeToString(sum[:])}).Error; err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// applyInverseFromLog 按 migration_log 推导 inverse：筛选 from/to 版本区间
// 内的 upgrade 批次，逆序、逐 op 以 OldValue 执行 replace 恢复 schema_json；
// remove 的逆操作是 add 回旧值，add 的逆操作是 remove。
// inverse 严格按日志条目的 SchemaID 绑定到具体 schema 行逐行恢复，保证
// 多 Schema 扩展回滚后每个 schema_json 与 content_hash 一致；历史日志
// 条目缺 SchemaID 时退化为按 namespace 全量套用（旧行为，仅兼容存量数据）。
func applyInverseFromLog(tx *gorm.DB, tenantID, namespace, migrationLog, targetVersion, currentVersion string) ([]migrationLogEntry, error) {
	var records []migrationLogRecord
	if err := json.Unmarshal([]byte(migrationLog), &records); err != nil {
		return nil, fmt.Errorf("迁移日志无法解析：%v", err)
	}
	var selected []migrationLogRecord
	for _, rec := range records {
		if rec.Direction != "upgrade" {
			continue
		}
		// 只回滚目标版本之后发生的升级
		fc, ferr := compareSemver(rec.From, targetVersion)
		tc, terr := compareSemver(rec.To, currentVersion)
		if ferr != nil || terr != nil {
			continue
		}
		if fc >= 0 && tc <= 0 {
			selected = append(selected, rec)
		}
	}
	var entries []migrationLogEntry
	// schemaID → 该行的 inverse op 序列（批次逆序 × op 逆序，顺序即恢复顺序）
	perSchema := map[uint64][]extensionmanifest.ManifestMigrationOp{}
	var order []uint64
	for i := len(selected) - 1; i >= 0; i-- {
		rec := selected[i]
		for j := len(rec.Ops) - 1; j >= 0; j-- {
			e := rec.Ops[j]
			var inv extensionmanifest.ManifestMigrationOp
			switch e.Op {
			case "replace":
				inv = extensionmanifest.ManifestMigrationOp{Op: "replace", Path: e.Path, Value: e.OldValue}
			case "add":
				inv = extensionmanifest.ManifestMigrationOp{Op: "remove", Path: e.Path}
			case "remove":
				inv = extensionmanifest.ManifestMigrationOp{Op: "add", Path: e.Path, Value: e.OldValue}
			default:
				continue
			}
			if e.SchemaID == 0 {
				// 存量日志缺 schema 绑定：维持旧的按 namespace 全量套用行为
				applied, err := applyMigrationOps(tx, tenantID, namespace, []extensionmanifest.ManifestMigrationOp{inv})
				if err != nil {
					return nil, err
				}
				entries = append(entries, applied...)
				continue
			}
			if _, ok := perSchema[e.SchemaID]; !ok {
				order = append(order, e.SchemaID)
			}
			perSchema[e.SchemaID] = append(perSchema[e.SchemaID], inv)
		}
	}
	for _, schemaID := range order {
		var schema rundomain.StateSchema
		if err := tx.Where("id=?", schemaID).First(&schema).Error; err != nil {
			return nil, fmt.Errorf("回滚找不到迁移记录绑定的状态 Schema（id=%d）：%v", schemaID, err)
		}
		applied, err := applyOpsToSchema(tx, &schema, perSchema[schemaID])
		if err != nil {
			return nil, fmt.Errorf("状态 Schema %s@%s 回滚失败：%v", schema.Name, schema.Version, err)
		}
		for _, e := range applied {
			e.SchemaID = schemaID
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// reverseOps 把显式 rollback 段逆序展开（显式声明时原样逆序执行即可，
// 语义由扩展方自己保证）。
func reverseOps(ops []extensionmanifest.ManifestMigrationOp) []extensionmanifest.ManifestMigrationOp {
	out := make([]extensionmanifest.ManifestMigrationOp, 0, len(ops))
	for i := len(ops) - 1; i >= 0; i-- {
		out = append(out, ops[i])
	}
	return out
}

// errOpNotApplicable 表示该 op 的目标路径在本 schema 中不存在：
// 多 Schema 扩展的迁移通常只作用于部分 schema，命不中即跳过（不记日志）。
var errOpNotApplicable = errors.New("op 不适用于该 schema")

// applySchemaOp 把单条 JSON Patch 子集 op 应用到 schema（原地修改）。
// 返回 replace/remove 命中位置的旧值（add 返回 nil）。路径段不支持
// 数组下标与 ~ / 转义——扩展 Schema 均为对象树。目标路径在本 schema
// 不存在时返回 errOpNotApplicable，由调用方跳过该行。
func applySchemaOp(schema map[string]any, op extensionmanifest.ManifestMigrationOp) (any, error) {
	if op.Path == "" || op.Path[0] != '/' {
		return nil, fmt.Errorf("路径 %q 必须是以 / 开头", op.Path)
	}
	segments := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
	if len(segments) == 0 {
		return nil, fmt.Errorf("路径 %q 不能为空", op.Path)
	}
	parent := schema
	for _, seg := range segments[:len(segments)-1] {
		next, ok := parent[seg].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w：路径 %q 的中间段 %q 不存在或不是对象", errOpNotApplicable, op.Path, seg)
		}
		parent = next
	}
	last := segments[len(segments)-1]
	switch op.Op {
	case "replace":
		old, ok := parent[last]
		if !ok {
			return nil, fmt.Errorf("%w：路径 %q 不存在，无法 replace", errOpNotApplicable, op.Path)
		}
		parent[last] = op.Value
		return old, nil
	case "add":
		parent[last] = op.Value
		return nil, nil
	case "remove":
		old, ok := parent[last]
		if !ok {
			return nil, fmt.Errorf("%w：路径 %q 不存在，无法 remove", errOpNotApplicable, op.Path)
		}
		delete(parent, last)
		return old, nil
	}
	return nil, fmt.Errorf("未知操作 %q", op.Op)
}
