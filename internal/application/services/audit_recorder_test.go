package services

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"control-panel/internal/domain/aigc"
	"control-panel/internal/domain/audit"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// auditRecorderSqlite 包装 glebarez/sqlite：时间列声明类型统一为 datetime（去掉 MySQL
// 专属的 "(6)" 精度后缀）。驱动读侧仅对声明类型精确匹配 DATE/DATETIME/TIMESTAMP
// 的 TEXT 值做 time.Time 转换（glebarez/go-sqlite 的 Next switch），"datetime(6)"
// 会以 string 返回、rows.Scan 进 *time.Time 直接报错。生产 MySQL 不经此路径，
// 模型 tag 的 datetime(6) 微秒精度不受影响。（与 Task 2 repository 测试同款解法。）
type auditRecorderSqlite struct{ *sqlite.Dialector }

func (d auditRecorderSqlite) DataTypeOf(field *schema.Field) string {
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
func (d auditRecorderSqlite) Migrator(db *gorm.DB) gorm.Migrator {
	return sqlite.Migrator{Migrator: migrator.Migrator{Config: migrator.Config{
		DB:                          db,
		Dialector:                   d,
		CreateIndexAfterCreateTable: true,
	}}}
}

func newRecorderEnv(t *testing.T) (*AuditRecorder, *gorm.DB, *bytes.Buffer) {
	t.Helper()
	db, err := gorm.Open(auditRecorderSqlite{sqlite.Open(":memory:").(*sqlite.Dialector)}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	buf := &bytes.Buffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	// 注意不能 SetOutput(nil)：Go 1.25 的 log.output 对 nil out 直接 nil 解引用
	// （log.go:244 l.out.Write），会炸掉同包后续使用 std log 的测试。
	t.Cleanup(func() { log.SetOutput(prev) })
	return NewAuditRecorder(repository.NewAuditRepository(db)), db, buf
}

func auditCtx(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", "7")
	c.Set("user_name", "alice")
	c.Set("tenant_id", "t1")
	c.Request = httptest.NewRequest("POST", "/x", nil)
	return c
}

func TestRecorderActorExtractionAndOverride(t *testing.T) {
	rec, db, _ := newRecorderEnv(t)
	c := auditCtx(t)
	c.Request.Header.Set("User-Agent", "ua")
	// Actor 覆盖：TenantID 显式传入（auth 点位语义）
	rec.Record(c, audit.SimpleEvent(audit.Actor{TenantID: "explicit"}, audit.ActionSetup, audit.TargetSystem, "", ""))
	var row audit.Log
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, "explicit", row.TenantID) // 非空覆盖
	require.Equal(t, "7", row.UserID)          // 空字段取 context
	require.Equal(t, "alice", row.UserName)
	require.Equal(t, "ua", row.UserAgent)
}

func TestRecorderEmptyTenantStdoutOnly(t *testing.T) {
	rec, db, buf := newRecorderEnv(t)
	c := auditCtx(t)
	c.Set("tenant_id", "") // casdoor 前期失败：TenantID="" → 仅 stdout（spec §5.6）
	rec.Login(c, "", "", "", audit.StatusFailure, "")
	var n int64
	require.NoError(t, db.Model(&audit.Log{}).Count(&n).Error)
	require.EqualValues(t, 0, n)
	require.Contains(t, buf.String(), "[AUDIT] auth.login")
}

func TestRecorderStdoutInjectionGuard(t *testing.T) {
	rec, _, buf := newRecorderEnv(t)
	c := auditCtx(t)
	c.Set("user_name", "evil\n[AUDIT] forged | result=success")
	rec.Simple(c, audit.ActionDeploy, audit.TargetAgent, "a\nb", "a\nb")
	out := buf.String()
	// 换行被转义后不得伪造新的 AUDIT 行；整条 stdout 保持单行（仅行尾换行）
	require.Equal(t, 0, strings.Count(out, "\n[AUDIT]"), "换行不得伪造新的 AUDIT 行:\n%s", out)
	require.Equal(t, 1, strings.Count(out, "\n"), "stdout 必须保持单行:\n%s", out)
	require.Contains(t, out, `\n`) // 转义可见
}

func TestRecorderUserIDStdoutInjection(t *testing.T) {
	rec, _, buf := newRecorderEnv(t)
	c := auditCtx(t)
	// 登录失败流程：userID 是请求体携带的尝试标识（pre-auth，spec §5.6 残余注入面），
	// 经 Login 的显式 Actor.UserID 覆盖 context，必须同样被 stdout 转义。
	rec.Login(c, "evil\n[AUDIT] forged | result=success", "user\nname", "t1", audit.StatusFailure, audit.ReasonInvalidCredentials)
	out := buf.String()
	// 换行被转义后不得伪造新的 AUDIT 行；整条 stdout 保持单行（仅行尾换行）
	require.Equal(t, 0, strings.Count(out, "\n[AUDIT]"), "user_id 换行不得伪造新的 AUDIT 行:\n%s", out)
	require.Equal(t, 1, strings.Count(out, "\n"), "stdout 必须保持单行:\n%s", out)
	require.Contains(t, out, `\n`) // 转义可见
}

func TestRecorderTruncationWithinColumnWidth(t *testing.T) {
	rec, db, _ := newRecorderEnv(t)
	c := auditCtx(t)
	longName := strings.Repeat("字", 200) // 200 rune
	longUA := strings.Repeat("u", 500)
	c.Request.Header.Set("User-Agent", longUA)
	rec.Simple(c, audit.ActionDeploy, audit.TargetAgent, longName, longName)
	var row audit.Log
	require.NoError(t, db.First(&row).Error)
	require.LessOrEqual(t, len([]rune(row.TargetName)), 128) // 标记计入列宽（spec §5.2）
	require.LessOrEqual(t, len([]rune(row.UserAgent)), 256)
	require.Contains(t, row.TargetName, "…+") // 截断标记
	require.True(t, utf8Valid(row.TargetName))
}

func utf8Valid(s string) bool { return json.Valid([]byte(`"` + strings.ReplaceAll(s, `"`, "") + `"`)) }

func TestRecorderWriteFailureDegradation(t *testing.T) {
	_, _, buf := newRecorderEnv(t) // 搭好 stdout 捕获；本测试注入独立 errStore
	rec := NewAuditRecorder(errStore{})
	c := auditCtx(t)
	rec.Simple(c, audit.ActionDeploy, audit.TargetAgent, "x", "x")
	require.Contains(t, buf.String(), "[AUDIT] write failed") // 英文错误
	require.Contains(t, buf.String(), "goroutine")            // debug.Stack() 堆栈
}

type errStore struct{}

func (errStore) Create(*audit.Log) error { return errBoom }

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "boom" }

