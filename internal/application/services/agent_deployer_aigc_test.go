package services

import (
	"errors"
	"testing"

	"control-panel/internal/infrastructure/deployer"

	"github.com/stretchr/testify/require"
)

type fakeAigcProvider struct {
	cfg *deployer.AigcConfig
	err error
}

func (f fakeAigcProvider) DeployerConfig(string) (*deployer.AigcConfig, error) { return f.cfg, f.err }

func TestApplyAigc_NilServiceLeavesRequestUntouched(t *testing.T) {
	s := &AgentDeployerService{}
	req := &deployer.CreateAgentRequest{}
	require.NoError(t, s.applyAigc(req, "acme"))
	require.Nil(t, req.Aigc)
}

func TestApplyAigc_NotConfiguredLeavesRequestUntouched(t *testing.T) {
	s := &AgentDeployerService{aigcSvc: fakeAigcProvider{}}
	req := &deployer.CreateAgentRequest{}
	require.NoError(t, s.applyAigc(req, "acme"))
	require.Nil(t, req.Aigc)
}

func TestApplyAigc_InjectsConfig(t *testing.T) {
	want := &deployer.AigcConfig{Enabled: true, ContentProducer: "001191320118MAK93FC72D10000"}
	s := &AgentDeployerService{aigcSvc: fakeAigcProvider{cfg: want}}
	req := &deployer.CreateAgentRequest{}
	require.NoError(t, s.applyAigc(req, "acme"))
	require.Same(t, want, req.Aigc)
}

func TestApplyAigc_ErrorPropagates(t *testing.T) {
	s := &AgentDeployerService{aigcSvc: fakeAigcProvider{err: errors.New("decrypt failed")}}
	req := &deployer.CreateAgentRequest{}
	require.Error(t, s.applyAigc(req, "acme"))
	require.Nil(t, req.Aigc)
}

func TestNewAgentDeployerService_NilAigcSvcLeavesFieldNil(t *testing.T) {
	s := NewAgentDeployerService(nil, "", "", "", "", "", nil, nil, nil, "", "", false)
	require.Nil(t, s.aigcSvc)

	req := &deployer.CreateAgentRequest{}
	require.NoError(t, s.applyAigc(req, "acme"))
	require.Nil(t, req.Aigc)
}
