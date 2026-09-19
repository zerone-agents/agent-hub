// H7.3 模板 spec 严格校验：引用闭环（memberRefs/relation refs 指向模板内
// 存在的 agent）、命名规范（agent/group/workflow/人格模板名）、modelRef 格式、
// stateSchemas 必须是可编译的 JSON Schema 对象、extensionDeps 为 DNS 式名 +
// 合法版本范围。全部错误一次性收集并返回中文描述。
package template

import (
	"fmt"
	"regexp"
	"strings"

	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/extension"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	templateDNSNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9-]*)+$`)
	templateIdentPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
	// modelRef 允许 provider/model 或裸模型标识（字母数字 ._-/）。
	modelRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
)

// ValidateTemplateSpec 严格校验模板 spec，返回全部错误（中文）；空切片
// 表示通过。引用闭环规则：groups[].memberRefs 与 relations[].fromRef/toRef
// 必须指向 agents[].name 中存在的名称。
func ValidateTemplateSpec(spec *Spec) []string {
	errs := []string{}
	if spec == nil {
		return []string{"spec 不能为空"}
	}
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	agentNames := map[string]int{}
	for i, a := range spec.Agents {
		if !templateIdentPattern.MatchString(a.Name) {
			add("agents[%d].name %q 不符合命名规范（字母开头，仅含字母数字 ._-，≤64 字符）", i, a.Name)
			continue
		}
		agentNames[a.Name]++
		if a.ModelRef != "" && !modelRefPattern.MatchString(a.ModelRef) {
			add("agents[%d].modelRef %q 格式不合法（仅允许字母数字 ._-/）", i, a.ModelRef)
		}
	}
	for name, n := range agentNames {
		if n > 1 {
			add("agents 中存在重复名称 %q", name)
		}
	}

	seenPT := map[string]bool{}
	for i, p := range spec.PersonalityTemplates {
		if !templateIdentPattern.MatchString(p.Name) {
			add("personalityTemplates[%d].name %q 不符合命名规范", i, p.Name)
			continue
		}
		if seenPT[p.Name] {
			add("personalityTemplates 中存在重复名称 %q", p.Name)
		}
		seenPT[p.Name] = true
	}

	seenGroup := map[string]bool{}
	for i, g := range spec.Groups {
		if !templateIdentPattern.MatchString(g.Name) {
			add("groups[%d].name %q 不符合命名规范", i, g.Name)
			continue
		}
		if seenGroup[g.Name] {
			add("groups 中存在重复名称 %q", g.Name)
		}
		seenGroup[g.Name] = true
		for _, ref := range g.MemberRefs {
			if agentNames[ref] == 0 {
				add("groups[%d](%q).memberRefs 引用了不存在的 agent %q", i, g.Name, ref)
			}
		}
	}

	for i, r := range spec.Relations {
		if agentNames[r.FromRef] == 0 {
			add("relations[%d].fromRef %q 不存在于 agents 中", i, r.FromRef)
		}
		if agentNames[r.ToRef] == 0 {
			add("relations[%d].toRef %q 不存在于 agents 中", i, r.ToRef)
		}
		if _, ok := agentrelation.RelationTypes[r.RelationType]; !ok {
			add("relations[%d].relationType %q 不是合法关系类型（可选：%s）", i, r.RelationType, relationTypeList())
		}
		for _, act := range r.AllowedActions {
			if _, ok := agentrelation.Actions[act]; !ok {
				add("relations[%d].allowedActions 包含非法动作 %q", i, act)
			}
		}
	}

	seenWF := map[string]bool{}
	for i, w := range spec.Workflows {
		if !templateIdentPattern.MatchString(w.Name) {
			add("workflows[%d].name %q 不符合命名规范", i, w.Name)
			continue
		}
		if seenWF[w.Name] {
			add("workflows 中存在重复名称 %q", w.Name)
		}
		seenWF[w.Name] = true
		stepKeys := map[string]bool{}
		for j, st := range w.Steps {
			if !templateIdentPattern.MatchString(st.Key) {
				add("workflows[%d](%q).steps[%d].key %q 不符合命名规范", i, w.Name, j, st.Key)
				continue
			}
			if stepKeys[st.Key] {
				add("workflows[%d](%q) 存在重复 step key %q", i, w.Name, st.Key)
			}
			stepKeys[st.Key] = true
		}
		for j, st := range w.Steps {
			for _, dep := range st.DependsOn {
				if !stepKeys[dep] {
					add("workflows[%d](%q).steps[%d](%q).dependsOn 引用了不存在的 step %q", i, w.Name, j, st.Key, dep)
				}
			}
		}
	}

	seenSchema := map[string]bool{}
	for i, ss := range spec.StateSchemas {
		if !templateDNSNamePattern.MatchString(ss.Namespace) {
			add("stateSchemas[%d].namespace %q 不符合 DNS 式命名（如 io.zerone.example）", i, ss.Namespace)
		}
		if !templateIdentPattern.MatchString(ss.Name) {
			add("stateSchemas[%d].name %q 不符合命名规范", i, ss.Name)
		}
		if ss.Version == "" || !extension.IsValidVersion(ss.Version) {
			add("stateSchemas[%d].version %q 不是合法语义化版本", i, ss.Version)
		}
		key := ss.Namespace + "/" + ss.Name + "@" + ss.Version
		if seenSchema[key] {
			add("stateSchemas 中存在重复定义 %q", key)
		}
		seenSchema[key] = true
		if len(ss.Schema) == 0 {
			add("stateSchemas[%d](%q).schema 不能为空", i, ss.Name)
		} else if err := compileJSONSchema(ss.Schema); err != nil {
			add("stateSchemas[%d](%q).schema 不是合法的 JSON Schema：%v", i, ss.Name, err)
		}
	}

	for i, dep := range spec.ExtensionDeps {
		if !templateDNSNamePattern.MatchString(dep.Name) {
			add("extensionDeps[%d].name %q 不符合 DNS 式命名（如 io.zerone.example）", i, dep.Name)
		}
		if strings.TrimSpace(dep.VersionRange) != "" {
			if _, err := extension.SatisfiesRange("0.0.1", dep.VersionRange); err != nil {
				add("extensionDeps[%d](%q).versionRange %q 不合法：%v", i, dep.Name, dep.VersionRange, err)
			}
		}
	}

	for i, sd := range spec.SampleData {
		if sd.Kind != "state" {
			add("sampleData[%d].kind %q 不受支持（仅支持 state）", i, sd.Kind)
		}
		if !templateDNSNamePattern.MatchString(sd.Namespace) {
			add("sampleData[%d].namespace %q 不符合 DNS 式命名", i, sd.Namespace)
		}
		if strings.TrimSpace(sd.SubjectType) == "" || strings.TrimSpace(sd.SubjectID) == "" {
			add("sampleData[%d] 缺少 subjectType 或 subjectId", i)
		}
	}

	return errs
}

func relationTypeList() string {
	names := make([]string, 0, len(agentrelation.RelationTypes))
	for k := range agentrelation.RelationTypes {
		names = append(names, k)
	}
	return strings.Join(names, "、")
}

// compileJSONSchema 用 JSON Schema 编译器验证 schema 对象可编译。
func compileJSONSchema(schema map[string]any) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schema); err != nil {
		return err
	}
	_, err := compiler.Compile("schema.json")
	return err
}