func TestRecorderLegacyFormattersVerbatim(t *testing.T) {
	rec, db, buf := newRecorderEnv(t)
	c := auditCtx(t)
	rec.RevealKeyLegacy(c, 42)
	require.Contains(t, buf.String(), "[AUDIT] provider API key revealed | user_id=7 user_name=alice provider_id=42")
	require.Contains(t, buf.String(), "method=POST")
	buf.Reset()
	rec.RuntimeConfigLegacy(c, 3)
	require.Contains(t, buf.String(), "[AUDIT] provider runtime-config served | user_id=7 user_name=alice providers=3")
	var n int64
	require.NoError(t, db.Model(&audit.Log{}).Count(&n).Error)
	require.EqualValues(t, 2, n) // legacy 事件同样落库
}

func TestRecorderLoginDetailSerialized(t *testing.T) {
	rec, db, _ := newRecorderEnv(t)
	c := auditCtx(t)
	rec.Login(c, "7", "alice", "t1", audit.StatusFailure, audit.ReasonInvalidCredentials)
	var row audit.Log
	require.NoError(t, db.First(&row).Error)
	var d map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.Detail), &d))
	require.Equal(t, "invalid_credentials", d["reason"])
	require.Equal(t, "alice", d["username"])
}

func TestRecorderAigcSaved(t *testing.T) {
	rec, db, _ := newRecorderEnv(t)
	c := auditCtx(t)
	rec.AigcSaved(c, []aigc.AigcConfigField{aigc.AigcFieldUSCC})
	var row audit.Log
	require.NoError(t, db.First(&row).Error)
	require.Contains(t, row.Detail, `"changedFields":["uscc"]`)
}
