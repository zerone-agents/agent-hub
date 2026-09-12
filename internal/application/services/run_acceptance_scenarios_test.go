package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	"github.com/stretchr/testify/require"
)

// TestH1SpeedingDualRunIsolation is deliberately a consumer fixture: all
// domain vocabulary lives in the registered schema/data, never in Run Core.
func TestH1SpeedingDualRunIsolation(t *testing.T) {
	s, db := newRunTestService(t)
	finance := agent.AgentConfig{TenantID: "speeding", Name: "finance-director", ContentHash: "finance-v1", SystemPrompt: "finance", PersonalityTemplateName: "cautious-cfo", PersonalityTemplateVersion: 3, PersonalityPrompt: "Protect liquidity and report material risk."}
	require.NoError(t, db.Create(&finance).Error)
	originalPrompt := finance.PersonalityPrompt

	_, err := s.RegisterStateSchema("speeding", RegisterStateSchemaInput{
		Namespace: "com.speeding.world", Name: "finance-situation", Version: "1.0.0",
		ScopeTypes: []string{"run"}, SubjectTypes: []string{"agent"},
		Schema: objectNumberSchema("stress", "trust", "wealth"),
	})
	require.NoError(t, err)

	crisis, err := s.Create("speeding", CreateRunInput{Name: "Crisis route"})
	require.NoError(t, err)
	turnaround, err := s.Create("speeding", CreateRunInput{Name: "Turnaround route"})
	require.NoError(t, err)
	_, err = s.AddAgent("speeding", crisis.ID, finance.ID, "finance")
	require.NoError(t, err)
	_, err = s.AddAgent("speeding", turnaround.ID, finance.ID, "finance")
	require.NoError(t, err)

	crisisState, err := s.InitializeState("speeding", crisis.ID, InitializeStateInput{Namespace: "com.speeding.world", SchemaName: "finance-situation", SchemaVersion: "1.0.0", SubjectType: "agent", SubjectID: "finance-director", Data: map[string]any{"stress": 90.0, "trust": 20.0, "wealth": 40.0}, IdempotencyKey: "crisis-initial"})
	require.NoError(t, err)
	_, err = s.InitializeState("speeding", turnaround.ID, InitializeStateInput{Namespace: "com.speeding.world", SchemaName: "finance-situation", SchemaVersion: "1.0.0", SubjectType: "agent", SubjectID: "finance-director", Data: map[string]any{"stress": 25.0, "trust": 75.0, "wealth": 85.0}, IdempotencyKey: "turnaround-initial"})
	require.NoError(t, err)
	_, err = s.CommitState("speeding", crisis.ID, crisisState.ID, CommitStateInput{ExpectedRevision: 1, Data: map[string]any{"stress": 98.0, "trust": 10.0, "wealth": 30.0}, IdempotencyKey: "crisis-next"})
	require.NoError(t, err)

	otherStates, err := s.States("speeding", turnaround.ID)
	require.NoError(t, err)
	require.Equal(t, float64(25), otherStates[0].Data["stress"])
	require.Equal(t, float64(75), otherStates[0].Data["trust"])
	require.Equal(t, float64(85), otherStates[0].Data["wealth"])
	var unchanged agent.AgentConfig
	require.NoError(t, db.Where("tenant_id=? AND id=?", "speeding", finance.ID).First(&unchanged).Error)
	require.Equal(t, originalPrompt, unchanged.PersonalityPrompt)
	require.Equal(t, "cautious-cfo", unchanged.PersonalityTemplateName)
	require.Equal(t, 3, unchanged.PersonalityTemplateVersion)
}

func TestH1ResearchDualRunIsolationUsesSameGenericStateService(t *testing.T) {
	s, db := newRunTestService(t)
	analyst := agent.AgentConfig{TenantID: "research", Name: "analyst", ContentHash: "analyst-v1", SystemPrompt: "analyse", PersonalityTemplateName: "skeptical-reviewer", PersonalityTemplateVersion: 1, PersonalityPrompt: "Separate evidence from inference."}
	require.NoError(t, db.Create(&analyst).Error)
	_, err := s.RegisterStateSchema("research", RegisterStateSchemaInput{
		Namespace: "io.example.research", Name: "assignment-state", Version: "1.0.0",
		ScopeTypes: []string{"run"}, SubjectTypes: []string{"agent"},
		Schema: objectNumberSchema("progress", "confidence"),
	})
	require.NoError(t, err)

	market, err := s.Create("research", CreateRunInput{Name: "Market research"})
	require.NoError(t, err)
	risk, err := s.Create("research", CreateRunInput{Name: "Risk review"})
	require.NoError(t, err)
	_, err = s.AddAgent("research", market.ID, analyst.ID, "analyst")
	require.NoError(t, err)
	_, err = s.AddAgent("research", risk.ID, analyst.ID, "analyst")
	require.NoError(t, err)
	marketState, err := s.InitializeState("research", market.ID, InitializeStateInput{Namespace: "io.example.research", SchemaName: "assignment-state", SchemaVersion: "1.0.0", SubjectType: "agent", SubjectID: "analyst", Data: map[string]any{"progress": 20.0, "confidence": 45.0}, IdempotencyKey: "market-initial"})
	require.NoError(t, err)
	_, err = s.InitializeState("research", risk.ID, InitializeStateInput{Namespace: "io.example.research", SchemaName: "assignment-state", SchemaVersion: "1.0.0", SubjectType: "agent", SubjectID: "analyst", Data: map[string]any{"progress": 70.0, "confidence": 80.0}, IdempotencyKey: "risk-initial"})
	require.NoError(t, err)
	_, err = s.CommitState("research", market.ID, marketState.ID, CommitStateInput{ExpectedRevision: 1, Data: map[string]any{"progress": 55.0, "confidence": 60.0}, IdempotencyKey: "market-next"})
	require.NoError(t, err)
	riskStates, err := s.States("research", risk.ID)
	require.NoError(t, err)
	require.Equal(t, float64(70), riskStates[0].Data["progress"])
	require.Equal(t, float64(80), riskStates[0].Data["confidence"])
}

func objectNumberSchema(fields ...string) map[string]any {
	properties := make(map[string]any, len(fields))
	required := make([]any, 0, len(fields))
	for _, field := range fields {
		properties[field] = map[string]any{"type": "number", "minimum": 0.0, "maximum": 100.0}
		required = append(required, field)
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}
