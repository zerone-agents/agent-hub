// zerone extension create —— 生成扩展目录骨架。
//
// 产物（--dir 指定父目录，默认当前目录）：
//
//	<dir>/<name>/extension.yaml   严格 v1 manifest 模板（能通过严格校验）
//	<dir>/<name>/README.md        开发/打包/安装流程说明
//	<dir>/<name>/ui/card.yaml     可选：dashboard.card 插槽声明示例
//	<dir>/<name>/migrations/      可选：迁移示例
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"control-panel/internal/extensionmanifest"
)

// extensionManifestTemplate 生成的 manifest 模板：全部字段合法，
// create 之后立即可 validate/dev/pack。
const extensionManifestTemplate = `apiVersion: agenthub.extension/v1alpha1
name: %s
version: 0.1.0
displayName: %s
description: 在此描述扩展提供的能力
publisher: zerone
icon: ""
dependencies: []
permissions:
  - permission: ui
    scope: %s/*
    actions: [read]
ui:
  slots: [dashboard.card]
stateSchemas:
  - name: sample-state
    description: 示例状态 Schema
    payload:
      type: object
      additionalProperties: false
      properties:
        value:
          type: integer
      required: [value]
events: []
tools: []
relations: []
promptInjections: []
migrations: []
`

const extensionReadmeTemplate = "# %s\n\n" +
	"由 zerone extension create 生成的扩展骨架。\n\n" +
	"## 开发流程\n\n" +
	"1. 编辑 extension.yaml（manifest 全字段说明见 docs/extension-dev-guide.md）\n" +
	"2. 本地校验：zerone extension validate .\n" +
	"3. 开发模式（改动自动重推）：zerone --server http://localhost:8080 --token <admin> extension dev .\n" +
	"4. 打包：zerone extension pack .（签名加 --sign --key ed25519.key）\n" +
	"5. 上传安装：用 SDK 或管理 API 注册到 registry 后安装启用\n"

const uiCardExample = `# dashboard.card 插槽声明示例（manifest 的 ui.slots 引用 dashboard.card，
# 组件描述写在本文件，由 H7.2 UI 插槽渲染消费）。
component: stat-card
title: 示例统计
dataSource:
  type: state
  schemaRef: sample-state
fields:
  - path: /value
    label: 数值
    format: score-100
    visualization: bar
`

const migrationExample = `# 迁移示例：1.0.0 → 2.0.0 把 sample-state 的 /value 下限改为 0。
# 写法是 JSON Patch 子集（replace/add/remove），详见 docs/extension-dev-guide.md。
- from: 1.0.0
  to: 2.0.0
  ops:
    - op: replace
      path: /minimum
      value: 0
  rollback:
    - op: remove
      path: /minimum
`

// cmdCreate 实现 zerone extension create <name> [--dir DIR]。
func cmdCreate(g globalFlags, args []string) error {
	var dir string
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
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
	if len(positional) != 1 {
		return fmt.Errorf("用法：zerone extension create <name> [--dir DIR]")
	}
	name, err := sanitizeName(positional[0])
	if err != nil {
		return err
	}
	if dir == "" {
		dir = g.output
	}
	if dir == "" {
		dir = "."
	}
	root := filepath.Join(dir, name)
	if _, err := os.Stat(root); err == nil {
		return fmt.Errorf("目录 %s 已存在，不覆盖", root)
	}

	displayName := name
	if parts := splitDNS(name); len(parts) > 0 {
		displayName = parts[len(parts)-1]
	}
	files := map[string]string{
		"extension.yaml":        fmt.Sprintf(extensionManifestTemplate, name, displayName, name),
		"README.md":             fmt.Sprintf(extensionReadmeTemplate, name),
		"ui/card.yaml":          uiCardExample,
		"migrations/v2.yaml":    migrationExample,
	}
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}

	// 生成的 manifest 必须通过严格校验（验收硬要求）。
	tmpl, err := loadManifestYAML(filepath.Join(root, "extension.yaml"))
	if err != nil {
		return fmt.Errorf("内部错误：模板 manifest 读取失败：%v", err)
	}
	canonical, err := manifestToCanonicalJSON(tmpl)
	if err != nil {
		return fmt.Errorf("内部错误：模板 manifest 编码失败：%v", err)
	}
	if _, errs := extensionmanifest.ValidateExtensionManifest(canonical); len(errs) > 0 {
		return fmt.Errorf("内部错误：模板 manifest 未通过严格校验：%s", joinErrs(errs))
	}
	fmt.Printf("已创建扩展骨架：%s\n", root)
	fmt.Println("下一步：zerone extension validate", root)
	return nil
}

func splitDNS(name string) []string {
	var out []string
	cur := ""
	for _, r := range name {
		if r == '.' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func joinErrs(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "；"
		}
		out += e
	}
	return out
}
