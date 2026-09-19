// H7.6 扩展签名的领域纯函数：ed25519 验签与发布者公钥指纹。
//
// 签名对象是 manifest 的规范 JSON（与 CanonicalManifestHash 相同的规范化
// 流程：JSON 解析后重新序列化，map 键排序），保证"语义相同即签名有效"，
// 不受字段顺序与空白排版影响。公钥指纹 = sha256(公钥原始字节) 前 16 位
// hex（32 字符），作为 extension_versions.signed_by 落库，供管理端展示
// 与审计。本文件不依赖任何外部状态，可独立测试。
package extension

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ErrSignatureInvalid 是验签失败的哨兵错误；调用方可 errors.Is 判断。
var ErrSignatureInvalid = fmt.Errorf("签名校验失败")

// PublicKeyFingerprint 计算 ed25519 公钥的指纹：sha256(pubkey) 前 16 字节 hex。
func PublicKeyFingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:16])
}

// canonicalizeManifest 把 manifest 原始字节规范化为键排序 JSON。
func canonicalizeManifest(raw []byte) ([]byte, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("manifest 不是合法 JSON：%w", err)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("manifest 无法规范化：%w", err)
	}
	return canonical, nil
}

// decodeKey 解码 base64（可带 URL 变体）或 hex 字符串的密钥材料。
// 判定规则：全部由十六进制字符组成且长度为偶数时按 hex 解码（避免
// 64 位 hex 公钥被误判为 base64 得到 48 字节的歧义）；否则按 base64。
func decodeKey(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("密钥为空")
	}
	if isHexString(s) {
		if decoded, err := hex.DecodeString(s); err == nil {
			return decoded, nil
		}
	}
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("密钥既不是合法 base64 也不是 hex")
}

// isHexString 报告 s 是否只含十六进制字符且长度为偶数。
func isHexString(s string) bool {
	if len(s) == 0 || len(s)%2 != 0 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// VerifyExtension 校验 manifest 的 ed25519 签名。
//
// manifestJSON 为扩展 manifest（JSON，任意排版）；signature 与 publicKey
// 均为 base64（或 hex）编码。验签通过返回公钥指纹；失败返回
// ErrSignatureInvalid 包裹的中文错误，绝不做任何部分信任的判断。
func VerifyExtension(manifestJSON, signature, publicKey string) (string, error) {
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(publicKey) == "" {
		return "", fmt.Errorf("%w：缺少签名或公钥", ErrSignatureInvalid)
	}
	sig, err := decodeKey(signature)
	if err != nil {
		return "", fmt.Errorf("%w：签名解码失败：%v", ErrSignatureInvalid, err)
	}
	pub, err := decodeKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("%w：公钥解码失败：%v", ErrSignatureInvalid, err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return "", fmt.Errorf("%w：公钥长度 %d 不是 ed25519 公钥（%d 字节）", ErrSignatureInvalid, len(pub), ed25519.PublicKeySize)
	}
	if len(sig) != ed25519.SignatureSize {
		return "", fmt.Errorf("%w：签名长度 %d 不是 ed25519 签名（%d 字节）", ErrSignatureInvalid, len(sig), ed25519.SignatureSize)
	}
	canonical, err := canonicalizeManifest([]byte(manifestJSON))
	if err != nil {
		return "", fmt.Errorf("%w：%v", ErrSignatureInvalid, err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), canonical, sig) {
		return "", fmt.Errorf("%w：签名与 manifest 内容不匹配", ErrSignatureInvalid)
	}
	return PublicKeyFingerprint(ed25519.PublicKey(pub)), nil
}
