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
	loginURL, err := auth.GetLoginURL(org, state, codeVerifier)
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
// 审计按签发结果逐阶段归属（spec §5.6）：org 未知阶段的失败 TenantID=""
// 由 Recorder 自动 stdout-only（空租户不落库），org 已知后正常落库。
func Callback(provider *auth.CasdoorProvider, ar *services.AuditRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			ar.Login(c, "", "", "", audit.StatusFailure, "") // org 未知 → stdout-only
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "code 和 state 参数必填",
			})
			return
		}

		session := auth.GetSession(state)
		if session == nil {
			ar.Login(c, "", "", "", audit.StatusFailure, "") // org 未知 → stdout-only
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
			ar.Login(c, "", "", session.Org, audit.StatusFailure, "") // org 已知 → 落库
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "登录回调处理失败，请重试",
			})
			return
		}

		loginName := ""
		if user, err := auth.GetUserInfo(tokenResp.AccessToken); err == nil {
			loginName = user.Name
			// 同步成员记录（Admin API 拉权威 IsAdmin 合成 + 落库）；
			// 失败仅记日志，不阻断登录。
			if _, err := provider.SyncMembership(user); err != nil {
				log.Printf("[Callback] sync membership failed: %v", err)
			}
		}
		// GetUserInfo 失败但 token 已下发并 redirect（现有放行行为）→ success
		// （会话已签发，照实记录，spec §5.6）。
		ar.Login(c, "", loginName, session.Org, audit.StatusSuccess, "")

		redirectURL := "/static/?token=" + url.QueryEscape(tokenResp.AccessToken)
		if tokenResp.RefreshToken != "" {
			redirectURL += "&refreshToken=" + url.QueryEscape(tokenResp.RefreshToken)
		}

		log.Printf("[Callback] Redirecting with tokens to /static/")
		c.Redirect(http.StatusFound, redirectURL)
	}
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

// Logout revokes the bearer token and returns a success response.
// 审计语义 = 凭证撤销结果（spec §3.1）：撤销返回错误 → failure（凭证可能
// 仍有效，如实记录）；HTTP 响应行为不变（恒 200）。
func Logout(c *gin.Context, ar *services.AuditRecorder) {
	authHeader := c.GetHeader("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	st := audit.StatusSuccess
	if token != authHeader && token != "" {
		if err := auth.RevokeToken(token); err != nil {
			st = audit.StatusFailure
		}
	}
	ar.SimpleWithStatus(c, audit.ActionLogout, audit.TargetSystem, "", "", st)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "logged out successfully",
	})
}
