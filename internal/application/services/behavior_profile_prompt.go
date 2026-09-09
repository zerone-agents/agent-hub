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
