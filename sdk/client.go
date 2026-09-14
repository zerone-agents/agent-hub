// Package sdk 是 Agent Hub H7 扩展管理 API 的 Go 客户端库（H7.6）。
//
// 面向扩展开发者与运维脚本：以管理员 token 调用
// /api/v1/admin/extensions 全组端点（注册/列表/详情/安装/启停/升级/
// 回滚/卸载/影响面）。服务端中文错误原样透传，HTTP 状态码映射为
// 类型化错误（ErrNotFound / ErrConflict / ErrValidation / ErrPermission），
// 便于调用方程序化分支。零外部依赖，仅标准库。
package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ExtensionAdminClient 是扩展管理 API 客户端。零值不可用，
// 请用 NewExtensionAdminClient 构造。并发安全。
type ExtensionAdminClient struct {
	// Server 形如 http://localhost:8080（不含 /api/v1 前缀）。
	Server string
	// Token 是管理员 Bearer token（cli_* 或 JWT）。
	Token string
	// Tenant 可选；非空时经 X-Tenant-Id 头透传。
	Tenant string
	// HTTPClient 可替换；默认 15s 超时。
	HTTPClient *http.Client
}

// NewExtensionAdminClient 构造客户端；server 可省略协议前缀时补 http://。
func NewExtensionAdminClient(server, token string) *ExtensionAdminClient {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server != "" && !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "http://" + server
	}
	return &ExtensionAdminClient{
		Server:     server,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// APIError 是服务端错误的类型化映射；Message 为服务端中文错误原样透传。
type APIError struct {
	StatusCode int
	Code       string // 机器可读 code（服务端提供时）
	Message    string // 中文错误信息
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d：%s", e.StatusCode, e.Message)
}

// Is 让 errors.Is(err, sdk.ErrNotFound) 等按状态码匹配类别，
// 信息保持服务端原文。
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	return ok && e.StatusCode == t.StatusCode
}

// 预定义错误类别，errors.As 匹配 *APIError 后按 StatusCode 分支，
// 或直接 errors.Is(err, sdk.ErrNotFound)。
var (
	ErrNotFound    = &APIError{StatusCode: 404, Message: "资源不存在"}
	ErrConflict    = &APIError{StatusCode: 409, Message: "资源冲突"}
	ErrValidation  = &APIError{StatusCode: 400, Message: "请求校验失败"}
	ErrPermission  = &APIError{StatusCode: 403, Message: "权限不足"}
	ErrUnauthorized = &APIError{StatusCode: 401, Message: "未认证"}
)

// RegisterInput 是注册扩展的请求：Manifest 为严格 v1 manifest（JSON 字节，
// 任意排版，服务端会规范化）。Signature/PublicKey 可选，成对提供时服务端
// 验签通过才注册，公钥指纹记入版本 signed_by。
type RegisterInput struct {
	Manifest  json.RawMessage
	Signature string
	PublicKey string
	Source    string
	Changelog string
}

// Register 注册扩展 + 版本（POST /api/v1/admin/extensions）。
func (c *ExtensionAdminClient) Register(in RegisterInput) (json.RawMessage, error) {
	var body []byte
	if in.Signature != "" || in.PublicKey != "" {
		env := map[string]any{"manifest": in.Manifest}
		if in.Signature != "" {
			env["signature"] = in.Signature
		}
		if in.PublicKey != "" {
			env["public_key"] = in.PublicKey
		}
		if in.Source != "" {
			env["source"] = in.Source
		}
		if in.Changelog != "" {
			env["changelog"] = in.Changelog
		}
		body, _ = json.Marshal(env)
	} else {
		body = in.Manifest
	}
	return c.do(http.MethodPost, "/api/v1/admin/extensions?"+registerQuery(in), body)
}

func registerQuery(in RegisterInput) string {
	q := url.Values{}
	if in.Source != "" && in.Signature == "" {
		q.Set("source", in.Source)
	}
	if in.Changelog != "" && in.Signature == "" {
		q.Set("changelog", in.Changelog)
	}
	return q.Encode()
}

// List 扩展列表（GET /api/v1/admin/extensions）。
func (c *ExtensionAdminClient) List(status, source string, page, pageSize int) (json.RawMessage, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if source != "" {
		q.Set("source", source)
	}
	if page > 0 {
		q.Set("page", fmt.Sprint(page))
	}
	if pageSize > 0 {
		q.Set("pageSize", fmt.Sprint(pageSize))
	}
	return c.do(http.MethodGet, "/api/v1/admin/extensions?"+q.Encode(), nil)
}

