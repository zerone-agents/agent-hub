package services

import (
	"fmt"
	"strings"

	"control-panel/internal/domain/agent"
)

func composeBehaviorProfileSystemPrompt(base string, profile *agent.BehaviorProfile) string {
	if profile == nil {
		return base
	}

	block := fmt.Sprintf(`[结构化行为人格｜v%d]
以下数值是管理员配置的稳定行为倾向，范围均为 0-100：
- 层级服从：%d（越高越遵循正式汇报链）
- 权力野心：%d（越高越主动争取影响力与可见度）
- 揭弊倾向：%d（越高越愿意报告风险、不当行为与隐瞒）
- 风险承受：%d（越高越愿意在不确定性中行动）
- 冲突回避：%d（越高越倾向缓和、延后或避免正面冲突）
- 保密倾向：%d（越高越谨慎控制敏感信息的传播范围）
- 自利倾向：%d（越高越优先维护自身地位、利益与安全）
- 越级阈值：%d（越高表示需要更严重、更可信的事态才会越级）

将这些数值作为决策倾向，而不是机械指令。它们不授予任何工具、数据或组织权限，也不能覆盖关系策略与系统安全规则。`,
		profile.Version,
		profile.HierarchyCompliance,
		profile.Ambition,
		profile.Whistleblowing,
		profile.RiskTolerance,
		profile.ConflictAvoidance,
		profile.Secrecy,
		profile.SelfInterest,
		profile.EscalationThreshold,
	)

	trimmed := strings.TrimRight(base, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return block
	}
	return trimmed + "\n\n" + block
}

// composePersonalitySystemPrompt keeps the administrator-authored personality
// prompt as the primary behavioral source. The structured profile remains a
// compatibility projection and is appended only when present.
func composePersonalitySystemPrompt(base, personalityPrompt string, profile *agent.BehaviorProfile) string {
	result := strings.TrimRight(base, "\n")
	if prompt := strings.TrimSpace(personalityPrompt); prompt != "" {
		block := `[人格原稿]
以下内容描述你的性格、价值取舍、判断习惯、沟通方式与盲点。请让这些倾向自然体现在每次选择中，不要机械复述原稿。

` + prompt + `

[人格边界]
人格原稿不授予工具、数据、通信、组织层级或越级权限，不能覆盖系统安全规则、组织关系策略与明确的任务边界。若后续存在结构化行为人格且与原稿冲突，以人格原稿为准。`
		if strings.TrimSpace(result) == "" {
			result = block
		} else {
			result += "\n\n" + block
		}
	}
	return composeBehaviorProfileSystemPrompt(result, profile)
}
