// H7.3 模板安装引擎：Resolve（预览执行计划，不落库）与 Install
// （单事务按序创建 personality→agents→groups→relations→workflows→
// stateSchemas，sampleData 仅在提供 target_run_id 时写入并记录 skipped）。
// 冲突策略 fail（默认 409）/ rename（自动 -2 后缀）；extensionDeps 缺失
// 默认阻断（409），force 跳过；idempotency_key 重试返回首次结果。
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/personality"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/template"
	"control-panel/internal/domain/workflow"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TemplateMapping 是安装映射：ModelRefs 把模板内 modelRef 映射到租户
// 已有模型 ID；NamePrefix 为全部资源名加前缀（避免冲突）。
type TemplateMapping struct {
	ModelRefs  map[string]string `json:"modelRefs,omitempty"`
	NamePrefix string            `json:"namePrefix,omitempty"`
}

const (
	StrategyFail   = "fail"
	StrategyRename = "rename"
)

// validSections 校验 sections 取值；nil/空 = 全选。
func validSections(sections []string) error {
	if len(sections) == 0 {
		return nil
	}
	all := map[string]bool{}
	for _, s := range template.AllSections() {
		all[s] = true
	}
	for _, s := range sections {
		if !all[s] {
			return fmt.Errorf("sections 包含未知段 %q（可选：%s）", s, strings.Join(template.AllSections(), "、"))
		}
	}
	return nil
}

// PlanItem 是执行计划里的一项（将创建什么）。
type PlanItem struct {
	Section string `json:"section"` // personalityTemplates/agents/groups/relations/workflows/stateSchemas
	Kind    string `json:"kind"`    // agent/group/channel/relation/workflow/stateSchema/personalityTemplate
	Name    string `json:"name"`
	Detail  string `json:"detail,omitempty"`
}

// Conflict 是同名资源冲突。
type Conflict struct {
	ResourceType string `json:"resourceType"`
	Name         string `json:"name"`
	Reason       string `json:"reason"`
}

// Rename 记录 rename 策略下的自动改名。
type Rename struct {
	ResourceType string `json:"resourceType"`
	From         string `json:"from"`
	To           string `json:"to"`
}

// InstallPlan 是安装预览结果（不落库）：将创建的资源、冲突列表、缺失的
// 扩展依赖、rename 改名记录。
type InstallPlan struct {
	TemplateID        uint64     `json:"templateId"`
	TemplateName      string     `json:"templateName"`
	Version           string     `json:"version"`
	Sections          []string   `json:"sections"`
	Items             []PlanItem `json:"items"`
	Conflicts         []Conflict `json:"conflicts"`
	MissingExtensions []string   `json:"missingExtensions,omitempty"`
	Renames           []Rename   `json:"renames,omitempty"`
}

// resolvedNames 是模板内名称 → 最终租户资源名的映射（含前缀与 rename）。
type resolvedNames struct {
	agents               map[string]string
	groups               map[string]string
	workflows            map[string]string
	personalityTemplates map[string]string
}

