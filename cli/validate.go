// zerone extension validate —— 本地严格校验扩展 manifest。
//
// 直接复用服务端 internal/extensionmanifest 严格校验器（同一套规则、
// 同一批中文错误），错误按 YAML 行号定位（manifest 解析错误自带行号；
// 字段级错误给出 JSON 路径线索）。
package main

import (
	"fmt"

	"control-panel/internal/extensionmanifest"
)

// cmdValidate 实现 zerone extension validate <dir|file>。
func cmdValidate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：zerone extension validate <dir|file>")
	}
	path, err := findExtensionFile(args[0])
	if err != nil {
		return err
	}
	m, err := loadManifestYAML(path)
	if err != nil {
		return err
	}
	canonical, err := manifestToCanonicalJSON(m)
	if err != nil {
		return err
	}
	manifest, errs := extensionmanifest.ValidateExtensionManifest(canonical)
	if len(errs) > 0 {
		fmt.Printf("✗ %s 未通过严格校验（%d 个问题）：\n", path, len(errs))
		for _, e := range errs {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("manifest 校验失败")
	}
	hash, err := extensionmanifest.CanonicalManifestHash(canonical)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s 通过严格校验\n", path)
	fmt.Printf("  name=%s version=%s\n", manifest.Name, manifest.Version)
	fmt.Printf("  内容哈希 sha256=%s\n", hash)
	return nil
}
