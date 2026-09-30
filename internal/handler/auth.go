package handler

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// Login initiates the OAuth flow by redirecting to the Casdoor login page.
func Login(c *gin.Context) {
	state, err := auth.GenerateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "生成 state 失败",
		})
		return
	}
	codeVerifier, err := auth.GenerateCodeVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "生成 code verifier 失败",
		})
		return
	}
	org := strings.TrimSpace(c.Query("org"))
	redirect := auth.SanitizeRedirect(strings.TrimSpace(c.Query("redirect")))
	loginURL, err := auth.GetLoginURL(org, state, codeVerifier, redirect)
	if err != nil {
		// 未注册/不存在的组织统一文案，不区分两种情况（避免探测）。
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "组织未注册或不存在，请联系平台管理员",
		})
		return
	}

	log.Printf("[Login] Generated: state=%s, redirecting to=%s", state, loginURL)

	c.Redirect(http.StatusFound, loginURL)
}

// Callback handles the OAuth callback, exchanging the code for tokens.
// 登录成功时经 provider.SyncMembership 合成成员角色并落库（本地成员表）；
// 同步失败仅记日志，不阻断登录（保持宽松语义，待审批由下游中间件拦截）。
// 审计按签发结果逐阶段归属（spec §5.6）：成功行的租户取 token 解析出的权威
// org（user.Owner），保证默认组织登录（session.Org 空）也正确落库；token
// 解析失败时回退 session.Org，仍为空才 stdout-only（空租户不落库）。
func Callback(provider *auth.CasdoorProvider, ar *services.AuditRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			ar.Login(c, "", "", "", audit.StatusFailure, audit.ReasonInvalidRequest) // org 未知 → stdout-only
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "code 和 state 参数必填",
			})
			return
		}

		session := auth.GetSession(state)
		if session == nil {
			ar.Login(c, "", "", "", audit.StatusFailure, audit.ReasonInvalidRequest) // org 未知 → stdout-only
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "无效的 state 参数或会话已过期",
			})
			return
		}

		codeVerifier := session.CodeVerifier

		tokenResp, err := auth.ExchangeCodeForToken(session.Org, code, codeVerifier)
		if err != nil {
			// 细节进日志（可能含 casdoor 原始响应/内部地址），客户端只见中性文案。
			log.Printf("[Callback] token exchange failed (org=%s): %v", session.Org, err)
			ar.Login(c, "", "", session.Org, audit.StatusFailure, audit.ReasonTokenIssuanceFailed) // org 已知 → 落库（签发失败，spec §3.1）
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "登录回调处理失败，请重试",
			})
			return
		}

		auditName, auditUserID, auditOrg := "", "", session.Org
		if user, err := auth.GetUserInfo(tokenResp.AccessToken); err == nil {
			auditName = user.Name
			auditUserID = user.Id
			// 审计租户取 token 解析出的权威组织（user.Owner）：默认组织登录
			// （发起 URL 无 ?org=、session.Org 为空）同样正确归属并落库，与
			// 后续已认证请求（租户同样来自 token owner）保持同一租户视图。
			if user.Owner != "" {
				auditOrg = user.Owner
			}
			// 同步成员记录（Admin API 拉权威 IsAdmin 合成 + 落库）；
			// 失败仅记日志，不阻断登录。
			if _, err := provider.SyncMembership(user); err != nil {
				log.Printf("[Callback] sync membership failed: %v", err)
			}
		}
		// GetUserInfo 失败但 token 已下发并 redirect（现有放行行为）→ success
		// （会话已签发，照实记录，spec §5.6）；此时租户回退 session.Org，
		// 仍为空则 stdout-only（不变）。
		ar.Login(c, auditUserID, auditName, auditOrg, audit.StatusSuccess, "")

		redirectURL := buildCallbackRedirect(session.Redirect, tokenResp.AccessToken, tokenResp.RefreshToken)

		log.Printf("[Callback] Redirecting with tokens to %s", session.Redirect)
		c.Redirect(http.StatusFound, redirectURL)
	}
}

