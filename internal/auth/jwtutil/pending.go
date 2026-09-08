package jwtutil

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// pendingApprovalMarker 是 403 错误体 error 字符串的固定前缀，
// 前端据此做字符串匹配识别"待审批"状态并渲染等待审批页。
const pendingApprovalMarker = "PENDING_APPROVAL"

// pendingWhitelistExact 是待审批用户仍可访问的精确路径（对照 cmd/server/main.go 实际注册的路由）：
//   - /auth/userinfo：前端获取当前用户信息的 "me" 端点，等待审批页依赖它判定登录态与角色；
//   - /auth/logout：允许待审批用户退出登录；
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

// pendingConfigPatterns 是 pending（空角色）用户可读的桌面配置端点：
//   - /api/v1/providers、/api/v1/providers/runtime-config：桌面 provider 同步
//   - /api/v1/agents/manifest、/api/v1/agents、/api/v1/agents/{name}：Agent 加载
//   - /api/v1/skills、/api/v1/skills/{name}、/api/v1/skills/{name}/download：SKILL 配置与内容
//
// 显式排除：chat/*、scenes、mcps、admin/*、providers/:id 及一切非 GET
// （见 spec D1 清单；与 cmd/server/main.go 实际注册路由逐条对应）。
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

// PendingApprovalGuard 拦截"待审批"用户（角色为空的 casdoor / cli 用户）的非白名单请求。
// 必须挂在 AuthMiddlewareWithCLI 之后（同一 Use 链），依赖其注入的 gin context keys：
//   - roles（[]string，空切片 = 待审批）
//   - auth_method（"builtin" | "casdoor" | "cli"）
//
// 行为：
//   - roles 非空 → 放行（正常用户）；
//   - roles 为空 且 auth_method ∈ {casdoor, cli} 且路径不在白名单
//     → 403 + Abort。错误体与 handler.respondError 形状一致
//     （{"success":false,"error":"..."}，error 为字符串），并以
//     "PENDING_APPROVAL" 开头供前端识别；
//   - roles 为空 且 auth_method=builtin → 放行（builtin 本地用户必有角色，
//     无待审批概念，防御性不拦，保证 builtin 模式全链路零感知）；
//   - auth_method 未设置（guard 被误挂到未鉴权路由）→ 防御性放行。
func PendingApprovalGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 正常用户：有角色直接放行，最常见路径优先判断。
		if rolesVal, ok := c.Get("roles"); ok {
			if roles, _ := rolesVal.([]string); len(roles) > 0 {
				c.Next()
				return
			}
		}

		methodVal, _ := c.Get("auth_method")
		authMethod, _ := methodVal.(string)
		// 仅拦 casdoor / cli；builtin 与未设置 auth_method 的请求防御性放行。
		if authMethod != "casdoor" && authMethod != "cli" {
			c.Next()
			return
		}

		// 白名单放行：既有精确路径/前缀 + issue #132 配置端点（仅 GET）。
		path := c.Request.URL.Path
		if _, ok := pendingWhitelistExact[path]; ok || strings.HasPrefix(path, healthPathPrefix) || matchesPendingConfig(c.Request.Method, path) {
			c.Next()
			return
		}

		// 待审批用户访问非白名单路径：403 并终止后续链。
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   pendingApprovalMarker + ": 账号待审批，请联系管理员分配角色",
		})
	}
}
