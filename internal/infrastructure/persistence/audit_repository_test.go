package repository

import (
	"reflect"
	"testing"
	"time"

	"control-panel/internal/domain/audit"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// auditTestSqlite 包装 glebarez/sqlite：时间列声明类型统一为 datetime（去掉 MySQL
// 专属的 "(6)" 精度后缀）。驱动读侧仅对声明类型精确匹配 DATE/DATETIME/TIMESTAMP
// 的 TEXT 值做 time.Time 转换（glebarez/go-sqlite 的 Next switch），"datetime(6)"
// 会以 string 返回、rows.Scan 进 *time.Time 直接报错。生产 MySQL 不经此路径，
// 模型 tag 的 datetime(6) 微秒精度不受影响。
type auditTestSqlite struct{ *sqlite.Dialector }

func (d auditTestSqlite) DataTypeOf(field *schema.Field) string {
	// 注意：显式 type tag 的字段其 field.DataType 就是 tag 值本身（如 "datetime(6)"），
	// 不等于 schema.Time，须按 Go 类型判断。
	if field.IndirectFieldType == reflect.TypeOf(time.Time{}) {
		return "datetime"
	}
	return d.Dialector.DataTypeOf(field)
}

// Migrator 覆写：glebarez 的 Migrator(db) 为值接收者，经嵌入字段提升时会把原生
// Dialector 拷贝进 migrator.Config，令 AutoMigrate 绕过上面的 DataTypeOf 覆写
// （DDL 仍生成 datetime(6)）。此处用包装器自身重建同款 Migrator。
func (d auditTestSqlite) Migrator(db *gorm.DB) gorm.Migrator {
	return sqlite.Migrator{Migrator: migrator.Migrator{Config: migrator.Config{
		DB:                          db,
		Dialector:                   d,
		CreateIndexAfterCreateTable: true,
	}}}
}

func newAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(auditTestSqlite{sqlite.Open(":memory:").(*sqlite.Dialector)}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	return db
}

func mustCreate(t *testing.T, db *gorm.DB, tenant string, action audit.Action, cat audit.Category, at time.Time) uint64 {
	t.Helper()
	l := &audit.Log{
		TenantID: tenant, UserID: "u1", UserName: "alice", Category: cat, Action: action,
		TargetType: audit.TargetSystem, Status: audit.StatusSuccess, CreatedAt: at,
	}
	require.NoError(t, db.Create(l).Error)
	return l.ID
}

func TestAuditCompositeIndexExists(t *testing.T) {
	db := newAuditTestDB(t)
	var n int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_audit_tenant_created'").Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

func TestAuditListFiltersAndOrder(t *testing.T) {
	db := newAuditTestDB(t)
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	mustCreate(t, db, "t1", audit.ActionDeploy, audit.CatAgent, base)
	mustCreate(t, db, "t1", audit.ActionLogin, audit.CatAuth, base.Add(time.Second))
	mustCreate(t, db, "t2", audit.ActionLogin, audit.CatAuth, base.Add(2*time.Second)) // 他租户

	repo := NewAuditRepository(db)
	logs, total, err := repo.List(AuditListFilter{TenantID: "t1", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Equal(t, audit.ActionLogin, logs[0].Action) // created_at DESC
	require.Equal(t, audit.ActionDeploy, logs[1].Action)

	logs, total, err = repo.List(AuditListFilter{TenantID: "t1", Category: "auth", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, audit.ActionLogin, logs[0].Action)
}

func TestAuditListUserFuzzy(t *testing.T) {
	db := newAuditTestDB(t)
	at := time.Now()
	mustCreate(t, db, "t1", audit.ActionLogin, audit.CatAuth, at)
	db.Exec("UPDATE audit_logs SET user_name='bob', user_id='42' WHERE 1=1")
	repo := NewAuditRepository(db)
	_, total, err := repo.List(AuditListFilter{TenantID: "t1", User: "ob", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total) // user_name 模糊
	_, total, err = repo.List(AuditListFilter{TenantID: "t1", User: "42", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total) // user_id 模糊
}

func TestAuditPaginationTieBreakByID(t *testing.T) {
	db := newAuditTestDB(t)
	at := time.Now() // 相同 created_at
	mustCreate(t, db, "t1", audit.ActionDeploy, audit.CatAgent, at)
	mustCreate(t, db, "t1", audit.ActionStop, audit.CatAgent, at)
	mustCreate(t, db, "t1", audit.ActionStart, audit.CatAgent, at)
	repo := NewAuditRepository(db)
	page1, _, err := repo.List(AuditListFilter{TenantID: "t1", Page: 1, PageSize: 2})
	require.NoError(t, err)
	page2, _, err := repo.List(AuditListFilter{TenantID: "t1", Page: 2, PageSize: 2})
	require.NoError(t, err)
	ids := []uint64{page1[0].ID, page1[1].ID, page2[0].ID}
	require.Equal(t, ids[0] > ids[1] && ids[1] > ids[2], true) // id DESC 决胜，不重复不漏
}

func TestAuditSnapshotBoundaryAndCountSamePredicate(t *testing.T) {
	db := newAuditTestDB(t)
	base := time.Now()
	mustCreate(t, db, "t1", audit.ActionDeploy, audit.CatAgent, base)
	mustCreate(t, db, "t1", audit.ActionStop, audit.CatAgent, base)
	repo := NewAuditRepository(db)
	snap, err := repo.MaxID("t1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, snap, uint64(2))

	// 快照后新写入（含 category/action 筛选不影响 snapshotId 全集语义，spec §5.3）
	newID := mustCreate(t, db, "t1", audit.ActionLogin, audit.CatAuth, base.Add(time.Second))

	snapPtr := snap
	logs, total, err := repo.List(AuditListFilter{TenantID: "t1", SnapshotID: &snapPtr, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total) // COUNT 与 items 同谓词：新记录不进 items 也不进 total
	for _, l := range logs {
		require.LessOrEqual(t, l.ID, snap)
		require.NotEqual(t, newID, l.ID)
	}

	zero := uint64(0)
	_, total, err = repo.List(AuditListFilter{TenantID: "t1", SnapshotID: &zero, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 0, total) // "0" 哨兵 = 恒空集
}

func TestAuditMaxIDEmptyTenant(t *testing.T) {
	db := newAuditTestDB(t)
	repo := NewAuditRepository(db)
	max, err := repo.MaxID("nobody")
	require.NoError(t, err)
	require.EqualValues(t, 0, max)
}