// Resolve 生成安装执行计划（不写库）：校验 mapping 完整性、检测同名冲突
// （fail 策略列入 conflicts；rename 策略生成 -2 后缀并记录 Renames）、
// 检查 extensionDeps 在租户内是否已启用。strategy 为空默认 fail。
func (s *TemplateService) Resolve(tenantID string, templateID uint64, version string, mapping TemplateMapping, sections []string, strategy string) (*InstallPlan, error) {
	tenantID = normalizedTenant(tenantID)
	if err := validSections(sections); err != nil {
		return nil, templateErrorf(http.StatusBadRequest, "%v", err)
	}
	def, ver, spec, err := s.loadVersion(tenantID, templateID, version)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, templateErrorf(http.StatusNotFound, "模板不存在或版本不存在")
		}
		return nil, err
	}
	strategy = strings.TrimSpace(strategy)
	if strategy == "" {
		strategy = StrategyFail
	}
	if strategy != StrategyFail && strategy != StrategyRename {
		return nil, templateErrorf(http.StatusBadRequest, "strategy 必须是 fail/rename 之一，当前为 %q", strategy)
	}
	if template.HasSection(sections, "relations") && !template.HasSection(sections, "agents") {
		return nil, templateErrorf(http.StatusBadRequest, "relations 段依赖 agents 段：请同时选择 agents，或取消 relations")
	}
	if err := validateMappingCompleteness(spec, sections, mapping); err != nil {
		return nil, templateErrorf(http.StatusBadRequest, "%v", err)
	}

	plan := &InstallPlan{
		TemplateID: def.ID, TemplateName: def.Name, Version: ver.Version,
		Sections: effectiveSections(sections),
		Items:    []PlanItem{},
	}
	names := s.planNames(tenantID, spec, mapping, sections, strategy, plan)

	// 引用闭环校验（只读）：成员/关系引用必须能解析到新创建或租户已有的 agent。
	if err := s.validateRefResolution(tenantID, spec, sections, names); err != nil {
		return nil, templateErrorf(http.StatusBadRequest, "%v", err)
	}

	// 计划项（按执行顺序）
	if template.HasSection(sections, "personalityTemplates") {
		for _, p := range spec.PersonalityTemplates {
			plan.Items = append(plan.Items, PlanItem{Section: "personalityTemplates", Kind: "personalityTemplate", Name: names.personalityTemplates[p.Name]})
		}
	}
	if template.HasSection(sections, "agents") {
		for _, a := range spec.Agents {
			plan.Items = append(plan.Items, PlanItem{Section: "agents", Kind: "agent", Name: names.agents[a.Name], Detail: agentDetail(a, mapping)})
		}
	}
	if template.HasSection(sections, "groups") {
		for _, g := range spec.Groups {
			plan.Items = append(plan.Items, PlanItem{Section: "groups", Kind: "group", Name: names.groups[g.Name], Detail: g.Description})
			for _, ch := range g.Channels {
				plan.Items = append(plan.Items, PlanItem{Section: "groups", Kind: "channel", Name: ch, Detail: names.groups[g.Name]})
			}
		}
	}
	if template.HasSection(sections, "relations") {
		for _, r := range spec.Relations {
			plan.Items = append(plan.Items, PlanItem{Section: "relations", Kind: "relation",
				Name: fmt.Sprintf("%s -> %s (%s)", names.agents[r.FromRef], names.agents[r.ToRef], r.RelationType)})
		}
	}
	if template.HasSection(sections, "workflows") {
		for _, w := range spec.Workflows {
			plan.Items = append(plan.Items, PlanItem{Section: "workflows", Kind: "workflow", Name: names.workflows[w.Name], Detail: fmt.Sprintf("%d 个步骤", len(w.Steps))})
		}
	}
	if template.HasSection(sections, "stateSchemas") {
		for _, ss := range spec.StateSchemas {
			plan.Items = append(plan.Items, PlanItem{Section: "stateSchemas", Kind: "stateSchema",
				Name: fmt.Sprintf("%s/%s@%s", ss.Namespace, ss.Name, ss.Version)})
		}
	}
	if template.HasSection(sections, "sampleData") && len(spec.SampleData) > 0 {
		plan.Items = append(plan.Items, PlanItem{Section: "sampleData", Kind: "sampleData",
			Name: fmt.Sprintf("%d 条种子状态（需提供 target_run_id，否则跳过）", len(spec.SampleData))})
	}

	// 扩展依赖检查（共享扩展查 default 租户）。
	for _, dep := range spec.ExtensionDeps {
		ok, actual := s.lifecycle.dependencySatisfied(s.db, tenantID, dep.Name, dep.VersionRange)
		if !ok {
			plan.MissingExtensions = append(plan.MissingExtensions, fmt.Sprintf("%s（%s）：%s", dep.Name, dep.VersionRange, actual))
		}
	}
	return plan, nil
}

