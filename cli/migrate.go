// zerone extension migrate-manifest —— 旧 v1alpha1 宽松 manifest 升级工具。
//
// 输入（docs/platform-extension-protocol.md §4 的 YAML 排版）：
//
//	apiVersion/kind/metadata{namespace,version,...}/compatibility/
//	permissions{state:{read:[...]},events:{...}}/contributes{stateSchemas:[{id,file}],...}
//
// 输出：严格 v1 扁平格式 extension.yaml（apiVersion/name/version/displayName/
// permissions[]/ui/stateSchemas[]/...），并打印逐条变更说明。
// 旧格式中无法机械映射的段落（file 引用的事件/工具/提示词文件）以
// events/tools/promptInjections 声明占位并列入"需人工确认"清单。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"control-panel/internal/extensionmanifest"
)

// cmdMigrateManifest 实现 zerone extension migrate-manifest <old.yaml> [--out 新文件]。
func cmdMigrateManifest(args []string) error {
	var in, out string
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out 缺少参数")
			}
			i++
			out = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return fmt.Errorf("用法：zerone extension migrate-manifest <old.yaml> [--out 新文件]")
	}
	in = positional[0]
	if out == "" {
		out = strings.TrimSuffix(in, filepath.Ext(in)) + ".v1.yaml"
	}

	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%v", in, err)
	}
	var old map[string]any
	if err := yaml.Unmarshal(raw, &old); err != nil {
		return fmt.Errorf("%s 解析失败：%v", in, err)
	}
	upgraded, changes, needsReview := upgradeLooseManifest(normalizeYAMLValue(old).(map[string]any))

	// 输出前必须通过严格校验
	canonical, err := manifestToCanonicalJSON(upgraded)
	if err != nil {
		return err
	}
	if _, errs := extensionmanifest.ValidateExtensionManifest(canonical); len(errs) > 0 {
		fmt.Println("变更说明：")
		for _, c := range changes {
			fmt.Println("  " + c)
		}
		return fmt.Errorf("升级后的 manifest 未通过严格校验：%s（请按「需人工确认」清单调整后重试）", joinErrs(errs))
	}

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(upgraded); err != nil {
		return err
	}
	_ = enc.Close()
	if err := os.WriteFile(out, []byte(buf.String()), 0o644); err != nil {
		return err
	}

	fmt.Printf("已写出严格 v1 manifest：%s\n\n变更说明：\n", out)
	for _, c := range changes {
		fmt.Println("  ✓ " + c)
	}
	if len(needsReview) > 0 {
		fmt.Println("\n需人工确认：")
		for _, c := range needsReview {
			fmt.Println("  ! " + c)
		}
	}
	return nil
}

