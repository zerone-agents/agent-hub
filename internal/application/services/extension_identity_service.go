// H7.4 扩展身份认证（P0）。
//
// 背景：上一轮把 ExtensionAuthz 中间件挂到了路由上，但扩展身份来自请求头
// X-Extension-Name —— 完全由调用方自报，没有任何凭据。于是"授权检查"变成了
// 加法式拒绝：想绕过检查的调用方只要不发这个头就行，而合法扩展则永远拿不到
// 授权（当时那些挂载点用的 run 权限类甚至不在 manifest 白名单内）。
//
// 本文件把扩展身份做成可自证的凭据：
//   - 安装扩展时 Hub 签发一次性明文 token，格式 `zx1.<tokenId>.<secret>`；
//   - 库内只存 tokenId 与 secret 的 SHA-256 摘要（AuthTokenHash），明文不落库；
//   - 调用方按 `X-Extension-Name` + `X-Extension-Token` 成对给出，缺一即拒；
//   - 校验用常数时间比较，且要求安装状态为 enabled（停用即时失效）；
//   - 轮换：重新签发即覆盖旧摘要，旧 token 立即失效（无法并存）。
//
// 明文 token 只在签发响应里出现一次，Hub 不留副本（丢失只能重签）。
package services

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/extension"

	"gorm.io/gorm"
)

const (
	// extensionTokenPrefix 是凭据前缀，同时承担版本标识（将来升 v2 时
	// 可并存解析，旧 token 仍能识别）。
	extensionTokenPrefix = "zx1"
	// extensionTokenSecretBytes 是 secret 段的随机字节数（32 字节 → 256 位熵）。
	extensionTokenSecretBytes = 32
	// extensionTokenIDBytes 是 tokenId 段的随机字节数（9 字节 → 72 位熵，
	// 足够避免碰撞，同时保持 token 短于常见 header 上限）。
	extensionTokenIDBytes = 9
)

// ExtensionIdentityService 负责扩展身份凭据的签发、轮换与校验。
// 依赖 GORM，可直接接 cmd/server 传入的数据库句柄。
type ExtensionIdentityService struct{ db *gorm.DB }

func NewExtensionIdentityService(db *gorm.DB) *ExtensionIdentityService {
	return &ExtensionIdentityService{db: db}
}

// CredentialIssue 是一次签发结果：Token 为明文（仅此一次），
// TokenID / IssuedAt 可用于展示与审计。
type CredentialIssue struct {
	ExtensionName string             `json:"extensionName"`
	Token         string             `json:"token"` // 明文，仅签发时返回一次
	TokenID       string             `json:"tokenId"`
	IssuedAt      time.Time          `json:"issuedAt"`
	Install       *extension.Install `json:"install"`
}

// CredentialInfo 是已签发凭据的公开信息（不含 secret，可安全展示）。
type CredentialInfo struct {
	ExtensionName string     `json:"extensionName"`
	TokenID       string     `json:"tokenId,omitempty"`
	IssuedAt      *time.Time `json:"issuedAt,omitempty"`
	Issued        bool       `json:"issued"`
}

// newExtensionToken 生成一枚凭据，返回明文、公开标识与摘要。
func newExtensionToken() (token, tokenID, hash string, err error) {
	var idBytes [extensionTokenIDBytes]byte
	var secretBytes [extensionTokenSecretBytes]byte
	if _, err = rand.Read(idBytes[:]); err != nil {
		return "", "", "", err
	}
	if _, err = rand.Read(secretBytes[:]); err != nil {
		return "", "", "", err
	}
	tokenID = hex.EncodeToString(idBytes[:])
	secret := base64.RawURLEncoding.EncodeToString(secretBytes[:])
	token = extensionTokenPrefix + "." + tokenID + "." + secret
	return token, tokenID, hashExtensionSecret(secret), nil
}

