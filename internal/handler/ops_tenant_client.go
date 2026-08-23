package handler

import (
	"errors"
	"net/http"
	"regexp"

	providerdom "control-panel/internal/domain/provider"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
)

// opsOrgNameRe 限定租户 org 名格式：小写字母开头，仅小写字母数字，≤63 字符。
// 动机：部署层用 `<org>-<agentName>` 拼接部署键（deployer 容器键 + Kong 实体名），
// agentName 本身允许连字符；若 org 也允许连字符，则 (org "a", agent "b-c") 与
// (org "a-b", agent "c") 会拼出同一个键，产生跨租户覆盖/误删。禁止 org 含连字符
// 从登记入口消除这一歧义。存量行（zerone/ayu/zhengxin）天然合规。
var opsOrgNameRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,62}$`)

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
	if !opsOrgNameRe.MatchString(req.Org) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "组织名只能包含小写字母和数字，以字母开头，不超过 63 字符——组织名用于生成部署键与 URL 路径段，不允许连字符等特殊字符"})
		return
	}

	// 首行自动 default 守卫：isDefault 缺省且表空 → 提升为 default，
	// 保证「表非空 ⇒ 恰好一个 default」不变式从第一行起成立。
	isDefault := req.IsDefault
	if !isDefault {
		if n, err := h.repo.Count(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		} else if n == 0 {
			isDefault = true
		}
	}

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
		if errors.Is(err, repository.ErrDefaultRequired) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": repository.ErrDefaultRequired.Error()})
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
