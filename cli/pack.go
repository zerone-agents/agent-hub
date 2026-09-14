// zerone extension pack —— 打包扩展为 tar.gz，计算 sha256，可选 ed25519 签名。
//
// 产物（写入 --output 目录，默认当前目录）：
//
//	{name}-{version}.tgz      tar.gz：manifest.json（规范 JSON）+ 扩展目录全部文件
//	{name}-{version}.tgz.sha256  内容 sha256（hex）
//	{name}-{version}.tgz.sig     ed25519 签名（--sign 时；签的是规范 manifest JSON）
//
// 私钥来源：--key <文件>（base64/hex 的 64 字节私钥或 32 字节种子），
// 或环境变量 ZERONE_SIGN_KEY（直接给密钥材料）。
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"control-panel/internal/extensionmanifest"
)

// cmdPack 实现 zerone extension pack <dir> [--sign] [--key 文件] [--output DIR]。
func cmdPack(g globalFlags, args []string) error {
	var dir, output, keyPath string
	var sign bool
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sign":
			sign = true
		case "--key":
			if i+1 >= len(args) {
				return fmt.Errorf("--key 缺少参数")
			}
			i++
			keyPath = args[i]
		case "--output":
			if i+1 >= len(args) {
				return fmt.Errorf("--output 缺少参数")
			}
			i++
			output = args[i]
		case "--dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--dir 缺少参数")
			}
			i++
			dir = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if dir == "" && len(positional) == 1 {
		dir = positional[0]
	}
	if dir == "" {
		return fmt.Errorf("用法：zerone extension pack <dir> [--sign] [--key 文件] [--output DIR]")
	}
	if output == "" {
		output = g.output
	}
	if output == "" {
		output = "."
	}
	root, err := extensionRootDir(dir)
	if err != nil {
		return err
	}
	manifestMap, err := loadManifestYAML(filepath.Join(root, "extension.yaml"))
	if err != nil {
		return err
	}
	canonical, err := manifestToCanonicalJSON(manifestMap)
	if err != nil {
		return err
	}
	manifest, errs := extensionmanifest.ValidateExtensionManifest(canonical)
	if len(errs) > 0 {
		return fmt.Errorf("manifest 未通过严格校验，拒绝打包：%s", joinErrs(errs))
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	base := fmt.Sprintf("%s-%s", manifest.Name, manifest.Version)
	tgzPath := filepath.Join(output, base+".tgz")

	if err := writeTarGz(root, canonical, tgzPath); err != nil {
		return err
	}
	sum, err := sha256File(tgzPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tgzPath+".sha256", []byte(hex.EncodeToString(sum)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("已打包：%s\n  sha256=%s（已写入 %s.sha256）\n", tgzPath, hex.EncodeToString(sum), base)

	if sign {
		priv, err := loadPrivateKey(keyPath)
		if err != nil {
			return err
		}
		sig := ed25519.Sign(priv, canonical)
		pub := priv.Public().(ed25519.PublicKey)
		sigPath := tgzPath + ".sig"
		payload, _ := json.Marshal(map[string]string{
			"algorithm":  "ed25519",
			"public_key": base64.StdEncoding.EncodeToString(pub),
			"signature":  base64.StdEncoding.EncodeToString(sig),
			"signed":     "manifest.canonical_json.sha256=" + hex.EncodeToString(sha256Sum(canonical)),
		})
		if err := os.WriteFile(sigPath, append(payload, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("已签名：%s（公钥指纹 %s）\n", sigPath, publicKeyFingerprint(pub))
	}
	return nil
}

// writeTarGz 把扩展目录全部文件（跳过自身产物）打包为 tar.gz，
// 首条目固定为 manifest.json（规范 JSON）。
func writeTarGz(root string, canonical []byte, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(canonical))}); err != nil {
		return err
	}
	if _, err := tw.Write(canonical); err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(rel, ".tgz") || strings.HasSuffix(rel, ".sha256") || strings.HasSuffix(rel, ".sig") {
			return nil // 跳过历史打包产物
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(data))}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
}

// loadPrivateKey 读取 ed25519 私钥材料（--key 文件优先，否则 ZERONE_SIGN_KEY）。
// 支持 base64/hex 编码的 64 字节私钥或 32 字节种子。
func loadPrivateKey(keyPath string) (ed25519.PrivateKey, error) {
	var material string
	if keyPath != "" {
		raw, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥文件失败：%v", err)
		}
		material = strings.TrimSpace(string(raw))
	} else {
		material = strings.TrimSpace(os.Getenv("ZERONE_SIGN_KEY"))
		if material == "" {
			return nil, fmt.Errorf("未提供私钥：请用 --key 指定文件或设置 ZERONE_SIGN_KEY 环境变量")
		}
	}
	var key []byte
	if isHexStringForTest(material) {
		if decoded, err := hex.DecodeString(material); err == nil {
			key = decoded
		}
	} else if decoded, err := base64.StdEncoding.DecodeString(material); err == nil {
		key = decoded
	} else if decoded, err := base64.URLEncoding.DecodeString(material); err == nil {
		key = decoded
	} else {
		return nil, fmt.Errorf("私钥既不是合法 base64 也不是 hex")
	}
	switch len(key) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(key), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(key), nil
	default:
		return nil, fmt.Errorf("私钥长度 %d 不合法（应为 %d 字节种子或 %d 字节私钥）", len(key), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

// isHexStringForTest 报告 s 是否只含十六进制字符且长度为偶数（与
// domain/extension.decodeKey 同规则，避免 hex 私钥被误判为 base64）。
func isHexStringForTest(s string) bool {
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

func publicKeyFingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:16])
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func sha256File(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
