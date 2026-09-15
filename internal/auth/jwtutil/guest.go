package jwtutil

import (
	"net/http"
	"strings"

	authdom "control-panel/internal/domain/auth"

	"github.com/gin-gonic/gin"
)

// pendingApprovalMarker 是 403 错误体 error 字符串的固定前缀（wire 契约，
// 值不可变更）：前端据此做字符串匹配识别 guest（原"待审批"）状态。
// guest 化改造后前缀原样下发，仅其后文案变更为聊天页引导。
const pendingApprovalMarker = "PENDING_APPROVAL"

// pendingWhitelistExact 是 guest 用户仍可访问的精确路径（对照 cmd/server/main.go 实际注册的路由）：
//   - /auth/userinfo：前端获取当前用户信息的 "me" 端点，聊天页/等待页依赖它判定登录态与角色；
//   - /auth/logout：允许 guest 用户退出登录；
//   - /health：健康检查。
//
// /health/:service 带路径参数，由 healthPathPrefix 前缀匹配放行。
//
// 说明：guard 目前只挂在 /api/v1 业务路由组（AuthMiddlewareWithCLI 之后），
// 上述白名单路径本身不在该组内；白名单是防御性设计——若将来 guard 挂载范围
// 扩大（例如挂到引擎级或 /auth 组），这些路径也不会被误伤。
var pendingWhitelistExact = map[string]struct{}{
	"/auth/userinfo": {},
	"/auth/logout":   {},
	"/health":        {},
}

const healthPathPrefix = "/health/"

// pathPattern 是段级路径模式：segments 按 "/" 分段（首段空串因前置 "/"），
// token "{}" 通配恰好一个非空段（序位匹配，无通配符穿越段边界）。
// 仅用于 GET 配置端点白名单（issue #132）——method 校验避免放行写操作。
type pathPattern struct {
	method   string
	segments []string
}

// pendingConfigPatterns 是 guest 用户可读的桌面配置端点：
//   - /api/v1/providers、/api/v1/providers/runtime-config：桌面 provider 同步
//   - /api/v1/agents/manifest、/api/v1/agents、/api/v1/agents/{name}：Agent 加载
//   - /api/v1/skills、/api/v1/skills/{name}、/api/v1/skills/{name}/download：SKILL 配置与内容
//   - /api/v1/scenes：公开场景列表（见下方追加条目）
//
// 显式排除：mcps、admin/*、providers/:id 及一切非 GET；chat 端点树不在本表，
// 由 matchesGuestChatPath 前缀放行（见 spec；与 cmd/server/main.go 实际注册
// 路由逐条对应）。POST /api/v1/chat/push 为唯一 guest 写操作放行，由
// matchesChatPushPath 独立处理（归属安全由 service/repo 强制覆写保证，issue #155）。
// 新增放行端点时必须新增对应回归用例。
var pendingConfigPatterns = []pathPattern{
	{http.MethodGet, []string{"", "api", "v1", "providers"}},
	{http.MethodGet, []string{"", "api", "v1", "providers", "runtime-config"}},
	{http.MethodGet, []string{"", "api", "v1", "agents", "manifest"}},
	{http.MethodGet, []string{"", "api", "v1", "agents"}},
	{http.MethodGet, []string{"", "api", "v1", "agents", "{}"}},
	{http.MethodGet, []string{"", "api", "v1", "skills"}},
	{http.MethodGet, []string{"", "api", "v1", "skills", "{}"}},
	{http.MethodGet, []string{"", "api", "v1", "skills", "{}", "download"}},
	// guest 可读公开场景列表（SceneWelcome 依赖；场景内容仍经 service 层
	// guest 过滤，见 spec 4.3）。
	{http.MethodGet, []string{"", "api", "v1", "scenes"}},
}