// Get 扩展详情（GET /api/v1/admin/extensions/:id）。
func (c *ExtensionAdminClient) Get(id uint64) (json.RawMessage, error) {
	return c.do(http.MethodGet, fmt.Sprintf("/api/v1/admin/extensions/%d", id), nil)
}

// Install 安装扩展：POST /extensions/:id/install，body {"version":"x.y.z"}。
func (c *ExtensionAdminClient) Install(id uint64, version string) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]string{"version": version})
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", id), body)
}

// Enable 启用扩展（幂等）。
func (c *ExtensionAdminClient) Enable(id uint64) (json.RawMessage, error) {
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/enable", id), nil)
}

// Disable 停用扩展（幂等）。
func (c *ExtensionAdminClient) Disable(id uint64) (json.RawMessage, error) {
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/disable", id), nil)
}

// Upgrade 升级扩展：POST /extensions/:id/upgrades，body {"target_version":"x.y.z"}。
func (c *ExtensionAdminClient) Upgrade(id uint64, targetVersion string) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]string{"target_version": targetVersion})
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/upgrade", id), body)
}

// Rollback 回滚扩展：body 可空（默认回滚到上一版本）。
func (c *ExtensionAdminClient) Rollback(id uint64, targetVersion string) (json.RawMessage, error) {
	body := []byte("{}")
	if targetVersion != "" {
		body, _ = json.Marshal(map[string]string{"target_version": targetVersion})
	}
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/rollback", id), body)
}

// Uninstall 卸载扩展；force 先自动停用依赖方，purge 额外删除本租户版本数据。
func (c *ExtensionAdminClient) Uninstall(id uint64, force, purge bool) (json.RawMessage, error) {
	q := url.Values{}
	if force {
		q.Set("force", "true")
	}
	if purge {
		q.Set("purge", "true")
	}
	return c.do(http.MethodDelete, fmt.Sprintf("/api/v1/admin/extensions/%d/uninstall?%s", id, q.Encode()), nil)
}

// Impact 依赖影响面预览（卸载/升级前必查）。
func (c *ExtensionAdminClient) Impact(id uint64) (json.RawMessage, error) {
	return c.do(http.MethodGet, fmt.Sprintf("/api/v1/admin/extensions/%d/impact", id), nil)
}

// GetVersion 返回单版本完整 manifest。
func (c *ExtensionAdminClient) GetVersion(id uint64, version string) (json.RawMessage, error) {
	return c.do(http.MethodGet, fmt.Sprintf("/api/v1/admin/extensions/%d/versions/%s", id, url.PathEscape(version)), nil)
}

// envelope 是服务端统一响应包络：{success, data} 或 {success:false, error, code}。
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Code    string          `json:"code"`
}

// do 发请求并解包响应；错误映射为类型化 *APIError（中文信息透传）。
func (c *ExtensionAdminClient) do(method, path string, body []byte) (json.RawMessage, error) {
	if c.Server == "" {
		return nil, fmt.Errorf("Server 不能为空")
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.Server+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Tenant != "" {
		req.Header.Set("X-Tenant-Id", c.Tenant)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s %s 失败：%v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode >= 400 || !env.Success {
		apiErr := mapAPIError(resp.StatusCode, env.Error, env.Code)
		return nil, apiErr
	}
	return env.Data, nil
}

// mapAPIError 把 HTTP 状态码映射为预定义错误类别（信息替换为服务端原文）。
func mapAPIError(status int, message, code string) *APIError {
	base := &APIError{StatusCode: status, Message: message, Code: code}
	switch status {
	case 401:
		base.Message = defaultMsg(message, ErrUnauthorized.Message)
	case 403:
		base.Message = defaultMsg(message, ErrPermission.Message)
	case 404:
		base.Message = defaultMsg(message, ErrNotFound.Message)
	case 409:
		base.Message = defaultMsg(message, ErrConflict.Message)
	case 400:
		base.Message = defaultMsg(message, ErrValidation.Message)
	}
	return base
}

func defaultMsg(msg, fallback string) string {
	if strings.TrimSpace(msg) == "" {
		return fallback
	}
	return msg
}
