package services

import (
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createScenarioAgent(t *testing.T, db *gorm.DB, name, tenantID string) agent.AgentConfig {
	t.Helper()
	agentConfig := agent.AgentConfig{Name: name, TenantID: tenantID}
	require.NoError(t, db.Create(&agentConfig).Error)
	return agentConfig
}

func createScenarioRelation(
	t *testing.T,
	service *AgentRelationService,
	tenantID string,
	sourceID, targetID uint64,
	scope, relationType, stance string,
	actions []string,
	contextPolicy, deliveryPolicy string,
	bidirectional bool,
) []*AgentRelationDTO {
	t.Helper()
	relations, err := service.Create(tenantID, &CreateAgentRelationInput{
		SourceAgentID:  sourceID,
		TargetAgentID:  targetID,
		Scope:          scope,
		RelationType:   relationType,
		Stance:         stance,
		AllowedActions: actions,
		ContextPolicy:  contextPolicy,
		DeliveryPolicy: deliveryPolicy,
		Enabled:        true,
		Bidirectional:  bidirectional,
	})
	require.NoError(t, err)
	return relations
}

// TestAgentRelationServiceSupportsMixedOrganizationGraph models the key
// SPEEDING cases in one graph. It proves that an agent can simultaneously be
// someone's ally, another agent's leader, a peer in a workflow, and an
// opponent elsewhere without those edges overwriting one another.
func TestAgentRelationServiceSupportsMixedOrganizationGraph(t *testing.T) {
	service, db, chief, legal := setupAgentRelationService(t)
	finance := createScenarioAgent(t, db, "finance-director", "org-a")
	communications := createScenarioAgent(t, db, "communications-director", "org-a")
	auditor := createScenarioAgent(t, db, "red-team-auditor", "org-a")
	rival := createScenarioAgent(t, db, "hostile-board-member", "org-a")

	// Asymmetric leadership: the subordinate reports upward, while the leader
	// gets a separate edge for assignments and review.
	createScenarioRelation(t, service, "org-a", legal.ID, chief.ID, "speeding-hq", "reports_to", "allied",
		[]string{"report", "escalate"}, "summary_only", "async", false)
	createScenarioRelation(t, service, "org-a", chief.ID, legal.ID, "speeding-hq", "oversight", "friendly",
		[]string{"assign", "review"}, "summary_only", "sync", false)

	// Symmetric peers can consult and hand work to each other.
	createScenarioRelation(t, service, "org-a", legal.ID, finance.ID, "speeding-hq", "peer", "friendly",
		[]string{"inform", "consult", "handoff"}, "shared_thread", "async", true)

	// Advisors and auditors remain separate roles even when they point at the
	// same leader.
	createScenarioRelation(t, service, "org-a", communications.ID, chief.ID, "speeding-hq", "advisor", "friendly",
		[]string{"inform", "consult"}, "summary_only", "async", false)
	createScenarioRelation(t, service, "org-a", auditor.ID, chief.ID, "speeding-hq", "reviewer", "wary",
		[]string{"review", "challenge", "escalate"}, "shared_thread", "sync", false)

	// An opponent is explicitly two-way, without affecting the chief's friendly
	// relationship with legal counsel.
	createScenarioRelation(t, service, "org-a", chief.ID, rival.ID, "speeding-hq", "opponent", "hostile",
		[]string{"challenge"}, "none", "async", true)

	relations, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, relations, 8)

	type edgeKey struct{ source, target uint64 }
	edges := make(map[edgeKey]*AgentRelationDTO, len(relations))
	for _, relation := range relations {
		edges[edgeKey{relation.SourceAgentID, relation.TargetAgentID}] = relation
	}

	require.Equal(t, "reports_to", edges[edgeKey{legal.ID, chief.ID}].RelationType)
	require.Equal(t, []string{"report", "escalate"}, edges[edgeKey{legal.ID, chief.ID}].AllowedActions)
	require.Equal(t, "oversight", edges[edgeKey{chief.ID, legal.ID}].RelationType)
	require.Equal(t, "friendly", edges[edgeKey{chief.ID, legal.ID}].Stance)
	require.Equal(t, "peer", edges[edgeKey{legal.ID, finance.ID}].RelationType)
	require.Equal(t, "peer", edges[edgeKey{finance.ID, legal.ID}].RelationType)
	require.Equal(t, "shared_thread", edges[edgeKey{finance.ID, legal.ID}].ContextPolicy)
	require.Equal(t, "advisor", edges[edgeKey{communications.ID, chief.ID}].RelationType)
	require.Equal(t, "reviewer", edges[edgeKey{auditor.ID, chief.ID}].RelationType)
	require.Equal(t, "hostile", edges[edgeKey{chief.ID, rival.ID}].Stance)
	require.Equal(t, "hostile", edges[edgeKey{rival.ID, chief.ID}].Stance)
}

