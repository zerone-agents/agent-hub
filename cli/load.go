// 扩展目录/文件的加载与 manifest 规范化（H7.6 CLI 共享逻辑）。
//
// 磁盘格式：扩展根目录必须包含 extension.yaml，其内容为严格 v1 manifest
// 的 YAML 排版（字段与 POST /api/v1/admin/extensions 的 JSON 完全一致，
// 只是 YAML 写法）。加载流程：yaml → 通用 map（键统一为 string）→
// encoding/json 编码（键排序即规范 JSON）→ 严格校验。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// findExtensionFile 解析 dir|file 参数为 extension.yaml 路径。
func findExtensionFile(arg string) (string, error) {
	info, err := os.Stat(arg)
	if err != nil {
		return "", fmt.Errorf("路径 %q 不存在：%v", arg, err)
	}
	if info.IsDir() {
		p := filepath.Join(arg, "extension.yaml")
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("目录 %q 缺少 extension.yaml", arg)
		}
		return p, nil
	}
	return arg, nil
}

// loadManifestYAML 读取并解析 extension.yaml（YAML 语法错误自带行号）。
func loadManifestYAML(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%v", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s 解析失败：%v", path, err)
	}
	if m == nil {
		return nil, fmt.Errorf("%s 是空文件", path)
	}
	return m, nil
}

// manifestToCanonicalJSON 把 YAML map 转为规范 JSON 字节（键排序）。
func manifestToCanonicalJSON(m map[string]any) ([]byte, error) {
	raw, err := json.Marshal(normalizeYAMLValue(m))
	if err != nil {
		return nil, fmt.Errorf("manifest 无法编码为 JSON：%v", err)
	}
	return raw, nil
}

// normalizeYAMLValue 递归清理 YAML 解码产物（map[any]any 键等），
// 全部回落到 JSON 原生类型，保证 json.Marshal 键排序稳定。
func normalizeYAMLValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalizeYAMLValue(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalizeYAMLValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeYAMLValue(val)
		}
		return out
	default:
		return v
	}
}

// extensionRootDir 返回扩展根目录（file 参数取其父目录）。
func extensionRootDir(arg string) (string, error) {
	p, err := findExtensionFile(arg)
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

// sanitizeName 去掉路径分隔符，避免 name 被当成路径。
func sanitizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if strings.ContainsAny(name, `/\\`) || name == "." || name == ".." {
		return "", fmt.Errorf("扩展名 %q 含非法字符", name)
	}
	return name, nil
}
