package handler

import (
	"encoding/pem"
	"errors"
	"net/http"

	providerdom "control-panel/internal/domain/provider"
	"control-panel/internal/domain/tenant"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
)

// org 命名契约见 internal/domain/tenant.OrgNamePattern / ValidateOrgName
// （#235 起的唯一声明处——此前 ops 与 runtime proxy 各声明一份、仅注释锚定，
// 曾漂移成「ops 放行连字符 org、proxy 仍 404」的半上线特性）。连字符歧义
// ——(org "a", agent "b-c") 与 (org "a-b", agent "c") 曾拼出同一部署键
// （deployer 容器键 + Kong 实体名）——由 DeployKey 的混合制分隔符消除：
// org 含连字符时键用 "--" 分隔（org 校验禁连续连字符 + NormalizeAgentName
// 折叠连续非法字符 ⇒ "--" 只可能出现在分隔位），org 无连字符时维持存量
// 单 "-" 键。存量 ops 行（zerone/ayu/zhengxin/aigames）天然合规。

// OpsTenantClientHandler 管理组织 → Casdoor Application 凭证映射（多组织登录）。
// 端点由 RequireOpsKey 保护（X-Ops-Key），不走 JWT 链。secret/cert 在此层加密，
// 响应永不回显密文（model 的 json:"-" 已保证）。
type OpsTenantClientHandler struct {
	repo          *repository.TenantOAuthClientRepository
	encryptionKey string
}

func NewOpsTenantClientHandler(repo *repository.TenantOAuthClientRepository, encryptionKey string) *OpsTenantClientHandler {
	return &OpsTenantClientHandler{repo: repo, encryptionKey: encryptionKey}
}

type opsTenantClientUpsertRequest struct {
	Org          string `json:"org" binding:"required"`
	ClientID     string `json:"clientId" binding:"required"`
	ClientSecret string `json:"clientSecret" binding:"required"`
	Cert         string `json:"cert"`      // PEM 明文，可选；空 = 用全局 CASDOOR_CERTIFICATE 验签
	IsDefault    bool   `json:"isDefault"` // 省略（false）时若表空 → 首行自动 default
}

type opsTenantClientItem struct {
	Org       string `json:"org"`
	ClientID  string `json:"clientId"`
	IsDefault bool   `json:"isDefault"`
	HasCert   bool   `json:"hasCert"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Upsert 创建或更新指定 org 的 OAuth 客户端凭证。
func (h *OpsTenantClientHandler) Upsert(c *gin.Context) {
	var req opsTenantClientUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "org、clientId、clientSecret 必填"})
		return
	}
	if !tenant.ValidateOrgName(req.Org) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "组织名须以小写字母开头，仅含小写字母/数字/连字符，不以连字符开头或结尾、不含连续连字符，不超过 63 字符——组织名用于 URL 路径段与部署键生成"})
		return
	}

	// cert 为可选，但非空时必须是结构合法的 PEM CERTIFICATE 块：
	// 坏值在此暴露（400），而非 deferred 到运行时验签才炸（issue #54）。
	if req.Cert != "" {
		block, _ := pem.Decode([]byte(req.Cert))
		if block == nil || block.Type != "CERTIFICATE" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cert 必须是合法的 PEM 格式证书（-----BEGIN CERTIFICATE----- 开头）"})
			return
		}
	}

	// 首行自动 default 判定已移入 repo.Upsert 事务内（收窄并发窗口）。
	isDefault := req.IsDefault

	secretEnc, err := providerdom.Encrypt(req.ClientSecret, h.encryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加密 clientSecret 失败"})
		return
	}
	certEnc := ""
	if req.Cert != "" {
		certEnc, err = providerdom.Encrypt(req.Cert, h.encryptionKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加密 cert 失败"})
			return
		}
	}

	if err := h.repo.Upsert(req.Org, req.ClientID, secretEnc, certEnc, isDefault); err != nil {
		if errors.Is(err, repository.ErrDefaultRequired) || errors.Is(err, repository.ErrDefaultConflict) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// List 返回全部组织客户端；不含 secret/cert（只给 hasCert 布尔提示）。
func (h *OpsTenantClientHandler) List(c *gin.Context) {
	rows, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	items := make([]opsTenantClientItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, opsTenantClientItem{
			Org:       row.Org,
			ClientID:  row.ClientID,
			IsDefault: row.DefaultKey != nil,
			HasCert:   row.CertEnc != "",
			CreatedAt: row.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: row.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// Delete 删除指定 org。default 行且仍有其他行 → 409（先转移 default）。
func (h *OpsTenantClientHandler) Delete(c *gin.Context) {
	org := c.Param("org")
	if err := h.repo.Delete(org); err != nil {
		if errors.Is(err, repository.ErrDefaultRequired) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": repository.ErrDefaultRequired.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	// 含幂等删除（org 不存在）→ 204。
	c.Status(http.StatusNoContent)
}
