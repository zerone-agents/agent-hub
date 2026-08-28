package services

import (
	"encoding/json"
	"testing"

	"control-panel/internal/infrastructure/deployer"

	"github.com/stretchr/testify/require"
)

func TestApplyHub_InjectsWhenConfigured(t *testing.T) {
	s := &AgentDeployerService{chatPushAPIKey: "k-123", chatPushPublicURL: "https://hub.example.com"}
	req := &deployer.CreateAgentRequest{}

	s.applyHub(req, "acme")

	require.NotNil(t, req.Hub)
	require.True(t, req.Hub.Enabled)
	require.Equal(t, "https://hub.example.com", req.Hub.BaseURL)
	require.Equal(t, "k-123", req.Hub.ChatPushKey)
	// 部署租户作为可信 org 下发（issue #78）
	require.Equal(t, "acme", req.Hub.Org)
}

// 部署租户透传（issue #78）：org 来自部署上下文而非调用方声明。单测只覆盖
// tenantID → HubConfig.Org 的复制；跨边界集成（同名 agent 双租户部署、
// runtime 回传落点）由 runtime/deployer 侧集成验证。
func TestApplyHub_PreservesDeploymentTenant(t *testing.T) {
	s := &AgentDeployerService{chatPushAPIKey: "k-123", chatPushPublicURL: "https://hub.example.com"}

	reqA := &deployer.CreateAgentRequest{}
	s.applyHub(reqA, "acme")
	reqB := &deployer.CreateAgentRequest{}
	s.applyHub(reqB, "globex")

	require.Equal(t, "acme", reqA.Hub.Org)
	require.Equal(t, "globex", reqB.Hub.Org)
}

func TestApplyHub_OmittedWhenNotConfigured(t *testing.T) {
	cases := []struct {
		name, key, url string
	}{
		{"both empty", "", ""},
		{"key only", "k-123", ""},
		{"url only", "", "https://hub.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &AgentDeployerService{chatPushAPIKey: tc.key, chatPushPublicURL: tc.url}
			req := &deployer.CreateAgentRequest{}

			s.applyHub(req, "acme")

			require.Nil(t, req.Hub)
		})
	}
}

// JSON 序列化保真：字段名与 deployer HubConfig schema（camelCase）一致，
// omitempty 语义下 disabled 时整个段省略。
func TestApplyHub_JSONShape(t *testing.T) {
	s := &AgentDeployerService{chatPushAPIKey: "k-123", chatPushPublicURL: "https://hub.example.com"}
	req := &deployer.CreateAgentRequest{}
	s.applyHub(req, "acme")

	b, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(b), `"hub":{"enabled":true,"baseUrl":"https://hub.example.com","chatPushKey":"k-123","org":"acme"}`)

	req2 := &deployer.CreateAgentRequest{}
	s2 := &AgentDeployerService{}
	s2.applyHub(req2, "acme")
	b2, err := json.Marshal(req2)
	require.NoError(t, err)
	require.NotContains(t, string(b2), `"hub"`)
}
