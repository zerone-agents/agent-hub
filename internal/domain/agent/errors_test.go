package agent

import (
	"strings"
	"testing"
)

func TestDatasetInUseError_ErrorListsDatasetsWithAgents(t *testing.T) {
	err := &DatasetInUseError{Datasets: []DatasetInUseItem{
		{ID: "kb-1", Agents: []string{"agent-a", "agent-b"}},
		{ID: "kb-2", Agents: []string{"agent-c"}, Foreign: true},
		{ID: "kb-3"},
	}}
	msg := err.Error()
	for _, want := range []string{
		"请先解除绑定",
		"kb-1（agent-a、agent-b）",
		"kb-2（agent-c；另被其他租户使用）",
		"kb-3（仍被其他租户使用）",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}

func TestSkillInUseError_ThreeStateMessage(t *testing.T) {
	cases := []struct {
		name    string
		err     *SkillInUseError
		expects []string
	}{
		{"own-only", &SkillInUseError{SkillName: "s1", Agents: []string{"a", "b"}}, []string{"技能 's1' 仍被 Agent 使用：a、b，请先解除绑定"}},
		{"own-foreign", &SkillInUseError{SkillName: "s2", Agents: []string{"a"}, Foreign: true}, []string{"技能 's2' 仍被 Agent 使用：a（另被其他租户使用），请先解除绑定"}},
		{"foreign-only", &SkillInUseError{SkillName: "s3", Foreign: true}, []string{"技能 's3' 仍被其他租户使用，请先解除绑定"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := c.err.Error()
			for _, want := range c.expects {
				if !strings.Contains(msg, want) {
					t.Fatalf("message %q missing %q", msg, want)
				}
			}
		})
	}
}

func TestMcpInUseError_ThreeStateMessage(t *testing.T) {
	cases := []struct {
		name    string
		err     *McpInUseError
		expects []string
	}{
		{"own-only", &McpInUseError{McpName: "m1", Agents: []string{"a"}}, []string{"MCP 'm1' 仍被 Agent 使用：a，请先解除绑定"}},
		{"own-foreign", &McpInUseError{McpName: "m2", Agents: []string{"a"}, Foreign: true}, []string{"MCP 'm2' 仍被 Agent 使用：a（另被其他租户使用），请先解除绑定"}},
		{"foreign-only", &McpInUseError{McpName: "m3", Foreign: true}, []string{"MCP 'm3' 仍被其他租户使用，请先解除绑定"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := c.err.Error()
			for _, want := range c.expects {
				if !strings.Contains(msg, want) {
					t.Fatalf("message %q missing %q", msg, want)
				}
			}
		})
	}
}

func TestToolInUseError_ForeignStates(t *testing.T) {
	cases := []struct {
		name    string
		err     *ToolInUseError
		expects []string
	}{
		{"own-only", &ToolInUseError{ToolName: "t1", Agents: []string{"a"}}, []string{"Tool 't1' 仍被以下 Agent 挂载：a，请先解除关联"}},
		{"own-foreign", &ToolInUseError{ToolName: "t2", Agents: []string{"a"}, Foreign: true}, []string{"Tool 't2' 仍被以下 Agent 挂载：a（另被其他租户使用），请先解除关联"}},
		{"foreign-only", &ToolInUseError{ToolName: "t3", Foreign: true}, []string{"Tool 't3' 仍被其他租户挂载，请先解除关联"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := c.err.Error()
			for _, want := range c.expects {
				if !strings.Contains(msg, want) {
					t.Fatalf("message %q missing %q", msg, want)
				}
			}
		})
	}
}
