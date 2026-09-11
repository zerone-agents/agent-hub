package services

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/domain/aigc"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// AuditStore 是 Recorder 的最小持久化依赖（便于单测注入）。
type AuditStore interface {
	Create(e *audit.Log) error
}

type AuditRecorder struct{ store AuditStore }

func NewAuditRecorder(store AuditStore) *AuditRecorder { return &AuditRecorder{store: store} }

// truncateUTF8：UTF-8 安全截断（按 rune），截断标记 …+N 计入 limit（spec §5.2）。
// 自 keep=limit-1 起逐个让位以最大化保留内容；标记始终放不下时退回硬截断。
func truncateUTF8(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	for keep := limit - 1; keep >= 0; keep-- {
		marker := "…+" + strconv.Itoa(len(runes)-keep)
		if len([]rune(marker)) <= limit-keep {
			return string(runes[:keep]) + marker, true
		}
	}
	return string(runes[:limit]), true
}

// escapeForStdout：CR/LF/TAB 与其余控制字符转可见转义，防日志行注入（spec §5.2）。
func escapeForStdout(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// truncateEntry：列宽上限对齐 audit.Log 的 gorm size tag（spec §5.2）。
func truncateEntry(e *audit.Entry) {
	e.Actor.UserName, _ = truncateUTF8(e.Actor.UserName, 64)
	e.TargetName, _ = truncateUTF8(e.TargetName, 128)
	e.Actor.UserAgent, _ = truncateUTF8(e.Actor.UserAgent, 256)
	if ld, ok := e.Detail.(audit.LoginDetail); ok {
		ld.Username, _ = truncateUTF8(ld.Username, 64)
		e.Detail = ld
	}
}

// Record：从 gin context 提取 actor（e.Actor 非空字段覆盖）→ 截断 →
// stdout 统一格式 → TenantID=="" 仅 stdout → 落库失败降级（不返回错误）。
func (r *AuditRecorder) Record(c *gin.Context, e audit.Entry) {
	a := r.actorFrom(c)
	if e.Actor.TenantID != "" {
		a.TenantID = e.Actor.TenantID
	}
	if e.Actor.UserID != "" {
		a.UserID = e.Actor.UserID
	}
	if e.Actor.UserName != "" {
		a.UserName = e.Actor.UserName
	}
	if e.Actor.RemoteIP != "" {
		a.RemoteIP = e.Actor.RemoteIP
	}
	if e.Actor.UserAgent != "" {
		a.UserAgent = e.Actor.UserAgent
	}
	e.Actor = a
	truncateEntry(&e) // stdout 与 DB 展示一致的截断值

	log.Printf("[AUDIT] %s | user_id=%s user_name=%s target=%s remote_ip=%s result=%s time=%s",
		e.Action, escapeForStdout(e.Actor.UserID), escapeForStdout(e.Actor.UserName), escapeForStdout(e.TargetName),
		escapeForStdout(e.Actor.RemoteIP), e.Status, time.Now().UTC().Format(time.RFC3339))

	r.persist(e)
}

// persist：Detail 序列化 → 空租户仅 stdout → Create → 失败降级（英文错误 + 堆栈）。
// truncateEntry 幂等（已截断值再过一次无变化），legacy 路径由此兜底截断。
func (r *AuditRecorder) persist(e audit.Entry) {
	truncateEntry(&e)
	detail := ""
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil { // 强类型 Detail 不可失败；防御性降级
			log.Printf("[AUDIT] write failed: detail marshal: %v\n%s", err, debug.Stack())
		} else {
			detail = string(b)
		}
	}
	if e.Actor.TenantID == "" {
		return // casdoor org 未知阶段：仅 stdout，防未认证端点填充 DB（spec §5.6）
	}
	row := &audit.Log{
		TenantID: e.Actor.TenantID, UserID: e.Actor.UserID, UserName: e.Actor.UserName,
		Category: e.Category, Action: e.Action, TargetType: e.TargetType,
		TargetID: e.TargetID, TargetName: e.TargetName, Status: e.Status,
		Detail: detail, RemoteIP: e.Actor.RemoteIP, UserAgent: e.Actor.UserAgent,
	}
	if err := r.store.Create(row); err != nil {
		log.Printf("[AUDIT] write failed: %v\n%s", err, debug.Stack())
	}
}

// ---- 便捷方法（handler 一行调用；schema 变更只改 domain 构造器）----

func (r *AuditRecorder) Simple(c *gin.Context, action audit.Action, tt audit.TargetType, id, name string) {
	r.Record(c, audit.SimpleEvent(audit.Actor{}, action, tt, id, name))
}

func (r *AuditRecorder) SimpleWithStatus(c *gin.Context, action audit.Action, tt audit.TargetType, id, name string, st audit.Status) {
	e := audit.SimpleEvent(audit.Actor{}, action, tt, id, name)
	e.Status = st
	r.Record(c, e)
}

func (r *AuditRecorder) Login(c *gin.Context, userID, username, org string, st audit.Status, reason string) {
	r.Record(c, audit.LoginEvent(audit.Actor{UserID: userID, TenantID: org}, username, org, st, reason))
}

func (r *AuditRecorder) RoleChanged(c *gin.Context, id, name, from, to string, st audit.Status) {
	r.Record(c, audit.RoleChangedEvent(audit.Actor{}, id, name, from, to, st))
}

func (r *AuditRecorder) StatusChanged(c *gin.Context, id, name, from, to string) {
	r.Record(c, audit.StatusChangedEvent(audit.Actor{}, id, name, from, to))
}

func (r *AuditRecorder) InviteCreated(c *gin.Context, inviteID, role string, days int) {
	r.Record(c, audit.InviteCreatedEvent(audit.Actor{}, inviteID, role, days))
}

func (r *AuditRecorder) AigcSaved(c *gin.Context, changed []aigc.AigcConfigField) {
	r.Record(c, audit.AigcSavedEvent(audit.Actor{}, changed))
}

// ---- legacy formatter（spec §5.5：stdout 逐字保留 provider.go 现输出，动态字段转义）----

func (r *AuditRecorder) RevealKeyLegacy(c *gin.Context, providerID uint64) {
	a := r.actorFrom(c)
	log.Printf("[AUDIT] provider API key revealed | user_id=%s user_name=%s provider_id=%d remote_ip=%s method=%s path=%s result=success time=%s",
		escapeForStdout(a.UserID), escapeForStdout(a.UserName), providerID, escapeForStdout(a.RemoteIP), c.Request.Method, c.Request.URL.Path,
		time.Now().UTC().Format(time.RFC3339))
	r.persist(audit.SimpleEvent(a, audit.ActionRevealKey, audit.TargetProvider, strconv.FormatUint(providerID, 10), ""))
}

func (r *AuditRecorder) RuntimeConfigLegacy(c *gin.Context, providers int) {
	a := r.actorFrom(c)
	log.Printf("[AUDIT] provider runtime-config served | user_id=%s user_name=%s providers=%d remote_ip=%s time=%s",
		escapeForStdout(a.UserID), escapeForStdout(a.UserName), providers, escapeForStdout(a.RemoteIP), time.Now().UTC().Format(time.RFC3339))
	r.persist(audit.RuntimeConfigEvent(a, providers))
}

func (r *AuditRecorder) actorFrom(c *gin.Context) audit.Actor {
	return audit.Actor{
		TenantID:  tenant.GetTenantID(c),
		UserID:    c.GetString("user_id"),
		UserName:  c.GetString("user_name"),
		RemoteIP:  c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}
}
