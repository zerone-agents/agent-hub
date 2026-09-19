// H7.3 模板 spec 校验测试：断引用 400 级错误（中文）、闭环通过、
// modelRef/JSON Schema/DNS 名/版本范围校验。
package template

import (
	"strings"
	"testing"
)

func validSpecForTest() *Spec {
	return &Spec{
		Agents: []AgentSpec{
			{Name: "coordinator", ModelRef: "default/general"},
			{Name: "executor", ModelRef: "default/general"},
		},
		Groups: []GroupSpec{
			{Name: "core-team", Channels: []string{"general"}, MemberRefs: []string{"coordinator", "executor"}},
		},
		Relations: []RelationSpec{
			{FromRef: "executor", ToRef: "coordinator", RelationType: "reports_to", AllowedActions: []string{"report"}},
		},
		Workflows: []WorkflowSpec{
			{Name: "daily-standup", Steps: []WorkflowStepSpec{
				{Key: "collect", Name: "收集进展"},
				{Key: "review", Name: "评审", DependsOn: []string{"collect"}},
			}},
		},
		StateSchemas: []StateSchemaSpec{
			{
				Namespace: "io.zerone.test", Name: "profile", Version: "1.0.0",
				Schema: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties":           map[string]any{"score": map[string]any{"type": "integer"}},
					"required":             []any{"score"},
				},
			},
		},
		ExtensionDeps: []ExtensionDepSpec{{Name: "io.zerone.organization.emotion", VersionRange: "^1.0.0"}},
		SampleData:    []SampleDataSpec{{Kind: "state", Namespace: "io.zerone.test", SubjectType: "agent", SubjectID: "coordinator", Data: map[string]any{"score": 1}}},
	}
}

func TestValidateTemplateSpecValid(t *testing.T) {
	if errs := ValidateTemplateSpec(validSpecForTest()); len(errs) != 0 {
		t.Fatalf("合法 spec 应通过校验，实际错误：%v", errs)
	}
}

func TestValidateTemplateSpecBrokenRefs(t *testing.T) {
	spec := validSpecForTest()
	spec.Groups[0].MemberRefs = []string{"ghost"}
	spec.Relations[0].FromRef = "nobody"
	errs := ValidateTemplateSpec(spec)
	if len(errs) == 0 {
		t.Fatal("断引用应报错")
	}
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "ghost") || !strings.Contains(joined, "nobody") {
		t.Fatalf("错误信息应指出断引用名称：%s", joined)
	}
}

func TestValidateTemplateSpecRelationTypeAndActions(t *testing.T) {
	spec := validSpecForTest()
	spec.Relations[0].RelationType = "not-a-type"
	spec.Relations[0].AllowedActions = []string{"hack"}
	errs := ValidateTemplateSpec(spec)
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "not-a-type") || !strings.Contains(joined, "hack") {
		t.Fatalf("应指出非法关系类型与动作：%s", joined)
	}
}

func TestValidateTemplateSpecModelRef(t *testing.T) {
	spec := validSpecForTest()
	spec.Agents[0].ModelRef = "bad ref!"
	errs := ValidateTemplateSpec(spec)
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "；"), "modelRef") {
		t.Fatalf("非法 modelRef 应报错：%v", errs)
	}
}

func TestValidateTemplateSpecInvalidJSONSchema(t *testing.T) {
	spec := validSpecForTest()
	spec.StateSchemas[0].Schema = map[string]any{"type": "not-a-type"}
	errs := ValidateTemplateSpec(spec)
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "；"), "JSON Schema") {
		t.Fatalf("非法 JSON Schema 应报错：%v", errs)
	}
	// 非对象 schema（数组）
	spec = validSpecForTest()
	spec.StateSchemas[0].Schema = map[string]any{}
	errs = ValidateTemplateSpec(spec)
	if len(errs) == 0 {
		t.Fatal("空 schema 对象应报错")
	}
}

func TestValidateTemplateSpecBadExtensionDep(t *testing.T) {
	spec := validSpecForTest()
	spec.ExtensionDeps[0].Name = "Not_DNS"
	spec.ExtensionDeps = append(spec.ExtensionDeps, ExtensionDepSpec{Name: "io.zerone.ok", VersionRange: "^^bad"})
	errs := ValidateTemplateSpec(spec)
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "DNS") || !strings.Contains(joined, "versionRange") {
		t.Fatalf("非法扩展依赖应报错：%s", joined)
	}
}

func TestValidateTemplateSpecDuplicateNamesAndSteps(t *testing.T) {
	spec := validSpecForTest()
	spec.Agents = append(spec.Agents, AgentSpec{Name: "coordinator"})
	spec.Workflows[0].Steps = append(spec.Workflows[0].Steps, WorkflowStepSpec{Key: "collect"})
	spec.Workflows[0].Steps = append(spec.Workflows[0].Steps, WorkflowStepSpec{Key: "missing-dep", DependsOn: []string{"ghost-step"}})
	errs := ValidateTemplateSpec(spec)
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "重复名称") || !strings.Contains(joined, "重复 step key") || !strings.Contains(joined, "ghost-step") {
		t.Fatalf("重复名/步骤与 dependsOn 断引用应报错：%s", joined)
	}
}

func TestHasSection(t *testing.T) {
	if !HasSection(nil, "agents") {
		t.Fatal("空 sections 应表示全选")
	}
	if HasSection([]string{"agents"}, "groups") {
		t.Fatal("部分选择应只含所选段")
	}
}
