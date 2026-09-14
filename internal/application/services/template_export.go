// H7.3 模板反向导出：从租户现有配置（agents/groups/relations/workflows/
// stateSchemas/personalityTemplates）读取并生成模板 spec JSON，可作为新
// 模板注册（Register 接受同结构 spec，round-trip 可复装）。
package services

import (
	"fmt"
	"sort"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/personality"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/template"
	"control-panel/internal/domain/workflow"
)

// ExportSelection 是导出选择：AgentIDs/GroupIDs 为空表示全量导出对应域。
type ExportSelection struct {
	AgentIDs []uint64
	GroupIDs []string
}

// Export 从当前租户配置反向生成模板 spec。agents 的 modelId 导出为
// modelRef（安装时需经 mapping 映射）；personalityPrompt 直接内联。
func (s *TemplateService) Export(tenantID string, sel ExportSelection) (*template.Spec, error) {
	tenantID = normalizedTenant(tenantID)
	spec := &template.Spec{}

	// ---- agents ----
	agentQ := s.db.Where("tenant_id=?", tenantID).Order("name")
	if len(sel.AgentIDs) > 0 {
		agentQ = agentQ.Where("id IN ?", sel.AgentIDs)
	}
	var agents []agent.AgentConfig
	if err := agentQ.Find(&agents).Error; err != nil {
		return nil, err
	}
	agentIDSet := map[uint64]bool{}
	agentNameByID := map[uint64]string{}
	for _, a := range agents {
		agentIDSet[a.ID] = true
		agentNameByID[a.ID] = a.Name
		title := a.Name
		if a.Title != nil {
			if v, ok := a.Title["zh-CN"]; ok && v != "" {
				title = v
			} else {
				// 取第一个非空值，保证导出稳定（按键排序）。
				keys := make([]string, 0, len(a.Title))
				for k := range a.Title {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if a.Title[k] != "" {
						title = a.Title[k]
						break
					}
				}
			}
		}
		as := template.AgentSpec{
			Name:              a.Name,
			Title:             title,
			SystemPrompt:      a.SystemPrompt,
			PersonalityPrompt: a.PersonalityPrompt,
		}
		if a.ModelID != "" {
			as.ModelRef = a.ModelID // 导出为符号引用，安装时经 mapping 映射
		}
		spec.Agents = append(spec.Agents, as)
	}

	// ---- personalityTemplates（被导出 agent 引用的）----
	if len(agents) > 0 {
		names := map[string]bool{}
		for _, a := range agents {
			if a.PersonalityTemplateName != "" {
				names[a.PersonalityTemplateName] = true
			}
		}
		ordered := make([]string, 0, len(names))
		for n := range names {
			ordered = append(ordered, n)
		}
		sort.Strings(ordered)
		for _, n := range ordered {
			var pt personality.Template
			if err := s.db.Where("tenant_id=? AND name=?", tenantID, n).First(&pt).Error; err != nil {
				continue
			}
			spec.PersonalityTemplates = append(spec.PersonalityTemplates, template.PersonalityTemplateSpec{
				Name: pt.Name, Content: pt.Prompt,
			})
		}
	}

	// ---- groups ----
	groupQ := s.db.Where("tenant_id=?", tenantID).Order("name")
	if len(sel.GroupIDs) > 0 {
		groupQ = groupQ.Where("id IN ?", sel.GroupIDs)
	}
	var groups []collaboration.Group
	if err := groupQ.Find(&groups).Error; err != nil {
		return nil, err
	}
	for _, g := range groups {
		gs := template.GroupSpec{Name: g.Name, Description: g.Description}
		var channels []collaboration.Channel
		if err := s.db.Where("tenant_id=? AND group_id=?", tenantID, g.ID).Order("name").Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, ch := range channels {
			gs.Channels = append(gs.Channels, ch.Name)
		}
		var members []collaboration.GroupMember
		if err := s.db.Where("tenant_id=? AND group_id=?", tenantID, g.ID).Find(&members).Error; err != nil {
			return nil, err
		}
		sort.Slice(members, func(i, j int) bool { return members[i].AgentID < members[j].AgentID })
		for _, m := range members {
			if name, ok := agentNameByID[m.AgentID]; ok {
				gs.MemberRefs = append(gs.MemberRefs, name)
			}
		}
		spec.Groups = append(spec.Groups, gs)
	}

	// ---- relations（仅导出两端都在导出范围内的）----
	var relations []agentrelation.AgentRelation
	if err := s.db.Where("tenant_id=? AND scope=?", tenantID, agentrelation.DefaultScope).Find(&relations).Error; err != nil {
		return nil, err
	}
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].SourceAgentID != relations[j].SourceAgentID {
			return relations[i].SourceAgentID < relations[j].SourceAgentID
		}
		return relations[i].TargetAgentID < relations[j].TargetAgentID
	})
	for _, r := range relations {
		if !agentIDSet[r.SourceAgentID] || !agentIDSet[r.TargetAgentID] {
			continue
		}
		spec.Relations = append(spec.Relations, template.RelationSpec{
			FromRef:        agentNameByID[r.SourceAgentID],
			ToRef:          agentNameByID[r.TargetAgentID],
			RelationType:   r.RelationType,
			AllowedActions: append([]string(nil), r.AllowedActions...),
		})
	}

	// ---- workflows（全部定义 + 最新版本步骤）----
	var defs []workflow.Definition
	if err := s.db.Where("tenant_id=?", tenantID).Order("name").Find(&defs).Error; err != nil {
		return nil, err
	}
	for _, d := range defs {
		ws := template.WorkflowSpec{Name: d.Name, Description: d.Description}
		var ver workflow.Version
		err := s.db.Where("tenant_id=? AND workflow_id=?", tenantID, d.ID).
			Order("version DESC").First(&ver).Error
		if err != nil {
			// 无版本的定义导出为空步骤工作流
			spec.Workflows = append(spec.Workflows, ws)
			continue
		}
		var steps []workflow.Step
		if err := s.db.Where("tenant_id=? AND version_id=?", tenantID, ver.ID).Order("created_at").Find(&steps).Error; err != nil {
			return nil, err
		}
		for _, st := range steps {
			ws.Steps = append(ws.Steps, template.WorkflowStepSpec{
				Key:            st.Key,
				Name:           st.Name,
				Type:           st.Type,
				ActorType:      st.ActorType,
				ActorRef:       st.ActorRef,
				DependsOn:      append([]string(nil), st.DependsOn...),
				Config:         st.Config,
				TimeoutSeconds: st.TimeoutSeconds,
				MaxRetries:     st.MaxRetries,
			})
		}
		spec.Workflows = append(spec.Workflows, ws)
	}

	// ---- stateSchemas（全量）----
	var schemas []rundomain.StateSchema
	if err := s.db.Where("tenant_id=?", tenantID).Order("namespace, name, version").Find(&schemas).Error; err != nil {
		return nil, err
	}
	for _, ss := range schemas {
		spec.StateSchemas = append(spec.StateSchemas, template.StateSchemaSpec{
			Namespace: ss.Namespace, Name: ss.Name, Version: ss.Version, Schema: ss.Schema,
		})
	}

	if errs := template.ValidateTemplateSpec(spec); len(errs) > 0 {
		return nil, fmt.Errorf("导出结果未通过模板校验：%s", strings.Join(errs, "；"))
	}
	return spec, nil
}
