package handler

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/audit"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
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
// 与 persistence/audit_repository_test.go 同款（跨包无法复用而复制）。
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

func newAuditHandlerEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(auditTestSqlite{sqlite.Open(":memory:").(*sqlite.Dialector)}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	repo := repository.NewAuditRepository(db)
	h := NewAuditHandler(services.NewAuditQuerier(repo))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api/v1/admin", func(c *gin.Context) {
		c.Set("tenant_id", "t1") // tenant.GetTenantID 读取的 key（Step 0 已核实）
		// RequireAdmin 经 RequireRole 读取 "roles"（[]string），与 JWT 中间件写入类型一致
		c.Set("roles", []string{c.GetHeader("X-Test-Role")})
		c.Next()
	}, middleware.RequireAdmin())
	grp.GET("/audit-logs", h.List)
	return r, db
}

func seedAudit(t *testing.T, db *gorm.DB, action audit.Action, name string) uint64 {
	t.Helper()
	l := &audit.Log{TenantID: "t1", UserID: "7", UserName: name, Category: audit.CatAuth,
		Action: action, TargetType: audit.TargetSystem, Status: audit.StatusSuccess, Detail: `{"field":"role"}`}
	require.NoError(t, db.Create(l).Error)
	return l.ID
}

// seedAuditAt 显式指定 created_at（from/to 范围过滤测试用）。
func seedAuditAt(t *testing.T, db *gorm.DB, action audit.Action, name string, at time.Time) uint64 {
	t.Helper()
	l := &audit.Log{TenantID: "t1", UserID: "7", UserName: name, Category: audit.CatAuth,
		Action: action, TargetType: audit.TargetSystem, Status: audit.StatusSuccess, CreatedAt: at}
	require.NoError(t, db.Create(l).Error)
	return l.ID
}

func getAudit(t *testing.T, r *gin.Engine, role, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/admin/audit-logs"+query, nil)
	req.Header.Set("X-Test-Role", role)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuditListAdmin200AndStructure(t *testing.T) {
	r, db := newAuditHandlerEnv(t)
	seedAudit(t, db, audit.ActionLogin, "alice")
	w := getAudit(t, r, "admin", "")
	require.Equal(t, 200, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"items"`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"snapshotId":"1"`)          // 十进制字符串
	require.Contains(t, body, `"detail":{"field":"role"}`) // detail 为 JSON 对象
	require.Contains(t, body, `"id":"1"`)                  // items[].id 字符串
}

func TestAuditListNonAdmin403(t *testing.T) {
	r, _ := newAuditHandlerEnv(t)
	require.Equal(t, 403, getAudit(t, r, "member", "").Code)
	require.Equal(t, 403, getAudit(t, r, "maintainer", "").Code)
}

func TestAuditSnapshotStableAcrossWrites(t *testing.T) {
	r, db := newAuditHandlerEnv(t)
	seedAudit(t, db, audit.ActionLogin, "a")
	w := getAudit(t, r, "admin", "")
	snap := `"snapshotId":"1"`
	require.Contains(t, w.Body.String(), snap)
	// 快照捕获后新写入
	seedAudit(t, db, audit.ActionLogout, "b")
	w2 := getAudit(t, r, "admin", "?snapshotId=1")
	require.Equal(t, 200, w2.Code)
	require.Contains(t, w2.Body.String(), `"total":1`) // items 与 total 均不变（spec §5.3）
	require.NotContains(t, w2.Body.String(), "auth.logout")
}

func TestAuditEmptyTenantIssuesZeroSentinel(t *testing.T) {
	r, db := newAuditHandlerEnv(t)
	db.Exec("DELETE FROM audit_logs") // 空租户
	w := getAudit(t, r, "admin", "")
	require.Contains(t, w.Body.String(), `"snapshotId":"0"`)
	require.Contains(t, w.Body.String(), `"total":0`)
	w2 := getAudit(t, r, "admin", "?snapshotId=0")
	require.Contains(t, w2.Body.String(), `"total":0`) // 续传 "0" 恒空集，快照不重开
	seedAudit(t, db, audit.ActionLogin, "x")
	w3 := getAudit(t, r, "admin", "?snapshotId=0")
	require.Contains(t, w3.Body.String(), `"total":0`) // 期间新写入不出现
}

func TestAuditSnapshotInvalid400Chinese(t *testing.T) {
	r, _ := newAuditHandlerEnv(t)
	for _, q := range []string{"?snapshotId=abc", "?snapshotId=-1", "?snapshotId=1e9", "?snapshotId=99999999999999999999999999"} {
		w := getAudit(t, r, "admin", q)
		require.Equal(t, 400, w.Code, q)
		require.Contains(t, w.Body.String(), "无效的快照参数", q)
	}
}

func TestAuditBadTimeRange400Chinese(t *testing.T) {
	r, _ := newAuditHandlerEnv(t)
	require.Equal(t, 400, getAudit(t, r, "admin", "?from=not-a-time").Code)
}

func TestAuditBigIDStringRoundTrip(t *testing.T) {
	r, db := newAuditHandlerEnv(t)
	big := uint64(1) << 62 // > 2^53-1
	l := &audit.Log{TenantID: "t1", Category: audit.CatAuth, Action: audit.ActionLogin,
		TargetType: audit.TargetSystem, Status: audit.StatusSuccess}
	l.ID = big
	require.NoError(t, db.Create(l).Error)
	w := getAudit(t, r, "admin", fmt.Sprintf("?snapshotId=%d", big))
	require.Contains(t, w.Body.String(), fmt.Sprintf(`"id":"%d"`, big)) // 序列化无损
	require.Contains(t, w.Body.String(), fmt.Sprintf(`"snapshotId":"%d"`, big))
	w2 := getAudit(t, r, "admin", fmt.Sprintf("?snapshotId=%d", big)) // 回传解析成功
	require.Equal(t, 200, w2.Code)
}

func TestAuditPageSizeCapAndDefaults(t *testing.T) {
	r, _ := newAuditHandlerEnv(t)
	require.Equal(t, 200, getAudit(t, r, "admin", "?page_size=500").Code) // 上限截断为 100，不报错
	require.Equal(t, 400, getAudit(t, r, "admin", "?page=0").Code)        // 非法中文 400
	w := getAudit(t, r, "admin", "")
	require.NotContains(t, w.Body.String(), "无效")
}

// TestAuditFromToRangeFilters 补 Task 2 ledgered 覆盖债：repository From/To 过滤
// 路径此前零覆盖。经 handler→querier→repo 全链路验证有效区间只命中落入区间的行。
func TestAuditFromToRangeFilters(t *testing.T) {
	r, db := newAuditHandlerEnv(t)
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	seedAuditAt(t, db, audit.ActionLogin, "a", base)                   // t1：区间外
	seedAuditAt(t, db, audit.ActionLogout, "b", base.Add(2*time.Hour)) // t2：区间内
	w := getAudit(t, r, "admin", fmt.Sprintf("?from=%s&to=%s",
		base.Add(time.Hour).Format(time.RFC3339), base.Add(3*time.Hour).Format(time.RFC3339)))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"total":1`) // 仅 t2 命中
	require.Contains(t, w.Body.String(), "auth.logout")
	require.NotContains(t, w.Body.String(), "auth.login")
}
