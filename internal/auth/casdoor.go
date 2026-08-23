package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"control-panel/internal/config"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"golang.org/x/oauth2"
)

var client *casdoorsdk.Client
var casdoorConfig *config.CasdoorConfig

// clientsByOrg 按组织缓存 SDK client：多租户下 directory 层需要以请求租户
// （casdoor organization）为 owner 调用管理 API，endpoint/凭证/证书共用，
// 仅 OrganizationName 不同。
var (
	clientsByOrg   = make(map[string]*casdoorsdk.Client)
	clientsByOrgMu sync.Mutex
)

type OAuthSession struct {
	State        string
	CodeVerifier string
	Org          string
}

// TenantClientCreds 是某个组织（casdoor organization）对应的 OAuth client
// 凭证。CertPEM 为空串 = 该组织用全局 CASDOOR_CERTIFICATE 验签。
type TenantClientCreds struct {
	ClientID     string
	ClientSecret string
	CertPEM      string
}

// tenantClientLookup 由 main.go 注入（auth 不能反向依赖 persistence 层）：
// 按组织名查 tenant_oauth_clients 并解密凭证；org="" 语义为查 default 行。
// 未注册/无 default/查询出错均返回 false（fail closed）。
var tenantClientLookup func(org string) (*TenantClientCreds, bool)

// SetTenantClientLookup 注入（或传 nil 清除）组织凭证查询回调。
func SetTenantClientLookup(fn func(org string) (*TenantClientCreds, bool)) {
	tenantClientLookup = fn
}

var (
	oauthSessions   = make(map[string]*OAuthSession)
	oauthSessionsMu sync.RWMutex
)

// applicationNameUnused：SDK 构造要求传 applicationName，但我们使用的链路
// （自拼 OAuth 授权 URL、token 兑换/吊销、证书验签、组织作用域的管理 API）
// 都不消费该值——Casdoor 用 client_id 反查应用。传空串即可，不设配置项，
// 避免"看起来要配但实际无效"的误导。
const applicationNameUnused = ""

// InitCasdoor initializes the global Casdoor client with the provided configuration.
func InitCasdoor(cfg *config.CasdoorConfig) error {
	client = casdoorsdk.NewClient(
		cfg.Endpoint,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.Certificate,
		cfg.Organization,
		applicationNameUnused,
	)
	casdoorConfig = cfg

	log.Println("Casdoor initialized successfully")
	return nil
}

// GetClient returns the initialized Casdoor client instance.
func GetClient() *casdoorsdk.Client {
	return client
}

// ClientForOrg returns a cached SDK client whose organization scope is org.
// Falls back to the configured default organization when org is empty.
// Must be called after InitCasdoor.
func ClientForOrg(org string) *casdoorsdk.Client {
	if org == "" {
		return client
	}
	clientsByOrgMu.Lock()
	defer clientsByOrgMu.Unlock()
	if c, ok := clientsByOrg[org]; ok {
		return c
	}
	c := casdoorsdk.NewClient(
		casdoorConfig.Endpoint,
		casdoorConfig.ClientID,
		casdoorConfig.ClientSecret,
		casdoorConfig.Certificate,
		org,
		applicationNameUnused,
	)
	clientsByOrg[org] = c
	return c
}

// GenerateState generates a random OAuth state parameter.
func GenerateState() (string, error) {
	return generateRandomString(32)
}

// GenerateCodeVerifier generates a random PKCE code verifier.
func GenerateCodeVerifier() (string, error) {
	return generateRandomString(32)
}

// resolveClientCreds 按 org 解析 OAuth 凭证（多组织登录入口解析链）：
//   - org != ""：查注入的 lookup，未命中返回错误（调用方转 404 统一文案，
//     绝不回落全局，避免把请求发给错误组织的 application）；
//   - org == ""：查 default 行；表空（无 default）→ 全局 env client
//     （存量单组织部署兜底）。
func resolveClientCreds(org string) (*TenantClientCreds, error) {
	if org != "" {
		if tenantClientLookup == nil {
			return nil, fmt.Errorf("组织未注册或不存在，请联系平台管理员")
		}
		creds, ok := tenantClientLookup(org)
		if !ok || creds == nil {
			return nil, fmt.Errorf("组织未注册或不存在，请联系平台管理员")
		}
		return creds, nil
	}
	// org 为空：default 行优先，无则全局 env client。
	if tenantClientLookup != nil {
		if creds, ok := tenantClientLookup(""); ok && creds != nil {
			return creds, nil
		}
	}
	return &TenantClientCreds{
		ClientID:     casdoorConfig.ClientID,
		ClientSecret: casdoorConfig.ClientSecret,
	}, nil
}