// buildCallbackRedirect 构造落地 URL："/static"+redirect 的 query/hash 保留
// （伪造的 token/refreshToken query 参数先剥离），签发的凭证只写入 URL
// fragment。fragment 不会被浏览器发往服务器，因此 token 不进网关/代理的
// access log、不受请求行长度限制（JWT 随用户数据增长，issue #185）；前端
// consumeAuthParams 同时兼容新 fragment 与旧 query 两种载体。解析异常回退
// "/static/"（失败闭合）。
func buildCallbackRedirect(redirectPath, accessToken, refreshToken string) string {
	u, err := url.Parse("/static" + redirectPath)
	if err != nil || !strings.HasPrefix(u.Path, "/static") {
		u = &url.URL{Path: "/static/"}
	}
	q := u.Query()
	// redirect 自带的认证参数必须先剥离：token 会被下方 fragment 覆盖，但
	// refreshToken 仅在服务端签发时写回——不 Del 会让 crafted 链接的伪造
	// refreshToken 存活到落地 URL（会话固定边缘，final review Important）。
	q.Del("token")
	q.Del("refreshToken")
	u.RawQuery = q.Encode()

	creds := url.Values{}
	creds.Set("token", accessToken)
	if refreshToken != "" {
		creds.Set("refreshToken", refreshToken)
	}
	// redirect 自带 hash 保留（如 "f"），但必须先按 "&" 分段剥离其中的伪造
	// token/refreshToken 段：真实凭证续接在末尾，前端 URLSearchParams
	// first-wins 会取到伪造值（会话固定，review Important，与 query 的
	// q.Del 对等）。不用 url.Values 重建——那会把裸 hash "f" 变成 "f="。
	segments := make([]string, 0, 2)
	if u.Fragment != "" {
		for _, seg := range strings.Split(u.Fragment, "&") {
			if seg == "" || strings.HasPrefix(seg, "token=") || strings.HasPrefix(seg, "refreshToken=") {
				continue
			}
			segments = append(segments, seg)
		}
	}
	segments = append(segments, creds.Encode())
	u.Fragment = strings.Join(segments, "&")
	return u.String()
}

// UserInfo returns the authenticated user's profile information.
// tenant_id 是权威字段（取自中间件经 tenant.SetTenantID 注入的租户上下文）；
// org_id 为向后兼容保留的同源值（builtin 模式下均为 "default"）。
func UserInfo(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userName, _ := c.Get("user_name")
	email, _ := c.Get("email")
	displayName, _ := c.Get("display_name")
	orgID, _ := c.Get("org_id")
	avatar, _ := c.Get("avatar")
	roles, _ := c.Get("roles")
	permissions, _ := c.Get("permissions")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":           userID,
			"username":     userName,
			"email":        email,
			"display_name": displayName,
			"tenant_id":    tenant.GetTenantID(c),
			"org_id":       orgID,
			"avatar":       avatar,
			"roles":        roles,
			"permissions":  permissions,
		},
	})
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(c *gin.Context) {
	var request struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "refresh token is required",
		})
		return
	}

	tokenResp, err := auth.RefreshAccessToken(request.RefreshToken)
	if err != nil {
		// 中性文案：内部细节（endpoint/网络错误原文）只进日志，不外泄给客户端
		// （镜像 PR #65 Callback 的处理模式）。
		log.Printf("[RefreshToken] refresh failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "刷新令牌无效或已过期，请重新登录",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"accessToken":  tokenResp.AccessToken,
			"refreshToken": tokenResp.RefreshToken,
			"expiresIn":    tokenResp.ExpiresIn,
			"tokenType":    tokenResp.TokenType,
		},
	})
}

// Logout 审计语义（audit spec v20 修订）：casdoor 服务端不提供 token 撤销
// 接口——v3.60.1 / v4.7.0 / master 路由表核对均无 /api/login/oauth/revoke
// （上游 casdoor#1574 明确不实现非标准撤销），旧实现对不存在端点发请求、
// 每次登出必记 Failure。登出为客户端语义：客户端清除本地凭证，审计记
// 「登出请求已受理」success；撤销结果语义仅 builtin 模式保留（见
// auth_builtin.go Logout）。
func Logout(c *gin.Context, ar *services.AuditRecorder) {
	ar.Simple(c, audit.ActionLogout, audit.TargetSystem, "", "")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "logged out successfully",
	})
}
