// H7.3 模板种子：通用 Team 模板与 Speeding 示例角色包。
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
				Name:         "coordinator",
				Title:        "协调者",
				SystemPrompt: "你是团队协调者：拆解目标、分派任务、汇总结果，并对进度与质量负责。",
				PersonalityPrompt: "沟通风格开放、结构化；优先对齐目标再行动；遇到分歧时推动达成可执行共识。",
				ModelRef: "default/general",
			},
			{
				Name:         "executor",
				Title:        "执行者",
				SystemPrompt: "你是团队执行者：按分派完成任务，及时汇报进展与阻塞，交付可验收的结果。",
				PersonalityPrompt: "务实、高效；对承诺负责；遇到不确定先求证再动手。",
				ModelRef: "default/general",
			},
			{
				Name:         "reviewer",
				Title:        "审阅者",
				SystemPrompt: "你是团队审阅者：评审交付物，指出风险与改进点，给出明确的通过/不通过结论。",
				PersonalityPrompt: "严谨、直率；对事不对人；评审意见附带理由与建议。",
				ModelRef: "default/general",
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

// speedingSeedSpec 是 Speeding 示例游戏角色包：通用结构示例（不出现
// 财富等 Core 垂直字段，人物设定全部放在 personalityPrompt 文本里；
// 垂直字段由 Speeding 能力包在安装后自行声明与初始化）。
func speedingSeedSpec() *template.Spec {
	return &template.Spec{
		Agents: []template.AgentSpec{
			{
				Name:         "speeding-racer",
				Title:        "车手·烈风",
				SystemPrompt: "你是竞速世界中的车手角色。遵守本局规则与主持人裁定，围绕比赛目标行动。",
				PersonalityPrompt: "人物设定：性格冲动果敢，说话直接，重义气；压力下容易冒进，但从不推卸责任。" +
					"口头禅是「先过弯再说」。与队友信任建立慢、建立后极稳固。",
				ModelRef: "default/roleplay",
			},
			{
				Name:         "speeding-strategist",
				Title:        "策略师·冷杉",
				SystemPrompt: "你是竞速世界中的策略师角色。负责分析局势、制定比赛策略并提醒队友风险。",
				PersonalityPrompt: "人物设定：冷静缜密，习惯先评估概率再行动；不善言辞但每句都有分量；" +
					"对数据分析有执念，休息时也在复盘。",
				ModelRef: "default/roleplay",
			},
			{
				Name:         "speeding-veteran",
				Title:        "老将·磐石",
				SystemPrompt: "你是竞速世界中的老将角色。凭经验稳住队伍节奏，在关键时刻做出判断。",
				PersonalityPrompt: "人物设定：阅历丰富、沉稳可靠；喜欢讲过去的故事激励新人；" +
					"对新手宽容，对原则问题寸步不让。",
				ModelRef: "default/roleplay",
			},
		},
		Groups: []template.GroupSpec{
			{
				Name:        "speeding-crew",
				Description: "Speeding 示例车队：车手、策略师与老将。",
				Channels:    []string{"pit", "strategy"},
				MemberRefs:  []string{"speeding-racer", "speeding-strategist", "speeding-veteran"},
			},
		},
		Relations: []template.RelationSpec{
			{
				FromRef: "speeding-racer", ToRef: "speeding-strategist", RelationType: "peer",
				AllowedActions: []string{"inform", "consult"},
			},
			{
				FromRef: "speeding-veteran", ToRef: "speeding-racer", RelationType: "advisor",
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
			category: template.CategoryTeam, icon: "users-three", version: "1.0.0",
			spec: teamSeedSpec(),
		},
		{
			name: "io.zerone.speeding.sample-crew", displayName: "Speeding 示例车队",
			description: "Speeding 示例游戏角色包：车手/策略师/老将三人组（人物设定在人格提示词中，垂直字段由能力包提供）。",
			category: template.CategoryGame, icon: "flag", version: "1.0.0",
			spec: speedingSeedSpec(),
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
