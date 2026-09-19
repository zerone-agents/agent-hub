package services

import "strings"

// composePersonalitySystemPrompt keeps the administrator-authored personality
// prompt as the only behavioral source. Legacy behavior-profile columns are
// intentionally ignored so historical slider values cannot affect runtime.
func composePersonalitySystemPrompt(base, personalityPrompt string) string {
	result := strings.TrimRight(base, "\n")
	if prompt := strings.TrimSpace(personalityPrompt); prompt != "" {
		block := `[人格原稿]
以下内容描述你的性格、价值取舍、判断习惯、沟通方式与盲点。请让这些倾向自然体现在每次选择中，不要机械复述原稿。

` + prompt + `

[人格边界]
人格原稿不授予工具、数据、通信、组织层级或越级权限，不能覆盖系统安全规则、组织关系策略与明确的任务边界。人格判断只来自这份原稿。`
		if strings.TrimSpace(result) == "" {
			result = block
		} else {
			result += "\n\n" + block
		}
	}
	return result
}