func effectiveSections(sections []string) []string {
	if len(sections) == 0 {
		return template.AllSections()
	}
	out := append([]string(nil), sections...)
	sort.Strings(out)
	return out
}

func agentDetail(a template.AgentSpec, mapping TemplateMapping) string {
	parts := []string{}
	if a.ModelRef != "" {
		parts = append(parts, "模型 "+mapping.ModelRefs[a.ModelRef])
	}
	if a.PersonalityPrompt != "" {
		parts = append(parts, "含人格提示词")
	}
	return strings.Join(parts, "，")
}

// validateMappingCompleteness 要求 agents 段内每个出现的 modelRef 都有映射。
func validateMappingCompleteness(spec *template.Spec, sections []string, mapping TemplateMapping) error {
	if !template.HasSection(sections, "agents") {
		return nil
	}
	seen := map[string]bool{}
	for _, a := range spec.Agents {
		if a.ModelRef == "" || seen[a.ModelRef] {
			continue
		}
		seen[a.ModelRef] = true
		mapped := strings.TrimSpace(mapping.ModelRefs[a.ModelRef])
		if mapped == "" {
			return fmt.Errorf("缺少模型映射：agents 使用 modelRef %q，但 mapping.modelRefs 未提供对应实际模型 ID", a.ModelRef)
		}
		if len(mapped) > 128 || strings.ContainsAny(mapped, " \t\n\r\"'") {
			return fmt.Errorf("modelRef %q 映射到的模型 ID %q 格式不合法", a.ModelRef, mapped)
		}
	}
	return nil
}

// planNames 计算最终资源名并检测冲突；rename 策略下自动追加 -2/-3 后缀。
func (s *TemplateService) planNames(tenantID string, spec *template.Spec, mapping TemplateMapping, sections []string, strategy string, plan *InstallPlan) *resolvedNames {
	names := &resolvedNames{
		agents: map[string]string{}, groups: map[string]string{},
		workflows: map[string]string{}, personalityTemplates: map[string]string{},
	}
	conflicts := []Conflict{}
	take := func(kind, base string, exists func(string) bool) string {
		name := mapping.NamePrefix + base
		if exists(name) {
			if strategy == StrategyRename {
				for i := 2; ; i++ {
					candidate := fmt.Sprintf("%s-%d", name, i)
					if !exists(candidate) {
						plan.Renames = append(plan.Renames, Rename{ResourceType: kind, From: name, To: candidate})
						return candidate
					}
				}
			}
			conflicts = append(conflicts, Conflict{ResourceType: kind, Name: name, Reason: "同名资源已存在"})
		}
		return name
	}

	if template.HasSection(sections, "personalityTemplates") {
		for _, p := range spec.PersonalityTemplates {
			names.personalityTemplates[p.Name] = take("personalityTemplate", p.Name, func(n string) bool {
				return s.db.Where("tenant_id=? AND name=?", tenantID, n).First(&personality.Template{}).Error == nil
			})
		}
	}
	if template.HasSection(sections, "agents") {
		for _, a := range spec.Agents {
			names.agents[a.Name] = take("agent", a.Name, func(n string) bool {
				return s.db.Where("tenant_id=? AND name=?", tenantID, n).First(&agent.AgentConfig{}).Error == nil
			})
		}
	}
	if template.HasSection(sections, "groups") {
		for _, g := range spec.Groups {
			names.groups[g.Name] = take("group", g.Name, func(n string) bool {
				return s.db.Where("tenant_id=? AND name=?", tenantID, n).First(&collaboration.Group{}).Error == nil
			})
		}
	}
	if template.HasSection(sections, "workflows") {
		for _, w := range spec.Workflows {
			names.workflows[w.Name] = take("workflow", w.Name, func(n string) bool {
				return s.db.Where("tenant_id=? AND name=?", tenantID, n).First(&workflow.Definition{}).Error == nil
			})
		}
	}
	// stateSchemas 的身份是 namespace+name+version，无法改名：同内容视为
	// 幂等（安装时跳过），不同内容记为冲突（fail/rename 都需用户介入）。
	if template.HasSection(sections, "stateSchemas") {
		for _, ss := range spec.StateSchemas {
			key := fmt.Sprintf("%s/%s@%s", ss.Namespace, ss.Name, ss.Version)
			var existing rundomain.StateSchema
			err := s.db.Where("tenant_id=? AND namespace=? AND name=? AND version=?",
				tenantID, ss.Namespace, ss.Name, ss.Version).First(&existing).Error
			if err == nil && existing.ContentHash != sha256SumJSON(ss.Schema) {
				conflicts = append(conflicts, Conflict{
					ResourceType: "stateSchema", Name: key,
					Reason: "同名 Schema 已存在且内容不同（Schema 身份不可改名）",
				})
			}
		}
	}
	plan.Conflicts = conflicts
	return names
}

