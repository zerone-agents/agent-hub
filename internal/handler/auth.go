package handler

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"control-panel/internal/auth"

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
func Callback(provider *auth.CasdoorProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "code 和 state 参数必填",
			})
			return
		}

		session := auth.GetSession(state)
		if session == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "无效的 state 参数或会话已过期",
			})
			return
		}

		codeVerifier := session.CodeVerifier

		tokenResp, err := auth.ExchangeCodeForToken(session.Org, code, codeVerifier)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Token 交换失败: " + err.Error(),
			})
			return
		}

		if user, err := auth.GetUserInfo(tokenResp.AccessToken); err == nil {
			// 同步成员记录（Admin API 拉权威 IsAdmin 合成 + 落库）；
			// 失败仅记日志，不阻断登录。
			if _, err := provider.SyncMembership(user); err != nil {
				log.Printf("[Callback] sync membership failed: %v", err)
			}
		}

		redirectURL := "/static/?token=" + url.QueryEscape(tokenResp.AccessToken)
		if tokenResp.RefreshToken != "" {
			redirectURL += "&refreshToken=" + url.QueryEscape(tokenResp.RefreshToken)
		}

		log.Printf("[Callback] Redirecting with tokens to /static/")
		c.Redirect(http.StatusFound, redirectURL)
	}
}

// UserInfo returns the authenticated user's profile information.
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
			"tenant_id":    "",
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
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "failed to refresh token: " + err.Error(),
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
func Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != authHeader && token != "" {
		auth.RevokeToken(token)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "logged out successfully",
	})
}
