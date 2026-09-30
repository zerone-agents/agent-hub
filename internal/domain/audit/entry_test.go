package audit

import (
	"encoding/json"
	"testing"

	"control-panel/internal/domain/aigc"

	"github.com/stretchr/testify/require"
)

func TestSimpleEventDefaults(t *testing.T) {
	e := SimpleEvent(Actor{UserID: "1"}, ActionDeploy, TargetAgent, "demo", "demo")
	require.Equal(t, CatAgent, e.Category)
	require.Equal(t, StatusSuccess, e.Status)
	require.Nil(t, e.Detail)
}

func TestEventCategoryConsistency(t *testing.T) {
	// 构造器的 Category 必须与其 Action 前缀一致（防错配）
	require.Equal(t, "auth", string(LoginEvent(Actor{}, "u", "o", StatusSuccess, "").Category))
	require.Equal(t, "user", string(RoleChangedEvent(Actor{}, "1", "n", "a", "b", StatusSuccess).Category))
	require.Equal(t, "invite", string(InviteCreatedEvent(Actor{}, "9", "member", 3).Category))
	require.Equal(t, "aigc", string(AigcSavedEvent(Actor{}, []aigc.AigcConfigField{}).Category))
	require.Equal(t, "agent", string(BindingsUpdatedEvent(Actor{}, BindingKindSkill, "bot", nil).Category))
	require.Equal(t, "agent", string(AgentUpdatedEvent(Actor{}, "bot", nil).Category))
}

// categoryOf 对「前缀 == 分类名」的 action 走 default 派生（#210 审查建议：
// 显式分支冗余已移除，本测试钉住派生结果防未来默认值漂移）。
func TestCategoryOfPrefixDerivation(t *testing.T) {
	cases := map[Action]Category{
		ActionSkillCreate:         CatSkill,
		ActionSkillUpdate:         CatSkill,
		ActionSkillDelete:         CatSkill,
		ActionToolCreate:          CatTool,
		ActionToolUploadFile:      CatTool,
		ActionMcpUpdate:           CatMcp,
		ActionAgentCreate:         CatAgent,
		ActionAgentUpdate:         CatAgent,
		ActionAgentUpdateBindings: CatAgent,
	}
	for action, want := range cases {
		require.Equal(t, want, categoryOf(action), "action=%s", action)
	}
}

func TestLoginDetailReasonConstants(t *testing.T) {
	require.Equal(t, "invalid_credentials", ReasonInvalidCredentials)
	require.Equal(t, "token_issuance_failed", ReasonTokenIssuanceFailed)
	require.Equal(t, "invalid_request", ReasonInvalidRequest) // casdoor callback 前期失败（PR #150 审查 P2）
	b, _ := json.Marshal(LoginDetail{Reason: ReasonTokenIssuanceFailed})
	require.Contains(t, string(b), `"reason":"token_issuance_failed"`)
	b, _ = json.Marshal(LoginDetail{Reason: ReasonInvalidRequest})
	require.Contains(t, string(b), `"reason":"invalid_request"`)
}