func TestAgentRelationServiceSamePairCanDifferByScope(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)

	hq := createScenarioRelation(t, service, "org-a", source.ID, target.ID, "speeding-hq", "peer", "friendly",
		[]string{"inform", "consult"}, "summary_only", "async", false)
	crisis := createScenarioRelation(t, service, "org-a", source.ID, target.ID, "crisis-room", "reviewer", "wary",
		[]string{"submit", "review", "challenge"}, "shared_thread", "sync", false)

	require.NotEqual(t, hq[0].ID, crisis[0].ID)
	relations, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, relations, 2)
	require.Equal(t, "crisis-room", relations[0].Scope)
	require.Equal(t, "reviewer", relations[0].RelationType)
	require.Equal(t, "speeding-hq", relations[1].Scope)
	require.Equal(t, "peer", relations[1].RelationType)
}

func TestAgentRelationServiceDeletingOneDirectionKeepsTheOther(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	created := createScenarioRelation(t, service, "org-a", source.ID, target.ID, "speeding-hq", "peer", "friendly",
		[]string{"inform", "consult"}, "shared_thread", "async", true)

	require.NoError(t, service.Delete("org-a", created[0].ID))
	relations, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, target.ID, relations[0].SourceAgentID)
	require.Equal(t, source.ID, relations[0].TargetAgentID)
}

func TestAgentRelationServiceBidirectionalCreateIsAtomicOnConflict(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)

	// B -> A already exists. Asking for A <-> B must not leave a half-created
	// A -> B edge behind when the reverse direction conflicts.
	createScenarioRelation(t, service, "org-a", target.ID, source.ID, "speeding-hq", "peer", "friendly",
		[]string{"inform"}, "summary_only", "async", false)
	input := validAgentRelationInput(source.ID, target.ID)
	input.Bidirectional = true
	_, err := service.Create("org-a", input)
	require.ErrorIs(t, err, agentrelation.ErrAlreadyExists)

	relations, listErr := service.List("org-a")
	require.NoError(t, listErr)
	require.Len(t, relations, 1)
	require.Equal(t, target.ID, relations[0].SourceAgentID)
	require.Equal(t, source.ID, relations[0].TargetAgentID)
}

func TestAgentRelationServiceTenantCannotMutateAnotherTenantEdge(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	created := createScenarioRelation(t, service, "org-a", source.ID, target.ID, "speeding-hq", "peer", "friendly",
		[]string{"inform"}, "summary_only", "async", false)

	hostile := "hostile"
	_, err := service.Update("org-b", created[0].ID, &UpdateAgentRelationInput{Stance: &hostile})
	require.ErrorIs(t, err, agentrelation.ErrNotFound)
	require.ErrorIs(t, service.Delete("org-b", created[0].ID), agentrelation.ErrNotFound)

	foreignList, err := service.List("org-b")
	require.NoError(t, err)
	require.Empty(t, foreignList)
	ownerList, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, ownerList, 1)
	require.Equal(t, "friendly", ownerList[0].Stance)
}

func TestAgentRelationServiceRejectsInvalidRelationshipContracts(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)

	tests := []struct {
		name    string
		mutate  func(*CreateAgentRelationInput)
		wantErr error
	}{
		{name: "invalid scope", mutate: func(input *CreateAgentRelationInput) { input.Scope = "bad scope" }, wantErr: agentrelation.ErrInvalidScope},
		{name: "unknown type", mutate: func(input *CreateAgentRelationInput) { input.RelationType = "best_friend" }, wantErr: agentrelation.ErrInvalidType},
		{name: "unknown stance", mutate: func(input *CreateAgentRelationInput) { input.Stance = "treacherous" }, wantErr: agentrelation.ErrInvalidStance},
		{name: "actions required", mutate: func(input *CreateAgentRelationInput) { input.AllowedActions = nil }, wantErr: agentrelation.ErrActionsRequired},
		{name: "unknown action", mutate: func(input *CreateAgentRelationInput) { input.AllowedActions = []string{"bribe"} }, wantErr: agentrelation.ErrInvalidAction},
		{name: "unknown context policy", mutate: func(input *CreateAgentRelationInput) { input.ContextPolicy = "all_memory" }, wantErr: agentrelation.ErrInvalidContext},
		{name: "unknown delivery policy", mutate: func(input *CreateAgentRelationInput) { input.DeliveryPolicy = "whenever" }, wantErr: agentrelation.ErrInvalidDelivery},
		{name: "constraint too long", mutate: func(input *CreateAgentRelationInput) { input.Constraint = strings.Repeat("界", 2001) }, wantErr: agentrelation.ErrConstraintTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validAgentRelationInput(source.ID, target.ID)
			tt.mutate(input)
			_, err := service.Create("org-a", input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	relations, err := service.List("org-a")
	require.NoError(t, err)
	require.Empty(t, relations)
}
