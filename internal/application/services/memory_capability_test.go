package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryCapabilitySignVerifyRoundTrip(t *testing.T) {
	svc := NewMemoryCapabilityService("unit-secret")
	payload := MemoryCapabilityPayload{
		TenantID: "t1", AgentName: "counselor", ScopeMode: "agent",
		Tools:            []string{"memory_remember", "memory_recall", "memory_list", "memory_update"},
		TokenFingerprint: "fp-1",
	}
	raw, err := svc.Sign(payload)
	require.NoError(t, err)
	got, err := svc.Verify(raw, "counselor", "fp-1")
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestMemoryCapabilityVerifyRejectsMismatches(t *testing.T) {
	svc := NewMemoryCapabilityService("unit-secret")
	raw, err := svc.Sign(MemoryCapabilityPayload{TenantID: "t1", AgentName: "a", ScopeMode: "agent", TokenFingerprint: "fp"})
	require.NoError(t, err)
	_, err = svc.Verify(raw, "other-agent", "fp")
	require.ErrorIs(t, err, ErrMemoryCapabilityDenied)
	_, err = svc.Verify(raw, "a", "other-fp")
	require.ErrorIs(t, err, ErrMemoryCapabilityDenied)
	_, err = svc.Verify("v1.tampered.sig", "a", "fp")
	require.ErrorIs(t, err, ErrMemoryCapabilityDenied)
	// 空 secret 一律拒绝（fail closed：HKDF(empty) 可公开计算）
	empty := NewMemoryCapabilityService("")
	_, err = empty.Verify(raw, "a", "fp")
	require.ErrorIs(t, err, ErrMemoryCapabilityDenied)
}
