package services

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"control-panel/internal/domain/agent"
	"control-panel/internal/infrastructure/deployer"
)

// newFailClosedService builds an AgentDeployerService whose deployer client
// points at the given mock server, with a repo that always finds the agent.
func newFailClosedService(deployerURL string) *AgentDeployerService {
	return &AgentDeployerService{
		client: deployer.NewClient(deployerURL, "test-key"),
		agentRepo: &mockAgentRepo{getByNameFunc: func(_, _ string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{Name: "general", TenantID: "t1"}, nil
		}},
	}
}

// 部署 404 → not_found（唯一允许映射为未部署的错误形态）。
func TestGetStatus_Deployer404_MapsToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"agent not found"}`))
	}))
	defer srv.Close()

	dto, err := newFailClosedService(srv.URL).GetStatus("t1", "general")
	require.NoError(t, err)
	require.NotNil(t, dto)
	require.Equal(t, "not_found", dto.Status)
}

// 部署 5xx → 错误传播（fail-closed，不得伪装 not_found）。
func TestGetStatus_Deployer5xx_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
	}))
	defer srv.Close()

	_, err := newFailClosedService(srv.URL).GetStatus("t1", "general")
	require.Error(t, err, "deployer 5xx 必须以 error 返回，不得映射 not_found")
}

// 部署不可达（连接拒绝）→ 错误传播。
func TestGetStatus_DeployerUnreachable_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close() // 立即关闭 → 连接拒绝

	_, err := newFailClosedService(url).GetStatus("t1", "general")
	require.Error(t, err, "deployer 网络故障必须以 error 返回，不得映射 not_found")
}
