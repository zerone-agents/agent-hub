package handler

import (
	"testing"

	"control-panel/internal/application/services"
	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
)

type runTraceStub struct {
	run        *rundomain.Run
	activities []services.AppendRunActivityInput
}

func (s *runTraceStub) Get(_, _ string) (*rundomain.Run, error) { return s.run, nil }

func (s *runTraceStub) AppendActivity(_, _ string, input services.AppendRunActivityInput) (*rundomain.RunActivity, error) {
	s.activities = append(s.activities, input)
	return &rundomain.RunActivity{Kind: input.Kind}, nil
}

func TestExtractRuntimeActivities_ToolLifecycleAndCompletion(t *testing.T) {
	sse := `event: system
data: {"type":"system","subtype":"init","session_id":"runtime-1"}

event: assistant
data: {"message":{"content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"tool-1","name":"search","input":{"query":"agent hub"}}]}}

event: tool_result
data: {"result":{"tool_use_id":"tool-1","tool_name":"search","output":{"matches":3}}}

event: result
data: {"type":"result","subtype":"success"}

`

	got := extractRuntimeActivities(sse)
	require.Equal(t, []runtimeActivity{
		{Kind: "tool_started", StepID: "tool-1", Name: "search", Status: "running", InputJSON: `{"query":"agent hub"}`},
		{Kind: "tool_finished", StepID: "tool-1", Name: "search", Status: "completed", OutputJSON: `{"matches":3}`},
		{Kind: "completed", Status: "completed"},
	}, got)
}

func TestExtractRuntimeActivities_FailedToolAndRun(t *testing.T) {
	sse := `event: tool_result
data: {"result":{"tool_use_id":"tool-2","tool_name":"fetch","output":"denied","is_error":true}}

event: result
data: {"type":"result","subtype":"error","error_type":"permission_denied","errors":["工具没有权限"]}

`

	got := extractRuntimeActivities(sse)
	require.Equal(t, "failed", got[0].Status)
	require.Equal(t, `"denied"`, got[0].OutputJSON)
	require.Equal(t, runtimeActivity{Kind: "failed", Status: "failed", Error: "工具没有权限"}, got[1])
}

func TestExtractRuntimeActivities_IgnoresMalformedAndUnknownEvents(t *testing.T) {
	sse := "event: assistant\ndata: nope\n\nevent: future_event\ndata: {\"x\":1}\n\n"
	require.Empty(t, extractRuntimeActivities(sse))
}

func TestValidateRunParticipant_RequiresRunningAndBoundAgent(t *testing.T) {
	stub := &runTraceStub{run: &rundomain.Run{
		Status: rundomain.StatusRunning,
		Agents: []rundomain.RunAgent{{AgentNameSnapshot: "Research-Agent"}},
	}}
	h := NewAgentChatHandler(nil, stub)

	require.NoError(t, h.validateRunParticipant("tenant-a", "run-a", "research-agent"))
	require.EqualError(t, h.validateRunParticipant("tenant-a", "run-a", "other-agent"), "当前 Agent 不在该运行的参与者中")

	stub.run.Status = rundomain.StatusPaused
	require.EqualError(t, h.validateRunParticipant("tenant-a", "run-a", "research-agent"), "运行当前不是执行中状态")
}

func TestAppendRunActivity_IsBestEffort(t *testing.T) {
	stub := &runTraceStub{}
	h := NewAgentChatHandler(nil, stub)
	h.appendRunActivity("tenant-a", "run-a", services.AppendRunActivityInput{Kind: "started", StepID: "exec-1"})
	require.Len(t, stub.activities, 1)
	require.Equal(t, "exec-1", stub.activities[0].StepID)
}