// validateRefResolution 校验 groups.memberRefs / relations refs 能解析到
// 本次安装的 agent 或租户内已存在的 agent（按最终名）。
func (s *TemplateService) validateRefResolution(tenantID string, spec *template.Spec, sections []string, names *resolvedNames) error {
	if template.HasSection(sections, "groups") {
		for _, g := range spec.Groups {
			for _, ref := range g.MemberRefs {
				if names.agents[ref] == "" {
					var id uint64
					if err := s.db.Model(&agent.AgentConfig{}).Select("id").Where("tenant_id=? AND name=?", tenantID, ref).Scan(&id).Error; err != nil || id == 0 {
						return fmt.Errorf("groups(%q).memberRefs 引用的 agent %q 既不在本次安装范围，也不存在于当前租户", g.Name, ref)
					}
					names.agents[ref] = ref // 解析到既有 agent
				}
			}
		}
	}
	if template.HasSection(sections, "relations") {
		for _, r := range spec.Relations {
			for _, ref := range []string{r.FromRef, r.ToRef} {
				if names.agents[ref] == "" {
					var id uint64
					if err := s.db.Model(&agent.AgentConfig{}).Select("id").Where("tenant_id=? AND name=?", tenantID, ref).Scan(&id).Error; err != nil || id == 0 {
						return fmt.Errorf("relations 引用的 agent %q 既不在本次安装范围，也不存在于当前租户", ref)
					}
					names.agents[ref] = ref
				}
			}
		}
	}
	return nil
}

// ---------- Install ----------

type InstallOptions struct {
	Version        string
	Mapping        TemplateMapping
	Sections       []string
	Strategy       string
	Force          bool // 跳过扩展依赖缺失阻断
	IdempotencyKey string
	TargetRunID    string // 提供时 sampleData 写入该运行
	Actor          string
}

type TemplateInstallResult struct {
	Plan             *InstallPlan        `json:"plan"`
	Created          map[string][]string `json:"created"`
	Skipped          []string            `json:"skipped,omitempty"`
	IdempotentReplay bool                `json:"idempotentReplay,omitempty"`
	InstallID        uint64              `json:"installId,omitempty"`
}

// errReplayResult 是事务内检测到并发幂等胜出者时的哨兵错误。
var errReplayResult = errors.New("template install replayed by concurrent winner")