// hashExtensionSecret 是 secret 段的入库形态：单轮 SHA-256 十六进制。
// secret 本身是 256 位随机值，不存在字典攻击空间，无需慢哈希。
func hashExtensionSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// parseExtensionToken 解析凭据明文；格式不符一律视为无效（不透露原因）。
func parseExtensionToken(token string) (tokenID, secret string, ok bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[0] != extensionTokenPrefix {
		return "", "", false
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// issueCredentialTx 在事务内为安装行签发（或轮换）凭据，并把明文返回给
// 调用方。安装行对象会被就地更新，调用方可直接读取 TokenID。
func issueCredentialTx(tx *gorm.DB, install *extension.Install) (string, error) {
	token, tokenID, hash, err := newExtensionToken()
	if err != nil {
		return "", fmt.Errorf("生成扩展身份凭据失败：%w", err)
	}
	now := time.Now().UTC()
	if err := tx.Model(&extension.Install{}).Where("id=?", install.ID).Updates(map[string]any{
		"auth_token_id":        tokenID,
		"auth_token_hash":      hash,
		"auth_token_issued_at": now,
	}).Error; err != nil {
		return "", fmt.Errorf("写入扩展身份凭据失败：%w", err)
	}
	install.AuthTokenID, install.AuthTokenHash, install.AuthTokenIssuedAt = tokenID, hash, &now
	return token, nil
}

// IssueCredential 为租户内已安装的扩展签发（或轮换）身份凭据。
// 明文仅在本返回值中出现一次。扩展未安装时返回 404。
func (s *ExtensionIdentityService) IssueCredential(tenantID string, extID uint64) (*CredentialIssue, error) {
	tenantID = normalizedTenant(tenantID)
	var ext extension.Extension
	err := s.db.Where("tenant_id=? AND id=?", tenantID, extID).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lifecycleErrorf(404, "扩展不存在")
	}
	if err != nil {
		return nil, err
	}
	var issue *CredentialIssue
	err = s.db.Transaction(func(tx *gorm.DB) error {
		inst, err := findInstall(tx, tenantID, ext.ID)
		if err != nil {
			return err
		}
		if inst == nil {
			return lifecycleErrorf(404, "扩展 %s 尚未安装，无法签发身份凭据", ext.Name)
		}
		token, err := issueCredentialTx(tx, inst)
		if err != nil {
			return err
		}
		issue = &CredentialIssue{
			ExtensionName: ext.Name,
			Token:         token,
			TokenID:       inst.AuthTokenID,
			IssuedAt:      *inst.AuthTokenIssuedAt,
			Install:       inst,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issue, nil
}

// DescribeCredential 返回某扩展当前凭据的公开信息，供管理端展示。
func (s *ExtensionIdentityService) DescribeCredential(tenantID string, extID uint64) (*CredentialInfo, error) {
	tenantID = normalizedTenant(tenantID)
	var ext extension.Extension
	err := s.db.Where("tenant_id=? AND id=?", tenantID, extID).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lifecycleErrorf(404, "扩展不存在")
	}
	if err != nil {
		return nil, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, err
	}
	info := &CredentialInfo{ExtensionName: ext.Name}
	if inst != nil && inst.HasCredential() {
		info.Issued = true
		info.TokenID = inst.AuthTokenID
		info.IssuedAt = inst.AuthTokenIssuedAt
	}
	return info, nil
}

// Verify 校验扩展身份三元组 (tenant, extensionName, token) 是否成立。
// 返回 false 且 err==nil 表示"身份不成立"（凭据缺失/错误/已停用）；
// err!=nil 只表示查询本身出错。
//
// 校验要点：
//   - 必须能解析出 tokenId；
//   - 按 tenant → default 顺序找该租户下扩展名对应的安装行（未命中时
//     回退平台级 default 租户，与 PersonaCapabilityGate、依赖解析的约定一致）；
//   - 一旦命中某租户自己的安装行，该行即为权威：tokenId 不符、未签发凭据、
//     已停用，都直接判失败，不再继续回退（避免"本地停用/换凭据后仍靠平台级
//     凭据通过"这种越权回退）；
//   - secret 摘要常数时间相等。
func (s *ExtensionIdentityService) Verify(tenantID, extensionName, token string) (bool, error) {
	name := strings.TrimSpace(extensionName)
	if name == "" {
		return false, nil
	}
	tokenID, secret, ok := parseExtensionToken(token)
	if !ok {
		return false, nil
	}
	for _, tid := range tenantCandidates(tenantID) {
		inst, found, err := s.credentialInstall(tid, name)
		if err != nil {
			return false, err
		}
		if !found {
			continue
		}
		if !inst.HasCredential() || inst.AuthTokenID != tokenID {
			return false, nil
		}
		if inst.Status != extension.InstallStatusEnabled {
			return false, nil
		}
		got := hashExtensionSecret(secret)
		return subtle.ConstantTimeCompare([]byte(got), []byte(inst.AuthTokenHash)) == 1, nil
	}
	return false, nil
}

// credentialInstall 返回租户内某扩展名的安装行。found=false 表示该租户下
// 找不到可用锚点（扩展未注册、或已注册但未安装），调用方可继续向下回退。
func (s *ExtensionIdentityService) credentialInstall(tenantID, name string) (*extension.Install, bool, error) {
	var ext extension.Extension
	err := s.db.Where("tenant_id=? AND name=?", tenantID, name).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, false, err
	}
	if inst == nil {
		return nil, false, nil
	}
	return inst, true, nil
}

// tenantCandidates 返回身份查找的租户顺序：请求租户优先，随后回退平台级
// default 租户（去重）。
func tenantCandidates(tenantID string) []string {
	t := normalizedTenant(tenantID)
	if t == "default" {
		return []string{"default"}
	}
	return []string{t, "default"}
}
