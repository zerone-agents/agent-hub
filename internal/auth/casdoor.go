package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"control-panel/internal/config"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
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
}

var (
	oauthSessions   = make(map[string]*OAuthSession)
	oauthSessionsMu sync.RWMutex
)

// InitCasdoor initializes the global Casdoor client with the provided configuration.
func InitCasdoor(cfg *config.CasdoorConfig) error {
	client = casdoorsdk.NewClient(
		cfg.Endpoint,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.Certificate,
		cfg.Organization,
		"middle-ground",
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
		"middle-ground",
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

// GenerateCodeChallenge creates a PKCE S256 code challenge from the verifier.
func GenerateCodeChallenge(verifier string) string {
	return generateCodeChallenge(verifier)
}

// GetLoginURL builds the Casdoor authorization URL and stores the OAuth session.
func GetLoginURL(state string, codeVerifier string) string {
	callbackURL := GetCallbackURL()

	params := url.Values{}
	params.Set("client_id", casdoorConfig.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", callbackURL)
	params.Set("scope", "read")
	params.Set("state", state)

	if codeVerifier != "" {
		params.Set("code_challenge", GenerateCodeChallenge(codeVerifier))
		params.Set("code_challenge_method", "S256")
	}

	loginURL := fmt.Sprintf("%s/login/oauth/authorize?%s", casdoorConfig.Endpoint, params.Encode())

	oauthSessionsMu.Lock()
	oauthSessions[state] = &OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
	}
	oauthSessionsMu.Unlock()

	log.Printf("[OAuth] Stored session: state=%s, sessions count=%d", state, len(oauthSessions))

	return loginURL
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

// ExchangeCodeForToken exchanges an authorization code for access and refresh tokens.
func ExchangeCodeForToken(code, codeVerifier string) (*TokenResponse, error) {
	callbackURL := GetCallbackURL()

	tokenURL := fmt.Sprintf("%s/api/login/oauth/access_token", client.Endpoint)

	data := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     client.ClientId,
		"client_secret": client.ClientSecret,
		"callback_url":  callbackURL,
	}

	if codeVerifier != "" {
		data["code_verifier"] = codeVerifier
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token request: %w", err)
	}

	resp, err := http.Post(tokenURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 记录调试信息
	log.Printf("Token exchange response status: %d", resp.StatusCode)
	log.Printf("Token exchange response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return nil, fmt.Errorf("casdoor error: %v", errorResp)
		}
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("access token is empty in response")
	}

	return &tokenResp, nil
}

// GetUserInfo extracts and validates user information from an access token.
func GetUserInfo(accessToken string) (*casdoorsdk.User, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access token is empty")
	}

	claims, err := client.ParseJwtToken(accessToken)
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

// CreateOrganization creates a new Casdoor organization.
func CreateOrganization(name, displayName, owner string) error {
	org := &casdoorsdk.Organization{
		Name:        name,
		DisplayName: displayName,
		Owner:       owner,
	}

	_, err := client.AddOrganization(org)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	return nil
}

// DeleteOrganization deletes a Casdoor organization by name.
func DeleteOrganization(name string) error {
	_, err := client.DeleteOrganization(&casdoorsdk.Organization{Name: name})
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	return nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成随机字符串失败: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b)[:length], nil
}

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
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

// RefreshAccessToken exchanges a refresh token for a new access token.
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is empty")
	}

	tokenURL := fmt.Sprintf("%s/api/login/oauth/access_token", client.Endpoint)

	data := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     client.ClientId,
		"client_secret": client.ClientSecret,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	resp, err := http.Post(tokenURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return &tokenResp, nil
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