// Install 执行模板安装。流程：幂等检查 → Resolve 预览 → 冲突
// （fail+冲突 → 409）/ 扩展依赖（缺失且未 force → 409）→ 单事务按序创建
// → 事务内写幂等记录 → sampleData（target_run_id 提供时）在事务外写入。
// 任一资源创建失败整体回滚。
func (s *TemplateService) Install(tenantID string, templateID uint64, opts InstallOptions) (*TemplateInstallResult, error) {
	tenantID = normalizedTenant(tenantID)
	if err := validSections(opts.Sections); err != nil {
		return nil, templateErrorf(http.StatusBadRequest, "%v", err)
	}
	key := strings.TrimSpace(opts.IdempotencyKey)
	mHash := mappingHash(opts.Mapping)

	// 幂等重放：同 key 同 mapping 返回首次结果；同 key 不同 mapping 409。
	if key != "" {
		var existing template.TemplateInstall
		if err := s.db.Where("tenant_id=? AND idempotency_key=?", tenantID, key).First(&existing).Error; err == nil {
			if existing.MappingHash != mHash {
				return nil, templateErrorf(http.StatusConflict,
					"幂等键 %q 已被使用且映射不同（模板 %s@%s）", key, existing.TemplateName, existing.Version)
			}
			var result TemplateInstallResult
			if err := json.Unmarshal([]byte(existing.Result), &result); err != nil {
				return nil, templateErrorf(http.StatusInternalServerError, "幂等结果解析失败：%v", err)
			}
			result.IdempotentReplay = true
			result.InstallID = existing.ID
			return &result, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	plan, err := s.Resolve(tenantID, templateID, opts.Version, opts.Mapping, opts.Sections, opts.Strategy)
	if err != nil {
		return nil, err
	}
	if len(plan.Conflicts) > 0 {
		msg := &strings.Builder{}
		fmt.Fprintf(msg, "安装冲突（strategy=fail）：")
		for i, c := range plan.Conflicts {
			if i > 0 {
				msg.WriteString("；")
			}
			fmt.Fprintf(msg, "%s %s（%s）", c.ResourceType, c.Name, c.Reason)
		}
		return nil, &TemplateError{Code: http.StatusConflict, Message: msg.String(), Plan: plan}
	}
	if len(plan.MissingExtensions) > 0 && !opts.Force {
		return nil, &TemplateError{Code: http.StatusConflict,
			Message: fmt.Sprintf("缺少已启用的扩展依赖：%s（可 force=true 跳过依赖继续安装，或先安装扩展）",
				strings.Join(plan.MissingExtensions, "；")),
			Plan: plan}
	}

	def, ver, spec, err := s.loadVersion(tenantID, templateID, opts.Version)
	if err != nil {
		return nil, err
	}
	result := &TemplateInstallResult{Plan: plan, Created: map[string][]string{}}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		names := s.planNames(tenantID, spec, opts.Mapping, opts.Sections, StrategyRename, &InstallPlan{})

		agentIDs := map[string]uint64{}
		// 解析既有 agent（本次不创建的引用）
		for ref, final := range names.agents {
			var id uint64
			if err := tx.Model(&agent.AgentConfig{}).Select("id").Where("tenant_id=? AND name=?", tenantID, final).Scan(&id).Error; err == nil && id > 0 {
				agentIDs[ref] = id
			}
		}

		// 1) personalityTemplates
		if template.HasSection(opts.Sections, "personalityTemplates") {
			for _, p := range spec.PersonalityTemplates {
				pt := personality.Template{
					TenantID: tenantID, Name: names.personalityTemplates[p.Name],
					Title: p.Name, Prompt: p.Content,
					CurrentVersion: 1, Enabled: true,
				}
				if err := tx.Create(&pt).Error; err != nil {
					return fmt.Errorf("创建人格模板 %s 失败：%w", pt.Name, err)
				}
				pv := personality.Version{TemplateID: pt.ID, TenantID: tenantID, Version: 1, Prompt: p.Content, ChangeNote: "模板安装"}
				if err := tx.Create(&pv).Error; err != nil {
					return fmt.Errorf("创建人格模板版本 %s 失败：%w", pt.Name, err)
				}
				result.Created["personalityTemplates"] = append(result.Created["personalityTemplates"], pt.Name)
			}
		}

		// 2) agents
		if template.HasSection(opts.Sections, "agents") {
			for _, a := range spec.Agents {
				cfg := agent.AgentConfig{
					TenantID:          tenantID,
					Name:              names.agents[a.Name],
					SystemPrompt:      a.SystemPrompt,
					PersonalityPrompt: a.PersonalityPrompt,
					PermissionMode:    "auto",
					MaxTurns:          50,
					Title:             map[string]string{"zh-CN": a.Title, "en-US": a.Title},
					Source:            "remote",
				}
				if a.ModelRef != "" {
					cfg.ModelID = strings.TrimSpace(opts.Mapping.ModelRefs[a.ModelRef])
				}
				cfg.ContentHash = sha256SumJSON(cfg)
				if err := tx.Create(&cfg).Error; err != nil {
					return fmt.Errorf("创建 Agent %s 失败：%w", cfg.Name, err)
				}
				agentIDs[a.Name] = cfg.ID
				result.Created["agents"] = append(result.Created["agents"], cfg.Name)
			}
		}

		// 3) groups（含频道与成员）
		if template.HasSection(opts.Sections, "groups") {
			for _, g := range spec.Groups {
				grp := collaboration.Group{
					ID:          uuid.NewString(),
					TenantID:    tenantID,
					Name:        names.groups[g.Name],
					Description: g.Description,
					Visibility:  collaboration.GroupVisibilityTenant,
					CreatedBy:   strings.TrimSpace(opts.Actor),
				}
				if err := tx.Create(&grp).Error; err != nil {
					return fmt.Errorf("创建群组 %s 失败：%w", grp.Name, err)
				}
				for _, ch := range g.Channels {
					channel := collaboration.Channel{
						ID: uuid.NewString(), TenantID: tenantID, GroupID: grp.ID,
						Name: ch, Visibility: collaboration.ChannelVisibilityGroup,
						CreatedBy: strings.TrimSpace(opts.Actor),
					}
					if err := tx.Create(&channel).Error; err != nil {
						return fmt.Errorf("创建频道 %s 失败：%w", ch, err)
					}
				}
				for _, ref := range g.MemberRefs {
					id, ok := agentIDs[ref]
					if !ok {
						return fmt.Errorf("群组 %s 的成员 %s 未解析到 Agent", g.Name, ref)
					}
					member := collaboration.GroupMember{
						TenantID: tenantID, GroupID: grp.ID, AgentID: id,
						Role: collaboration.RoleMember,
						// MySQL NO_ZERO_DATE 拒绝零值时间，SQLite 恰好放行——本地测试
						// 全绿但生产安装必失败，必须显式赋值（参考 collaboration_service）。
						JoinedAt: time.Now().UTC(),
					}
					if err := tx.Create(&member).Error; err != nil {
						return fmt.Errorf("添加群组成员 %s 失败：%w", ref, err)
					}
				}
				result.Created["groups"] = append(result.Created["groups"], grp.Name)
			}
		}

		// 4) relations
		if template.HasSection(opts.Sections, "relations") {
			for _, r := range spec.Relations {
				fromID, ok1 := agentIDs[r.FromRef]
				toID, ok2 := agentIDs[r.ToRef]
				if !ok1 || !ok2 {
					return fmt.Errorf("关系 %s->%s 引用的 Agent 未解析", r.FromRef, r.ToRef)
				}
				rel := agentrelation.AgentRelation{
					TenantID:       tenantID,
					Scope:          agentrelation.DefaultScope,
					SourceAgentID:  fromID,
					TargetAgentID:  toID,
					RelationType:   r.RelationType,
					Stance:         "neutral",
					AllowedActions: r.AllowedActions,
					ContextPolicy:  agentrelation.DefaultContextPolicy,
					DeliveryPolicy: agentrelation.DefaultDeliveryPolicy,
					Enabled:        true,
				}
				if err := tx.Create(&rel).Error; err != nil {
					return fmt.Errorf("创建关系 %s->%s 失败：%w", r.FromRef, r.ToRef, err)
				}
				result.Created["relations"] = append(result.Created["relations"],
					fmt.Sprintf("%s->%s(%s)", names.agents[r.FromRef], names.agents[r.ToRef], r.RelationType))
			}
		}

		// 5) workflows（Definition + Version(draft) + Steps）
		if template.HasSection(opts.Sections, "workflows") {
			for _, w := range spec.Workflows {
				wfDef := workflow.Definition{
					ID:          uuid.NewString(),
					TenantID:    tenantID,
					Name:        names.workflows[w.Name],
					Description: w.Description,
					CreatedBy:   strings.TrimSpace(opts.Actor),
				}
				if err := tx.Create(&wfDef).Error; err != nil {
					return fmt.Errorf("创建工作流 %s 失败：%w", wfDef.Name, err)
				}
				wfVer := workflow.Version{
					ID:         uuid.NewString(),
					TenantID:   tenantID,
					WorkflowID: wfDef.ID,
					Version:    1,
					Status:     workflow.VersionDraft,
					CreatedBy:  strings.TrimSpace(opts.Actor),
				}
				if err := tx.Create(&wfVer).Error; err != nil {
					return fmt.Errorf("创建工作流版本 %s 失败：%w", wfDef.Name, err)
				}
				for _, st := range w.Steps {
					step := workflow.Step{
						ID:             uuid.NewString(),
						TenantID:       tenantID,
						VersionID:      wfVer.ID,
						Key:            st.Key,
						Name:           firstNonEmpty(st.Name, st.Key),
						Type:           firstNonEmpty(st.Type, "agent"),
						ActorType:      firstNonEmpty(st.ActorType, "agent"),
						ActorRef:       st.ActorRef,
						DependsOn:      st.DependsOn,
						Config:         st.Config,
						TimeoutSeconds: st.TimeoutSeconds,
						MaxRetries:     st.MaxRetries,
					}
					if err := tx.Create(&step).Error; err != nil {
						return fmt.Errorf("创建工作流步骤 %s/%s 失败：%w", wfDef.Name, st.Key, err)
					}
				}
				result.Created["workflows"] = append(result.Created["workflows"], wfDef.Name)
			}
		}

		// 6) stateSchemas（spec 注册时已编译校验；与 RunService.RegisterStateSchema 同语义；
		// 同内容已注册则幂等跳过）
		if template.HasSection(opts.Sections, "stateSchemas") {
			for _, ss := range spec.StateSchemas {
				hash := sha256SumJSON(ss.Schema)
				var existing rundomain.StateSchema
				err := tx.Where("tenant_id=? AND namespace=? AND name=? AND version=?",
					tenantID, ss.Namespace, ss.Name, ss.Version).First(&existing).Error
				if err == nil {
					if existing.ContentHash == hash {
						continue // 同内容已注册：幂等跳过
					}
					return fmt.Errorf("状态 Schema %s/%s@%s 已存在且内容不同（Schema 身份不可改名）", ss.Namespace, ss.Name, ss.Version)
				}
				row := rundomain.StateSchema{
					TenantID:     tenantID,
					Namespace:    ss.Namespace,
					Name:         ss.Name,
					Version:      ss.Version,
					Schema:       ss.Schema,
					ScopeTypes:   []string{"run"},
					SubjectTypes: []string{"agent"},
					ContentHash:  hash,
				}
				if err := tx.Create(&row).Error; err != nil {
					return fmt.Errorf("注册状态 Schema %s/%s@%s 失败：%w", ss.Namespace, ss.Name, ss.Version, err)
				}
				result.Created["stateSchemas"] = append(result.Created["stateSchemas"],
					fmt.Sprintf("%s/%s@%s", ss.Namespace, ss.Name, ss.Version))
			}
		}

		// 幂等记录随事务提交（并发同 key 时唯一索引保证只有一个成功）。
		if key != "" {
			raw, err := json.Marshal(result)
			if err != nil {
				return err
			}
			record := template.TemplateInstall{
				TenantID: tenantID, TemplateID: def.ID, TemplateName: def.Name,
				Version: ver.Version, MappingHash: mHash,
				IdempotencyKey: key, Result: string(raw),
			}
			if err := tx.Create(&record).Error; err != nil {
				if isDuplicate(err) {
					// 并发胜出者已落库：转为幂等重放
					var winner template.TemplateInstall
					if findErr := tx.Where("tenant_id=? AND idempotency_key=?", tenantID, key).First(&winner).Error; findErr == nil && winner.MappingHash == mHash {
						return errReplayResult
					}
				}
				return err
			}
			result.InstallID = record.ID
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errReplayResult) {
			var existing template.TemplateInstall
			if findErr := s.db.Where("tenant_id=? AND idempotency_key=?", tenantID, key).First(&existing).Error; findErr == nil {
				var replay TemplateInstallResult
				if uerr := json.Unmarshal([]byte(existing.Result), &replay); uerr == nil {
					replay.IdempotentReplay = true
					replay.InstallID = existing.ID
					return &replay, nil
				}
			}
		}
		return nil, err
	}

	// 7) sampleData：仅在提供 target_run_id 时写入该运行（主事务外，失败
	// 不影响已创建的配置，但在结果里标注）。未提供 → skipped。
	if template.HasSection(opts.Sections, "sampleData") && len(spec.SampleData) > 0 {
		if strings.TrimSpace(opts.TargetRunID) == "" {
			result.Skipped = append(result.Skipped,
				fmt.Sprintf("sampleData：未提供 target_run_id，跳过 %d 条种子状态写入", len(spec.SampleData)))
		} else if s.run == nil {
			result.Skipped = append(result.Skipped, "sampleData：RunService 未注入，无法写入种子状态")
		} else {
			// sampleData 的 schema 定位约定：与 stateSchemas 段中同 namespace
			// 的条目同名同版本。
			schemaByNS := map[string]template.StateSchemaSpec{}
			for _, ss := range spec.StateSchemas {
				schemaByNS[ss.Namespace] = ss
			}
			for i, sd := range spec.SampleData {
				if sd.Kind != "state" {
					result.Skipped = append(result.Skipped, fmt.Sprintf("sampleData[%d]：kind %q 不支持", i, sd.Kind))
					continue
				}
				ss, ok := schemaByNS[sd.Namespace]
				if !ok {
					result.Skipped = append(result.Skipped,
						fmt.Sprintf("sampleData[%d]：namespace %q 未在 stateSchemas 段声明，无法定位 Schema", i, sd.Namespace))
					continue
				}
				_, err := s.run.InitializeState(tenantID, opts.TargetRunID, InitializeStateInput{
					Namespace:      sd.Namespace,
					SchemaName:     ss.Name,
					SchemaVersion:  ss.Version,
					SubjectType:    sd.SubjectType,
					SubjectID:      sd.SubjectID,
					Data:           sd.Data,
					IdempotencyKey: fmt.Sprintf("template-install:%s:%s:%d", def.Name, ver.Version, i),
					Reason:         "模板安装种子数据",
					Source:         "template-install",
				})
				if err != nil {
					result.Skipped = append(result.Skipped, fmt.Sprintf("sampleData[%d]：写入失败：%v", i, err))
				} else {
					result.Created["sampleData"] = append(result.Created["sampleData"],
						fmt.Sprintf("%s/%s:%s", sd.Namespace, sd.SubjectType, sd.SubjectID))
				}
			}
		}
	}
	return result, nil
}

// sha256SumJSON 计算任意 JSON 可序列化值的稳定内容哈希。
func sha256SumJSON(v any) string {
	raw, _ := json.Marshal(v)
	return sha256Hex(raw)
}

func sha256Hex(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
