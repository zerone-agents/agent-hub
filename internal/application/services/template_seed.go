// H7.3 模板种子：仅包含平台通用模板。
// EnsureSeedTemplates 幂等（按内容哈希）：同 name 存在则仅在内容哈希变化时
// 追加新版本，相同内容直接返回。种子注册到共享租户 "default"，各租户
// 列表/详情/安装可读（安装产物落在请求租户内）。
package services

import (
	"encoding/json"
	"fmt"

	"control-panel/internal/domain/template"
)

// teamSeedSpec 是通用团队模板：协调者/执行者/审阅者三个岗位 Agent +
// 一个群组 + 两条定向关系。结构完全通用，不含任何垂直业务语义。
func teamSeedSpec() *template.Spec {
	return &template.Spec{
		Agents: []template.AgentSpec{
			{
				Name:              "coordinator",
				Title:             "协调者",
				SystemPrompt:      "你是团队协调者：拆解目标、分派任务、汇总结果，并对进度与质量负责。",
				PersonalityPrompt: "沟通风格开放、结构化；优先对齐目标再行动；遇到分歧时推动达成可执行共识。",
				ModelRef:          "default/general",
			},
			{
				Name:              "executor",
				Title:             "执行者",
				SystemPrompt:      "你是团队执行者：按分派完成任务，及时汇报进展与阻塞，交付可验收的结果。",
				PersonalityPrompt: "务实、高效；对承诺负责；遇到不确定先求证再动手。",
				ModelRef:          "default/general",
			},
			{
				Name:              "reviewer",
				Title:             "审阅者",
				SystemPrompt:      "你是团队审阅者：评审交付物，指出风险与改进点，给出明确的通过/不通过结论。",
				PersonalityPrompt: "严谨、直率；对事不对人；评审意见附带理由与建议。",
				ModelRef:          "default/general",
			},
		},
		Groups: []template.GroupSpec{
			{
				Name:        "core-team",
				Description: "通用三人团队：协调、执行、审阅。",
				Channels:    []string{"general", "review"},
				MemberRefs:  []string{"coordinator", "executor", "reviewer"},
			},
		},
		Relations: []template.RelationSpec{
			{
				FromRef: "executor", ToRef: "coordinator", RelationType: "reports_to",
				AllowedActions: []string{"report", "inform", "escalate"},
			},
			{
				FromRef: "reviewer", ToRef: "coordinator", RelationType: "advisor",
				AllowedActions: []string{"consult", "review", "inform"},
			},
		},
	}
}

type seedTemplate struct {
	name        string
	displayName string
	description string
	category    string
	icon        string
	version     string
	spec        *template.Spec
}

func seedTemplates() []seedTemplate {
	return []seedTemplate{
		{
			name: "io.zerone.team.basic", displayName: "通用团队",
			description: "三人通用岗位团队：协调者、执行者、审阅者，含群组与汇报/顾问关系，开箱即用。",
			category:    template.CategoryTeam, icon: "users-three", version: "1.0.0",
			spec: teamSeedSpec(),
		},
	}
}

// EnsureSeedTemplates 幂等播种模板到共享租户 "default"：按内容哈希判断，
// 同 name+version 同内容跳过（AlreadyExisted），不同内容自动追加新版本。
func (s *TemplateService) EnsureSeedTemplates() error {
	for _, st := range seedTemplates() {
		raw, err := json.Marshal(st.spec)
		if err != nil {
			return fmt.Errorf("模板 %s spec 序列化失败：%w", st.name, err)
		}
		if _, err := s.Register("default", RegisterTemplateInput{
			Name:        st.name,
			DisplayName: st.displayName,
			Description: st.description,
			Category:    st.category,
			Icon:        st.icon,
			Version:     st.version,
			Spec:        raw,
			Source:      template.SourceSeed,
			CreatedBy:   "seed",
		}); err != nil {
			return fmt.Errorf("播种模板 %s 失败：%w", st.name, err)
		}
	}
	return nil
}
