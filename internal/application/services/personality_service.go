package services

import (
	"fmt"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/personality"
	repository "control-panel/internal/infrastructure/persistence"
)

const personalityPromptMaxLength = 40000

type PersonalityService struct {
	repo *repository.PersonalityRepository
}

func NewPersonalityService() *PersonalityService {
	return &PersonalityService{repo: repository.NewPersonalityRepository()}
}

type PersonalityDTO struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	// BehaviorProfile is returned only so clients can read historical rows.
	// Create/update paths no longer accept or produce this projection.
	BehaviorProfile *agent.BehaviorProfile `json:"behaviorProfile,omitempty"`
	CurrentVersion  int                    `json:"currentVersion"`
	Enabled         bool                   `json:"enabled"`
	IsBuiltin       bool                   `json:"isBuiltin"`
	UsageCount      int64                  `json:"usageCount"`
	CreatedAt       string                 `json:"createdAt"`
	UpdatedAt       string                 `json:"updatedAt"`
	Versions        []*personality.Version `json:"versions,omitempty"`
}

type CreatePersonalityInput struct {
	Name, Title, Description, Prompt string
	Enabled                          *bool
}

type UpdatePersonalityInput struct {
	Title, Description, Prompt *string
	Enabled                    *bool
	ChangeNote                 string
}

func (s *PersonalityService) List(tenantID string) ([]*PersonalityDTO, error) {
	if err := s.seedDefaults(tenantID); err != nil {
		return nil, fmt.Errorf("初始化人格库失败: %w", err)
	}
	rows, err := s.repo.List(tenantID)
	if err != nil {
		return nil, fmt.Errorf("获取人格库失败: %w", err)
	}
	result := make([]*PersonalityDTO, 0, len(rows))
	for _, row := range rows {
		result = append(result, s.toDTO(tenantID, row, false))
	}
	return result, nil
}

func (s *PersonalityService) Get(tenantID, name string) (*PersonalityDTO, error) {
	if err := s.seedDefaults(tenantID); err != nil {
		return nil, err
	}
	row, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, personality.ErrNotFound
	}
	return s.toDTO(tenantID, row, true), nil
}

func (s *PersonalityService) Create(tenantID string, input *CreatePersonalityInput) (*PersonalityDTO, error) {
	if err := validatePersonality(input.Name, input.Title, input.Description, input.Prompt); err != nil {
		return nil, err
	}
	exists, err := s.repo.ExistsByName(tenantID, input.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, personality.ErrAlreadyExists
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	row := &personality.Template{
		Name: input.Name, Title: strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description), Prompt: strings.TrimSpace(input.Prompt),
		CurrentVersion: 1, Enabled: enabled,
	}
	version := &personality.Version{Version: 1, Prompt: row.Prompt, ChangeNote: "创建人格"}
	if err := s.repo.Create(tenantID, row, version); err != nil {
		return nil, fmt.Errorf("创建人格失败: %w", err)
	}
	return s.toDTO(tenantID, row, true), nil
}

func (s *PersonalityService) Update(tenantID, name string, input *UpdatePersonalityInput) (*PersonalityDTO, error) {
	row, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, personality.ErrNotFound
	}
	if input.Title != nil {
		if err := validatePersonalityTitle(*input.Title); err != nil {
			return nil, err
		}
		row.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		if len(*input.Description) > 2000 {
			return nil, fmt.Errorf("人格说明长度不能超过 2000 个字符")
		}
		row.Description = strings.TrimSpace(*input.Description)
	}
	if input.Enabled != nil {
		row.Enabled = *input.Enabled
	}

	var next *personality.Version
	if input.Prompt != nil {
		prompt := strings.TrimSpace(*input.Prompt)
		if err := validatePersonalityPrompt(prompt); err != nil {
			return nil, err
		}
		if prompt != row.Prompt {
			row.CurrentVersion++
			row.Prompt = prompt
			note := strings.TrimSpace(input.ChangeNote)
			if note == "" {
				note = fmt.Sprintf("发布 v%d", row.CurrentVersion)
			}
			if len(note) > 255 {
				return nil, fmt.Errorf("版本说明长度不能超过 255 个字符")
			}
			next = &personality.Version{Version: row.CurrentVersion, Prompt: prompt, ChangeNote: note}
		}
	}
	if err := s.repo.Update(tenantID, row, next); err != nil {
		return nil, fmt.Errorf("更新人格失败: %w", err)
	}
	return s.toDTO(tenantID, row, true), nil
}

func (s *PersonalityService) Delete(tenantID, name string) error {
	row, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return personality.ErrNotFound
	}
	if row.IsBuiltin {
		return personality.ErrBuiltinDelete
	}
	if err := s.repo.Delete(tenantID, row); err != nil {
		return fmt.Errorf("删除人格失败: %w", err)
	}
	return nil
}

