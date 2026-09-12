package services

import (
	"testing"
	"time"

	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
)

func TestToolResultSpeedingAssetProposalCommitsOnce(t *testing.T) {
	runs, db := newRunTestService(t)
	_, err := runs.RegisterStateSchema("speeding", RegisterStateSchemaInput{
		Namespace: "com.speeding.world", Name: "balance-sheet", Version: "1.0.0",
		ScopeTypes: []string{"run"}, SubjectTypes: []string{"run"},
		Schema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"liquidAssets": map[string]any{"type": "number", "minimum": 0.0}, "publicTrust": map[string]any{"type": "number", "minimum": 0.0, "maximum": 100.0}},
			"required":   []any{"liquidAssets", "publicTrust"},
		},
	})
	require.NoError(t, err)
	run, err := runs.Create("speeding", CreateRunInput{Name: "asset disposal"})
	require.NoError(t, err)
	state, err := runs.InitializeState("speeding", run.ID, InitializeStateInput{
		Namespace: "com.speeding.world", SchemaName: "balance-sheet", SchemaVersion: "1.0.0",
		SubjectType: "run", SubjectID: run.ID, Data: map[string]any{"liquidAssets": 1000.0, "publicTrust": 60.0},
		IdempotencyKey: "opening-balance",
	})
	require.NoError(t, err)

	availableAt := time.Now().UTC().Add(time.Hour)
	toolResult := rundomain.ToolResult{
		Result:    map[string]any{"decision": "sell"},
		Cost:      rundomain.ToolCost{InputTokens: 120, OutputTokens: 35, ToolCalls: 1},
		LatencyMS: 240, AvailableAt: &availableAt,
		StateProposals: []rundomain.StateProposal{{
			StateID: state.ID, ExpectedRevision: 1, Reason: "settled by Speeding rules",
			Patch: []rundomain.PatchOperation{{Op: "increment", Path: "/liquidAssets", Value: -250.0}, {Op: "increment", Path: "/publicTrust", Value: -5.0}},
		}},
		EventProposals: []rundomain.EventProposal{{Type: "com.speeding.world.asset-disposed.v1", SubjectType: "run", SubjectID: run.ID, DelayMS: 3600000}},
	}
	service := NewToolResultService(db)
	preview, err := service.Validate("speeding", run.ID, toolResult)
	require.NoError(t, err)
	require.Equal(t, 750.0, preview.StateProposals[0].NextData["liquidAssets"])

	input := CommitToolResultInput{ToolName: "speeding.settle_asset", ActorType: "agent", ActorID: "finance", IdempotencyKey: "turn-7:asset-1", Result: toolResult}
	first, err := service.Commit("speeding", run.ID, input)
	require.NoError(t, err)
	require.Len(t, first.CommittedChanges, 1)
	replayed, err := service.Commit("speeding", run.ID, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)
	states, err := runs.States("speeding", run.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(2), states[0].Revision)
	require.Equal(t, 750.0, states[0].Data["liquidAssets"])
	var count int64
	require.NoError(t, db.Model(&rundomain.RunStateChange{}).Where("tenant_id=? AND run_id=?", "speeding", run.ID).Count(&count).Error)
	require.Equal(t, int64(2), count) // initialization + exactly one settlement
	var eventCount int64
	require.NoError(t, db.Table("events").Where("tenant_id=? AND run_id=?", "speeding", run.ID).Count(&eventCount).Error)
	require.Equal(t, int64(2), eventCount) // state.changed + delayed domain event
}

func TestToolResultResearchApprovalAndInvalidProposalRollback(t *testing.T) {
	runs, db := newRunTestService(t)
	_, err := runs.RegisterStateSchema("research", RegisterStateSchemaInput{
		Namespace: "io.example.research", Name: "report", Version: "1.0.0",
		ScopeTypes: []string{"run"}, SubjectTypes: []string{"agent"},
		Schema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"status": map[string]any{"type": "string", "enum": []any{"review", "approved"}}, "approvalCount": map[string]any{"type": "number", "minimum": 0.0}},
			"required":   []any{"status", "approvalCount"},
		},
	})
	require.NoError(t, err)
	run, _ := runs.Create("research", CreateRunInput{Name: "report approval"})
	state, err := runs.InitializeState("research", run.ID, InitializeStateInput{
		Namespace: "io.example.research", SchemaName: "report", SchemaVersion: "1.0.0", SubjectType: "agent", SubjectID: "analyst",
		Data: map[string]any{"status": "review", "approvalCount": 0.0}, IdempotencyKey: "report-created",
	})
	require.NoError(t, err)
	service := NewToolResultService(db)
	approved := rundomain.ToolResult{StateProposals: []rundomain.StateProposal{{
		StateID: state.ID, ExpectedRevision: 1, Reason: "reviewer approved report",
		Patch: []rundomain.PatchOperation{{Op: "replace", Path: "/status", Value: "approved"}, {Op: "increment", Path: "/approvalCount", Value: 1.0}},
	}}}
	_, err = service.Commit("research", run.ID, CommitToolResultInput{ToolName: "review.approve", IdempotencyKey: "approval:report-1", Result: approved})
	require.NoError(t, err)

	invalid := rundomain.ToolResult{StateProposals: []rundomain.StateProposal{{
		StateID: state.ID, ExpectedRevision: 2,
		Patch: []rundomain.PatchOperation{{Op: "replace", Path: "/status", Value: "secretly-overwritten"}},
	}}}
	_, err = service.Commit("research", run.ID, CommitToolResultInput{ToolName: "llm.direct_write", IdempotencyKey: "invalid-write", Result: invalid})
	require.ErrorContains(t, err, "state validation failed")
	states, err := runs.States("research", run.ID)
	require.NoError(t, err)
	require.Equal(t, "approved", states[0].Data["status"])
	require.Equal(t, uint64(2), states[0].Revision)
	var records int64
	require.NoError(t, db.Model(&rundomain.ToolResultRecord{}).Where("tenant_id=? AND run_id=?", "research", run.ID).Count(&records).Error)
	require.Equal(t, int64(1), records)
}

func TestRestrictedPatchRejectsWholeStateAndExpressions(t *testing.T) {
	_, err := applyRestrictedPatch(map[string]any{"value": 1.0}, []rundomain.PatchOperation{{Op: "replace", Path: "", Value: map[string]any{"value": 2.0}}})
	require.ErrorContains(t, err, "non-root")
	_, err = applyRestrictedPatch(map[string]any{"value": 1.0}, []rundomain.PatchOperation{{Op: "script", Path: "/value", Value: "value + secret"}})
	require.ErrorContains(t, err, "not allowed")
}

func TestToolResultRejectionIsAuditedAndIdempotent(t *testing.T) {
	runs, db := newRunTestService(t)
	run, err := runs.Create("tenant", CreateRunInput{Name: "approval"})
	require.NoError(t, err)
	service := NewToolResultService(db)
	input := CommitToolResultInput{ToolName: "external.publish", IdempotencyKey: "publish-1", Result: rundomain.ToolResult{Result: map[string]any{"draft": "report"}}}
	first, err := service.Reject("tenant", run.ID, input, "human approval denied")
	require.NoError(t, err)
	require.Equal(t, ToolResultRejected, first.Status)
	second, err := service.Reject("tenant", run.ID, input, "changed reason must not rewrite audit")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "human approval denied", second.DecisionReason)
}