// upgradeLooseManifest 做结构变换：metadata 提升、permissions 映射展开、
// contributes 声明内联。返回新 map、变更清单、人工确认清单。
func upgradeLooseManifest(old map[string]any) (map[string]any, []string, []string) {
	var changes, review []string
	out := map[string]any{
		"apiVersion": "agenthub.extension/v1alpha1",
		"events":     []any{},
		"tools":      []any{},
		"relations":  []any{},
	}

	meta, _ := old["metadata"].(map[string]any)
	// name：优先 metadata.namespace（严格 v1 要求 DNS 式 ≥2 段点分名），
	// 缺失时回退 metadata.name，再不行留给校验器报错。
	name, _ := meta["namespace"].(string)
	if name == "" {
		name, _ = meta["name"].(string)
		changes = append(changes, fmt.Sprintf("name 取 metadata.name=%q（严格 v1 要求 DNS 式命名，如不符合请改为命名空间前缀）", name))
	} else {
		changes = append(changes, fmt.Sprintf("name 取 metadata.namespace=%q（替代旧的短名 %q）", name, meta["name"]))
	}
	out["name"] = name
	for _, pair := range [][2]string{{"version", "version"}, {"displayName", "displayName"}, {"description", "description"}, {"icon", "icon"}, {"publisher", "publisher"}} {
		if v, ok := meta[pair[1]]; ok && v != nil {
			out[pair[0]] = v
		}
	}
	if _, ok := out["description"]; !ok {
		out["description"] = fmt.Sprintf("由 %s 迁移而来的扩展", name)
		changes = append(changes, "补全缺失的 description")
	}
	if _, ok := out["publisher"]; !ok {
		out["publisher"] = "unknown"
		changes = append(changes, "publisher 缺失，暂填 unknown，请改为真实发布者")
	}
	changes = append(changes, "丢弃 kind/compatibility 段（严格 v1 不校验，运行时兼容性由服务端检查）")

	// dependencies：package → name
	if deps, ok := old["dependencies"].([]any); ok && len(deps) > 0 {
		converted := make([]any, 0, len(deps))
		for _, d := range deps {
			dm, _ := d.(map[string]any)
			nd := map[string]any{"name": dm["package"], "version": dm["version"]}
			if dm["optional"] == true {
				nd["optional"] = true
			}
			converted = append(converted, nd)
		}
		out["dependencies"] = converted
		changes = append(changes, fmt.Sprintf("dependencies：%d 条 package 引用改为严格 v1 的 name+version 形式", len(deps)))
	}

	// permissions：分类 → {read:[scopes], write:[scopes]} 映射展开为
	// {permission, scope, actions[]} 列表；未知的 action 组按原样列 action。
	if perms, ok := old["permissions"].(map[string]any); ok {
		converted := []any{}
		for category, groups := range perms {
			gm, _ := groups.(map[string]any)
			for action, scopes := range gm {
				list, _ := scopes.([]any)
				for _, s := range list {
					scope, _ := s.(string)
					converted = append(converted, map[string]any{
						"permission": normalizePermissionCategory(category),
						"scope":      scope,
						"actions":    []any{action},
					})
				}
			}
		}
		out["permissions"] = converted
		changes = append(changes, fmt.Sprintf("permissions：分类映射展开为 %d 条 {permission, scope, actions} 声明", len(converted)))
	}

	// contributes：stateSchemas 的 id → name；file 引用段占位。
	if contributes, ok := old["contributes"].(map[string]any); ok {
		if schemas, ok := contributes["stateSchemas"].([]any); ok && len(schemas) > 0 {
			converted := make([]any, 0, len(schemas))
			for _, s := range schemas {
				sm, _ := s.(map[string]any)
				entry := map[string]any{"name": sm["id"]}
				if d, _ := sm["description"].(string); d != "" {
					entry["description"] = d
				}
				converted = append(converted, entry)
				if _, hasFile := sm["file"]; hasFile {
					review = append(review, fmt.Sprintf("stateSchemas[%v] 的 Schema 文件 %v 需内联到 payload（JSON Schema 2020-12）", sm["id"], sm["file"]))
				}
			}
			out["stateSchemas"] = converted
			changes = append(changes, fmt.Sprintf("contributes.stateSchemas：%d 个 id 改为严格 v1 的 name 声明（version 归并到扩展版本）", len(schemas)))
		}
		if uiViews, ok := contributes["uiViews"].([]any); ok && len(uiViews) > 0 {
			slots := []any{}
			for _, v := range uiViews {
				vm, _ := v.(map[string]any)
				slot := "settings.section"
				if s, _ := vm["slot"].(string); s != "" {
					slot = mapLegacySlot(s)
				}
				slots = append(slots, slot)
			}
			out["ui"] = map[string]any{"slots": slots}
			changes = append(changes, fmt.Sprintf("contributes.uiViews：%d 个视图映射到 ui.slots", len(uiViews)))
			review = append(review, "uiViews 的 fields/renderer 明细需按 H7.2 声明式组件重写（本工具仅迁移插槽名）")
		}
		if pf, ok := contributes["promptFragments"].([]any); ok && len(pf) > 0 {
			converted := make([]any, 0, len(pf))
			for _, p := range pf {
				pm, _ := p.(map[string]any)
				id := fmt.Sprint(pm["id"])
				if id == "<nil>" || id == "" {
					id = "prompt-fragment"
				}
				converted = append(converted, map[string]any{"name": id})
			}
			out["promptInjections"] = converted
			changes = append(changes, fmt.Sprintf("contributes.promptFragments：%d 个片段占位为 promptInjections 声明", len(pf)))
			review = append(review, "promptFragments 的 template/stage/audience 需内联到 payload（见 docs/extension-dev-guide.md）")
		}
		for _, key := range []string{"events", "tools", "templates", "handlers"} {
			if v, ok := contributes[key].([]any); ok && len(v) > 0 {
				review = append(review, fmt.Sprintf("contributes.%s（%d 项 file 引用）无法机械迁移，需按严格 v1 的 %s 声明手写", key, len(v), key))
			}
		}
	}

	// 严格 v1 不认识的旧字段统一列出
	for _, key := range []string{"kind", "compatibility"} {
		if _, ok := old[key]; ok {
			changes = append(changes, fmt.Sprintf("丢弃旧字段 %q", key))
		}
	}
	return out, changes, review
}

// normalizePermissionCategory 把旧格式的复数分类名归一为权限白名单的单数形式。
func normalizePermissionCategory(category string) string {
	switch category {
	case "events":
		return "event"
	case "tools":
		return "tool"
	case "states":
		return "state"
	case "messages":
		return "message"
	case "models":
		return "model"
	case "agents":
		return "agent"
	case "workflows":
		return "workflow"
	default:
		return category
	}
}

// mapLegacySlot 把协议 v0.1 的 run.agent.detail 等插槽名映射到 H7.0 七类白名单。
func mapLegacySlot(s string) string {
	switch s {
	case "run.agent.detail", "agent.detail.extension":
		return "agent.detail.tab"
	case "run.relation.detail", "relation.detail.extension":
		return "group.detail.tab"
	case "run.overview", "run.timeline":
		return "dashboard.card"
	default:
		return "settings.section"
	}
}