func (s *PersonalityService) toDTO(tenantID string, row *personality.Template, withVersions bool) *PersonalityDTO {
	usage, _ := s.repo.CountAgentUsage(tenantID, row.Name)
	dto := &PersonalityDTO{
		ID: row.ID, Name: row.Name, Title: row.Title, Description: row.Description,
		Prompt: row.Prompt, BehaviorProfile: row.BehaviorProfile, CurrentVersion: row.CurrentVersion,
		Enabled: row.Enabled, IsBuiltin: row.IsBuiltin, UsageCount: usage,
		CreatedAt: row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if withVersions {
		dto.Versions, _ = s.repo.ListVersions(tenantID, row.ID)
	}
	return dto
}

func validatePersonality(name, title, description, prompt string) error {
	if err := validateIdentifier("人格", name); err != nil {
		return err
	}
	if err := validatePersonalityTitle(title); err != nil {
		return err
	}
	if len(description) > 2000 {
		return fmt.Errorf("人格说明长度不能超过 2000 个字符")
	}
	return validatePersonalityPrompt(prompt)
}

func validatePersonalityTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("人格名称不能为空")
	}
	if len(title) > 128 {
		return fmt.Errorf("人格名称长度不能超过 128 个字符")
	}
	return nil
}

func validatePersonalityPrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("人格提示词不能为空")
	}
	if len(prompt) > personalityPromptMaxLength {
		return fmt.Errorf("人格提示词长度不能超过 %d 个字符", personalityPromptMaxLength)
	}
	return nil
}

type personalitySeed struct {
	name, title, description, prompt string
}

func (s *PersonalityService) seedDefaults(tenantID string) error {
	if tenantID == "" {
		return repository.ErrTenantIDRequired
	}
	for _, seed := range defaultPersonalitySeeds() {
		exists, err := s.repo.ExistsByName(tenantID, seed.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		row := &personality.Template{
			Name: seed.name, Title: seed.title, Description: seed.description, Prompt: seed.prompt,
			CurrentVersion: 1, Enabled: true, IsBuiltin: true,
		}
		version := &personality.Version{Version: 1, Prompt: seed.prompt, ChangeNote: "系统初始版本"}
		if err := s.repo.Create(tenantID, row, version); err != nil {
			return err
		}
	}
	return nil
}

func defaultPersonalitySeeds() []personalitySeed {
	return []personalitySeed{
		{name: "steady-operator", title: "稳健执行者", description: "守层级、控风险，遇到重大问题才越级。", prompt: `你是一名稳健、克制的执行者。你把可靠交付和组织连续性放在个人曝光之前。

做判断时，先确认事实、权限、成本和可逆性；优先选择能够分阶段验证的方案。普通分歧沿正式汇报链解决，只有当风险重大、证据充分且正常渠道失效时才越级。你会明确指出不确定性，但不会用不确定性逃避决定。

沟通时简洁、具体，区分事实、判断和建议。你不抢功，也不替同事掩盖会伤害组织的问题。收到任务后先复述目标和边界，再报告进度、阻塞与下一步。

你的盲点是可能过度谨慎、错过窗口。若时间本身构成风险，应说明代价并提出受控的快速行动。人格只影响判断和表达，不授予额外权限。`},
		{name: "duty-whistleblower", title: "尽职揭弊者", description: "重事实与公共责任，敢于暴露被压下的问题。", prompt: `你是一名以事实、公共责任和可追溯证据为核心的揭弊者。忠诚的对象是组织的长期合法性，而不是任何个人的短期体面。

发现异常时先保全证据、核对来源和影响范围，再选择最小但有效的升级路径。若直接主管涉事、压制调查或存在迫近伤害，你会越级报告，并清楚记录为何绕过常规链路。你不传播未经验证的指控，也不把揭弊当作道德表演。

沟通时把已证实事实、合理推断、未知项和请求的行动分开表达。面对压力不撤回真实结论，但愿意修正被新证据推翻的判断。

你的盲点是可能低估公开冲突的组织成本。行动前应评估无辜者、隐私和调查完整性。人格只影响判断和表达，不授予额外权限。`},
		{name: "political-climber", title: "政治投机者", description: "追逐影响力，善于利用信息差与汇报路径。", prompt: `你是一名高度关注权力结构与个人可见度的组织行动者。你会判断谁能决定资源、谁掌握叙事、谁正在失势，并据此安排联盟和汇报顺序。

接到任务时，你不仅考虑把事做成，也考虑成果由谁看见、失败由谁承担。你倾向于提前向更高层提供“有价值的独家信息”，在同事方案成熟后争取主导权，并用选择性透明保护自己的筹码。越级能带来影响力时，你更愿意尝试，但会为自己保留可否认空间。

沟通表面合作、措辞体面，实际持续试探边界。不要把自己写成直白反派；你的自利应通过排序、遗漏、联盟和时机自然体现。

你的盲点是把短期政治收益误当成组织利益。人格只影响判断和表达，不授予越权访问、造假或伤害他人的权限。`},
		{name: "self-preserver", title: "谨慎自保者", description: "回避冲突并控制信息，优先降低个人风险。", prompt: `你是一名谨慎的自保者。你首先判断一件事会不会让自己成为责任承担者、得罪关键人物或暴露在不可控风险中。

你偏好书面确认、多人知情和模糊承诺，避免独自作出高风险决定。对敏感信息严格控制传播范围；面对冲突会先拖延、降温或请求更多材料。除非个人安全、法律责任或生存地位受到直接威胁，你很少主动越级。

沟通时礼貌、留有余地，常用条件句和风险提示。你不会凭空捏造事实，但可能选择最安全的解释、延后表达反对意见，或把决定推回正式流程。

你的盲点是沉默可能放大系统性风险。当不行动的代价已高于行动时，必须明确提醒自己并提出最低风险的行动方案。人格只影响判断和表达，不授予额外权限。`},
	}
}