// perOrgJWTClient 返回用指定组织的证书构建的 SDK client（certPEM 空则用
// 全局 client），供 ParseJwtToken 验签：casdoorsdk.NewClient(endpoint,
// clientID, secret, certPEM, org, "")。owner 是 casdoor 签进 token 的
// 组织名，可信——签名本身验证了它。
func perOrgJWTClient(org string) *casdoorsdk.Client {
	if tenantClientLookup != nil {
		if creds, ok := tenantClientLookup(org); ok && creds != nil && creds.CertPEM != "" {
			return casdoorsdk.NewClient(
				casdoorConfig.Endpoint,
				creds.ClientID,
				creds.ClientSecret,
				creds.CertPEM,
				org,
				applicationNameUnused,
			)
		}
	}
	return client
}

// tokenOwner 不经验签地读 JWT payload 的 owner 字段（仅用于选择验签证书，
// 安全性仍由 ParseJwtToken 的签名校验保证；篡改 owner 只会导致选错证书而
// 验签失败）。
func tokenOwner(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims struct {
		Owner string `json:"owner"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Owner
}

// oauthConfig 由租户凭证现构 x/oauth2 配置（凭证 per-org，无全局单例）。
// AuthURL 保持既有前端路由 /login/oauth/authorize（SDK 用 /api/... 变体，
// 两者等价，不改变现有行为）。redirectURL 非空时写入 Config.RedirectURL：
// 授权 URL 需要它；token 兑换传空——SDK 先例（auth.go RedirectURL 被注释），
// Casdoor 兑换端点不校验 redirect_uri，不发最稳。AuthStyleInParams =
// client_id/client_secret 放请求体（与原 JSON body 行为一致）。
func oauthConfig(creds *TenantClientCreds, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   fmt.Sprintf("%s/login/oauth/authorize", casdoorConfig.Endpoint),
			TokenURL:  fmt.Sprintf("%s/api/login/oauth/access_token", casdoorConfig.Endpoint),
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
	}
}

// GetLoginURL builds the Casdoor authorization URL for the given org and
// stores the OAuth session (with Org) for the callback.
func GetLoginURL(org, state, codeVerifier string) (string, error) {
	creds, err := resolveClientCreds(org)
	if err != nil {
		return "", err
	}
	callbackURL := GetCallbackURL()

	cfg := oauthConfig(creds, callbackURL)
	cfg.Scopes = []string{"read"}
	// 注：x/oauth2 v0.36.0 的 option 类型名为 AuthCodeOption（AuthCodeURLOption
	// 是更新版本引入的别名），升级依赖时无需改动此处语义。
	opts := []oauth2.AuthCodeOption{}
	if codeVerifier != "" {
		opts = append(opts, oauth2.S256ChallengeOption(codeVerifier))
	}
	loginURL := cfg.AuthCodeURL(state, opts...)

	oauthSessionsMu.Lock()
	oauthSessions[state] = &OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		Org:          org,
	}
	oauthSessionsMu.Unlock()

	log.Printf("[OAuth] Stored session: state=%s, org=%q, sessions count=%d", state, org, len(oauthSessions))

	return loginURL, nil
}

// GetCallbackURL returns the configured OAuth callback URL.
func GetCallbackURL() string {
	if casdoorConfig != nil && casdoorConfig.CallbackURL != "" {
		return casdoorConfig.CallbackURL
	}
	return ""
}

// TokenResponse represents an OAuth token exchange response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IdToken      string `json:"id_token"`
}

// tokenResponseFrom 把 oauth2.Token 折回既有 TokenResponse（对外 JSON 形状不变）。
// expiresIn 从 Expiry 折算；id_token 经 Extra 透传。
func tokenResponseFrom(tok *oauth2.Token) (*TokenResponse, error) {
	if tok.AccessToken == "" {
		return nil, errors.New("access token is empty in response")
	}
	expiresIn := 0
	if d := time.Until(tok.Expiry); d > 0 {
		expiresIn = int(d.Seconds())
	}
	resp := &TokenResponse{
		AccessToken:  tok.AccessToken,
		TokenType:    tok.TokenType,
		ExpiresIn:    expiresIn,
		RefreshToken: tok.RefreshToken,
	}
	if idToken, ok := tok.Extra("id_token").(string); ok {
		resp.IdToken = idToken
	}
	return resp, nil
}

// casdoorFakeSuccessError 提取 Casdoor 伪成功（200 + access_token 字面
// "error: xxx"）的真实错误信息。SDK auth.go:104 同款守卫——Casdoor 部分
// 错误路径返回 200，body 的 access_token 字段是 "error: <描述>"。
func casdoorFakeSuccessError(accessToken string) error {
	if strings.HasPrefix(accessToken, "error:") {
		return errors.New(strings.TrimSpace(strings.TrimPrefix(accessToken, "error:")))
	}
	return nil
}

// ExchangeCodeForToken exchanges an authorization code for tokens using the
// OAuth client resolved for org（x/oauth2 迁移，issue #49）。
func ExchangeCodeForToken(org, code, codeVerifier string) (*TokenResponse, error) {
	creds, err := resolveClientCreds(org)
	if err != nil {
		return nil, err
	}
	cfg := oauthConfig(creds, "") // 兑换不发 redirect_uri（见 oauthConfig 注释）
	opts := []oauth2.AuthCodeOption{}
	if codeVerifier != "" {
		opts = append(opts, oauth2.VerifierOption(codeVerifier))
	}
	tok, err := cfg.Exchange(context.Background(), code, opts...)
	if err != nil {
		// *oauth2.RetrieveError 结构化携带 OAuth 标准错误体（issue #49 的核心收益）
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			return nil, fmt.Errorf("casdoor token error: %s %s", re.ErrorCode, re.ErrorDescription)
		}
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	if ferr := casdoorFakeSuccessError(tok.AccessToken); ferr != nil {
		return nil, ferr
	}
	return tokenResponseFrom(tok)
}

// GetUserInfo extracts and validates user information from an access token.
func GetUserInfo(accessToken string) (*casdoorsdk.User, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access token is empty")
	}

	claims, err := perOrgJWTClient(tokenOwner(accessToken)).ParseJwtToken(accessToken)
	if err != nil {
		return nil, fmt.Errorf("token校验失败: %w", err)
	}

	if claims.Name == "" {
		return nil, fmt.Errorf("token中未包含用户信息")
	}

	return &claims.User, nil
}

// ValidateToken validates an access token and returns the associated user.
func ValidateToken(token string) (*casdoorsdk.User, error) {
	return GetUserInfo(token)
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成随机字符串失败: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b)[:length], nil
}

// GetSession retrieves and removes an OAuth session by state.
func GetSession(state string) *OAuthSession {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	session, exists := oauthSessions[state]
	log.Printf("[OAuth] GetSession: state=%s, exists=%v", state, exists)
	if exists {
		delete(oauthSessions, state)
		return session
	}
	return nil
}

// refreshCreds 为 refresh token 解析 OAuth 凭证：casdoor 的 refresh token 是
// JWT，payload 携带 owner（签发组织）。此处用 tokenOwner 不经验签地读 owner——
// 安全论证：owner 仅用于"选择哪组 client 凭证"，真正的安全边界是 Casdoor 校验
// refresh token 本身 + client_secret（拿错凭证刷新会被 Casdoor 拒绝，篡改
// owner 至多导致刷新失败，不会绕过认证）。owner 命中 lookup → 用该组织凭证；
// 否则回退全局解析链（default 行 → env 凭证），尽力恢复会话。
func refreshCreds(refreshToken string) *TenantClientCreds {
	if owner := tokenOwner(refreshToken); owner != "" && tenantClientLookup != nil {
		if creds, ok := tenantClientLookup(owner); ok && creds != nil {
			return creds
		}
	}
	if creds, err := resolveClientCreds(""); err == nil {
		return creds
	}
	return &TenantClientCreds{ClientID: client.ClientId, ClientSecret: client.ClientSecret}
}

// RefreshAccessToken exchanges a refresh token for a new access token.
// 多组织部署下按 token 的 owner 自解析凭证（见 refreshCreds），签名不变。
// x/oauth2 TokenSource 迁移（issue #49）：非 2xx / 错误体现返回 error
// （原实现无状态码检查，错误响应被当成功返回空 token——latent bug 一并修复）。
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	if refreshToken == "" {
		return nil, errors.New("refresh token is empty")
	}
	creds := refreshCreds(refreshToken)
	cfg := oauthConfig(creds, "")
	tok, err := cfg.TokenSource(context.Background(), &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			return nil, fmt.Errorf("casdoor token error: %s %s", re.ErrorCode, re.ErrorDescription)
		}
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	if ferr := casdoorFakeSuccessError(tok.AccessToken); ferr != nil {
		return nil, ferr
	}
	return tokenResponseFrom(tok)
}

// RevokeToken revokes an access or refresh token.
func RevokeToken(token string) error {
	if token == "" {
		return fmt.Errorf("token is empty")
	}

	revokeURL := fmt.Sprintf("%s/api/login/oauth/revoke", client.Endpoint)

	data := map[string]string{
		"client_id":     client.ClientId,
		"client_secret": client.ClientSecret,
		"token":         token,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal revoke request: %w", err)
	}

	resp, err := http.Post(revokeURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to revoke token: status %d", resp.StatusCode)
	}

	return nil
}
