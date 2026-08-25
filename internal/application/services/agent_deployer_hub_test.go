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

	s.applyHub(req)

	require.NotNil(t, req.Hub)
	require.True(t, req.Hub.Enabled)
	require.Equal(t, "https://hub.example.com", req.Hub.BaseURL)
	require.Equal(t, "k-123", req.Hub.ChatPushKey)
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

			s.applyHub(req)

			require.Nil(t, req.Hub)
		})
	}
}

// JSON 序列化保真：字段名与 deployer HubConfig schema（camelCase）一致，
// omitempty 语义下 disabled 时整个段省略。
func TestApplyHub_JSONShape(t *testing.T) {
	s := &AgentDeployerService{chatPushAPIKey: "k-123", chatPushPublicURL: "https://hub.example.com"}
	req := &deployer.CreateAgentRequest{}
	s.applyHub(req)

	b, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(b), `"hub":{"enabled":true,"baseUrl":"https://hub.example.com","chatPushKey":"k-123"}`)

	req2 := &deployer.CreateAgentRequest{}
	s2 := &AgentDeployerService{}
	s2.applyHub(req2)
	b2, err := json.Marshal(req2)
	require.NoError(t, err)
	require.NotContains(t, string(b2), `"hub"`)
}
