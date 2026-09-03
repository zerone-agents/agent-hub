package deployer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateAgentBody_OmitsAigcWhenNil(t *testing.T) {
	body, err := json.Marshal(createAgentBody{
		CreateAgentRequest: &CreateAgentRequest{
			RootAgentID: "a",
			Agents:      []AgentDefinition{{Name: "a", Model: "glm-4.5"}},
			Provider:    ProviderConfig{Protocol: "anthropic-messages", BaseURL: "https://x", LockedAPIKey: "k"},
		},
	})
	require.NoError(t, err)
	require.NotContains(t, string(body), "aigc")
}

func TestCreateAgentBody_IncludesAigc(t *testing.T) {
	body, err := json.Marshal(createAgentBody{
		CreateAgentRequest: &CreateAgentRequest{
			RootAgentID: "a",
			Agents:      []AgentDefinition{{Name: "a", Model: "glm-4.5"}},
			Provider:    ProviderConfig{Protocol: "anthropic-messages", BaseURL: "https://x", LockedAPIKey: "k"},
			Aigc: &AigcConfig{
				Enabled:         true,
				ContentProducer: "001191320118MAK93FC72D10000",
				SigningKey:      "secret",
				ModelCodes:      map[string]string{"glm-4.5": "0001"},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, string(body), `"aigc"`)
	require.Contains(t, string(body), `"contentProducer":"001191320118MAK93FC72D10000"`)
	require.Contains(t, string(body), `"glm-4.5":"0001"`)
}
