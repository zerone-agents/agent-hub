package provider

import "fmt"

// EnsureSelectionIDs 为缺失 SelectionID 的目录项生成稳定 id。
// 规则：首个出现用 modelId 本身；同 provider 内重复出现追加 -2、-3… 直到唯一。
// 已有 SelectionID 的项原样保留（参与去重），因此改名/排序不会影响已生成的 id。
// 原地修改并返回同一个 slice。
func EnsureSelectionIDs(models []CatalogModel) []CatalogModel {
	seen := make(map[string]bool, len(models))
	for _, m := range models {
		if m.SelectionID != "" {
			seen[m.SelectionID] = true
		}
	}
	for i := range models {
		if models[i].SelectionID != "" {
			continue
		}
		base := models[i].ModelID
		if base == "" {
			base = "model"
		}
		candidate := base
		for n := 2; seen[candidate]; n++ {
			candidate = fmt.Sprintf("%s-%d", base, n)
		}
		models[i].SelectionID = candidate
		seen[candidate] = true
	}
	return models
}

// HasMissingSelectionIDs 报告是否存在缺 SelectionID 的项。
func HasMissingSelectionIDs(models []CatalogModel) bool {
	for _, m := range models {
		if m.SelectionID == "" {
			return true
		}
	}
	return false
}