// matchesPendingConfig 报告 method+path 是否命中 pending 配置白名单。
// 段级比较：段数必须相等；"{}" 只匹配非空单段（防空段绕过）；具体段
// 精确相等。gin 对未注册尾斜杠形式的请求会 301 到无斜杠路径（生产中
// guard 看不到尾斜杠）；若尾斜杠仍到达（例如测试直连注册含尾斜杠的
// 路由），尾段空串按段失配处理——失败闭合 403，不得静默归并成合法路径。
func matchesPendingConfig(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	seg := strings.Split(path, "/")
	for _, p := range pendingConfigPatterns {
		if len(seg) != len(p.segments) {
			continue
		}
		match := true
		for i := range seg {
			if p.segments[i] == "{}" {
				if seg[i] == "" {
					match = false
					break
				}
				continue
			}
			if seg[i] != p.segments[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// guestChatPrefix 是聊天端点树的段级前缀：/api/v1/agents/{name}/chat/...。
// "{}" 通配恰好一个非空段；后续任意段（含空尾段防御——gin 301 收敛后
// 生产看不到尾斜杠，仍到达时按失配处理失败闭合）。
var guestChatPrefix = []string{"", "api", "v1", "agents", "{}", "chat"}

// matchesGuestChatPath 报告 path 是否落在聊天端点树内（不限 method：
// sessions 增删查 / messages 读写 SSE / capabilities / uploads / attachments）。
func matchesGuestChatPath(path string) bool {
	seg := strings.Split(path, "/")
	if len(seg) < len(guestChatPrefix) {
		return false
	}
	for i := range guestChatPrefix {
		if guestChatPrefix[i] == "{}" {
			if seg[i] == "" {
				return false
			}
			continue
		}
		if seg[i] != guestChatPrefix[i] {
			return false
		}
	}
	return true
}

// matchesChatPushPath 报告 method+path 是否命中桌面会话上传端点
// POST /api/v1/chat/push（issue #155：guest 体验账号允许同步自己的会话）。
// 写操作必须 method 校验：仅 POST 放行；path 做段级精确匹配
// ["", "api", "v1", "chat", "push"]——段数严格相等，尾斜杠尾段为空串
// 按失配处理（fail-closed 403），不做前缀模糊放行
// （/api/v1/chat/anything、/api/v1/chat/pushx 均 403）。
// 与 cmd/server/main.go 实际注册路由（chatPushGroup.POST("/push")）逐段对应。
func matchesChatPushPath(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	want := []string{"", "api", "v1", "chat", "push"}
	seg := strings.Split(path, "/")
	if len(seg) != len(want) {
		return false
	}
	for i := range seg {
		if seg[i] != want[i] {
			return false
		}
	}
	return true
}

// IsGuest reports whether the caller is an "effective guest": an explicit
// guest role (any auth method) or empty roles with casdoor/cli auth.
// 正式角色（admin/maintainer/member）、builtin 空 roles（防御放行）、
// 未注入 keys（误挂）均返回 false。
func IsGuest(c *gin.Context) bool {
	rolesVal, _ := c.Get("roles")
	roles, _ := rolesVal.([]string)
	for _, r := range roles {
		if r == authdom.RoleGuest {
			return true
		}
	}
	if len(roles) > 0 {
		return false
	}
	methodVal, _ := c.Get("auth_method")
	m, _ := methodVal.(string)
	return m == "casdoor" || m == "cli"
}

// GuestGuard 拦截"有效 guest"的非白名单请求（原 PendingApprovalGuard 的
// guest 语义重构，spec 3.2）。行为矩阵：
//   - 非 guest（正式角色 / builtin 空 roles / keys 未注入）→ 放行；
//   - guest + 白名单（原有精确路径、health、GET 配置端点、GET scenes、
//     聊天端点树、POST /api/v1/chat/push 自身会话上传 #155）→ 放行；
//   - guest + 其他路径 → 403，error 以 PENDING_APPROVAL 开头（wire 契约保留）。
func GuestGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsGuest(c) {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if _, ok := pendingWhitelistExact[path]; ok ||
			strings.HasPrefix(path, healthPathPrefix) ||
			matchesPendingConfig(c.Request.Method, path) ||
			matchesGuestChatPath(path) ||
			matchesChatPushPath(c.Request.Method, path) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   pendingApprovalMarker + ": 体验账号无管理权限，可前往 Agent 聊天页继续体验",
		})
	}
}
