package handler

import (
	"testing"

	"control-panel/internal/domain/agent"
)

// TestErrcodes_RegistryPinsDomainAgentCodes pins the #201 P5 dual-write
// contract: every agent-domain stable code constant equals its errcodes.go
// registry literal, so neither side can drift silently. The handler-side
// constants are otherwise unreferenced (dispatch reads ve.StableCode()),
// and handler wire tests only pin the domain side — this test is the
// compile-adjacent binding between the two registries (PR #204 评审建议).
func TestErrcodes_RegistryPinsDomainAgentCodes(t *testing.T) {
	pairs := []struct {
		registry string
		domain   agent.StableCode
	}{
		{ErrCodeAgentNameRequired, agent.CodeAgentNameRequired},
		{ErrCodeAgentNameTooLong, agent.CodeAgentNameTooLong},
		{ErrCodeAgentNameInvalid, agent.CodeAgentNameInvalid},
		{ErrCodeAgentNameExists, agent.CodeAgentNameExists},
		{ErrCodeAgentNotFound, agent.CodeAgentNotFound},
		{ErrCodeSubagentNotFound, agent.CodeSubagentNotFound},
		{ErrCodeSubagentSelfReference, agent.CodeSubagentSelfReference},
		{ErrCodeSystemPromptRequired, agent.CodeSystemPromptRequired},
		{ErrCodeConfigKeyRenamed, agent.CodeConfigKeyRenamed},
		// PR #204 评审补充：400 路径引用拆键 + 表单高频码
		{ErrCodeAgentReferenceNotFound, agent.CodeAgentReferenceNotFound},
		{ErrCodeInvalidPermissionMode, agent.CodeInvalidPermissionMode},
		{ErrCodeMaxTurnsNegative, agent.CodeMaxTurnsNegative},
		{ErrCodeMaxTurnsTooLarge, agent.CodeMaxTurnsTooLarge},
		{ErrCodeProviderIdNotFound, agent.CodeProviderIdNotFound},
		{ErrCodeModelSelectionNotFound, agent.CodeModelSelectionNotFound},
		{ErrCodeModelNotFound, agent.CodeModelNotFound},
		{ErrCodeModelTypeMismatch, agent.CodeModelTypeMismatch},
	}
	for _, p := range pairs {
		if string(p.domain) != p.registry {
			t.Errorf("registry %q != domain %q：双写契约漂移", p.registry, string(p.domain))
		}
	}
}
