// ExtensionSlotHandler 是 H7.2 UI 插槽管理 API 与授权 API 代理。
//
// 管理端点：
//   GET  /api/v1/admin/extensions/slots?slot=      聚合当前租户插槽内容
//   POST /api/v1/admin/extensions/:id/slots/:slot/visible  临时隐藏/恢复组件
//
// 代理端点（注册在 /api/v1/extensions/:name/*，非 admin 分组、登录即可用）：
// 只允许 GET；path 必须命中扩展 manifest 声明的 apiRoutes（声明时严格校验
// method=GET、/api/v1/extensions/{name}/ 前缀、http(s) upstream）。转发注入
// X-Extension-Name 与 X-Tenant-ID（来自 JWT 的租户上下文），响应限 512KB，
// 超时 5s，失败一律 502 中文错误。扩展不能借此执行任意代码或访问任意上游。
package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/extensionmanifest"
	"github.com/gin-gonic/gin"
)

// 代理安全边界常量。
const (
	extensionProxyMaxBody = 512 * 1024 // 响应限 512KB
	extensionProxyTimeout = 5 * time.Second
)

type ExtensionSlotHandler struct {
	slots  *services.ExtensionSlotService
	client *http.Client
	// secureClientFor 每次请求为 upstream 构造"校验 IP + 固定拨号"的
	// HTTP 客户端（默认 SecureHTTPClientForUpstream，测试可替换）。
	secureClientFor func(target string) (*http.Client, error)
}

func NewExtensionSlotHandler(s *services.ExtensionSlotService) *ExtensionSlotHandler {
	return &ExtensionSlotHandler{
		slots:           s,
		client:          &http.Client{Timeout: extensionProxyTimeout},
		secureClientFor: SecureHTTPClientForUpstream,
	}
}

// SecureHTTPClientForUpstream 解析并校验 upstream 的全部 IP，返回拨号
// 目标固定为这些 IP 的 HTTP 客户端（防 DNS rebinding TOCTOU 与跳转绕过）。
func SecureHTTPClientForUpstream(target string) (*http.Client, error) {
	base, err := url.Parse(strings.TrimSpace(target))
	if err != nil || base.Hostname() == "" {
		return nil, fmt.Errorf("upstream %q 必须是合法的 http(s) URL", target)
	}
	ips, err := extensionmanifest.ValidateAndResolveIPs(target)
	if err != nil {
		return nil, err
	}
	return extensionmanifest.SecureHTTPClient(ips, base.Hostname(), extensionProxyTimeout), nil
}

// ListSlots 聚合当前租户已启用扩展的插槽内容：GET /admin/extensions/slots?slot=。
func (h *ExtensionSlotHandler) ListSlots(c *gin.Context) {
	items, err := h.slots.List(tenant.GetTenantID(c), c.Query("slot"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": items})
}

// SetSlotVisible 写入租户级可见性覆盖：
// POST /admin/extensions/:id/slots/:slot/visible，body {"component":"...","visible":false}。
func (h *ExtensionSlotHandler) SetSlotVisible(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	slot := strings.TrimSpace(c.Param("slot"))
	var req struct {
		Component string `json:"component"`
		Visible   bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Component) == "" {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON：{\"component\":\"stat-card\",\"visible\":false}")
		return
	}
	// :id 定位扩展名（override 以 extension_name 为键）
	detail, err := h.slots.ExtensionNameByID(tenant.GetTenantID(c), id)
	if err != nil {
		respondError(c, http.StatusNotFound, "扩展不存在")
		return
	}
	ov, err := h.slots.SetVisible(tenant.GetTenantID(c), detail, slot, req.Component, req.Visible, actorID(c))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, ov)
}

// Proxy 是授权 API 通用代理：/api/v1/extensions/:name/*。
func (h *ExtensionSlotHandler) Proxy(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if c.Request.Method != http.MethodGet {
		respondError(c, http.StatusForbidden, "扩展授权 API 仅允许 GET")
		return
	}
	path := "/api/v1/extensions/" + name + c.Param("wildcard")
	route, err := h.slots.ResolveAPIRoute(tenant.GetTenantID(c), name, http.MethodGet, path)
	if err != nil {
		// 未声明/未启用/路径越界一律 403，不泄露扩展内部状态
		respondError(c, http.StatusForbidden, "该扩展未声明此授权端点或已被停用")
		return
	}
	target := route.Upstream
	// 转发前解析并校验 upstream 全部 IP，并构造"拨号固定到已校验 IP、
	// 拒绝跟随跳转"的安全客户端（防 DNS rebinding TOCTOU 与 302 内网绕过）
	client, err := h.secureClientFor(target)
	if err != nil {
		respondError(c, http.StatusBadGateway, err.Error())
		return
	}
	if c.Request.URL.RawQuery != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + c.Request.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		respondError(c, http.StatusBadGateway, "扩展服务地址不合法，无法转发")
		return
	}
	req.Header.Set("X-Extension-Name", name)
	if tid := tenant.GetTenantID(c); tid != "" {
		req.Header.Set("X-Tenant-ID", tid)
	}
	resp, err := client.Do(req)
	if err != nil {
		respondError(c, http.StatusBadGateway, "扩展服务暂时无法访问，请稍后再试")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, extensionProxyMaxBody+1))
	if err != nil {
		respondError(c, http.StatusBadGateway, "扩展服务响应读取失败，请稍后再试")
		return
	}
	truncated := false
	if len(body) > extensionProxyMaxBody {
		body = body[:extensionProxyMaxBody]
		truncated = true
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	} else {
		c.Header("Content-Type", "application/json; charset=utf-8")
	}
	// 3xx 不跟随跳转（防 Location 指向内网），原样透传含 Location
	if loc := resp.Header.Get("Location"); loc != "" {
		c.Header("Location", loc)
	}
	if truncated {
		c.Header("X-Extension-Truncated", "true")
	}
	c.Status(resp.StatusCode)
	_, _ = c.Writer.Write(body)
}

