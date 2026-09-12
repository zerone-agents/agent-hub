package services

import (
	"errors"
	"fmt"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentRelationService(t *testing.T) (*AgentRelationService, *gorm.DB, agent.AgentConfig, agent.AgentConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}, &agentrelation.AgentRelationEvent{}))

	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })

	source := agent.AgentConfig{Name: "chief-of-staff", TenantID: "org-a"}
	target := agent.AgentConfig{Name: "legal-counsel", TenantID: "org-a"}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&target).Error)
	return NewAgentRelationService(), db, source, target
}

func validAgentRelationInput(sourceID, targetID uint64) *CreateAgentRelationInput {
	return &CreateAgentRelationInput{
		SourceAgentID:  sourceID,
		TargetAgentID:  targetID,
		Scope:          "speeding-hq",
		RelationType:   "peer",
		Stance:         "friendly",
		AllowedActions: []string{"inform", "consult", "inform"},
		ContextPolicy:  "summary_only",
		DeliveryPolicy: "async",
		Constraint:     "公开声明前先复核",
		Enabled:        true,
	}
}

func TestAgentRelationServiceCreateBidirectional(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	input := validAgentRelationInput(source.ID, target.ID)
	input.Bidirectional = true

	created, err := service.Create("org-a", input)
	require.NoError(t, err)
	require.Len(t, created, 2)
	require.Equal(t, source.Name, created[0].SourceAgentName)
	require.Equal(t, target.Name, created[0].TargetAgentName)
	require.Equal(t, target.Name, created[1].SourceAgentName)
	require.Equal(t, source.Name, created[1].TargetAgentName)
	require.Equal(t, []string{"inform", "consult"}, created[0].AllowedActions)
	require.Equal(t, 0, created[0].RelationshipScore)
	require.Equal(t, "neutral", created[0].Stance)
	require.Nil(t, created[0].LastChangedAt)

	otherTenant, err := service.List("org-b")
	require.NoError(t, err)
	require.Empty(t, otherTenant)
}

func TestAgentRelationServiceLegacyDynamicMutationIsReadOnly(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	created, err := service.Create("org-a", validAgentRelationInput(source.ID, target.ID))
	require.NoError(t, err)
	require.Equal(t, 0, created[0].RelationshipScore)

	result, err := service.RecordEvent("org-a", created[0].ID, &RecordAgentRelationEventInput{
		EventType: "promise_broken", Severity: 1, Reason: "承诺提交合同但没有交付",
		Visibility: "participants", SourceKind: "task", SourceID: "task-7", IdempotencyKey: "task-7-promise",
	}, "admin", "operator")
	require.ErrorIs(t, err, agentrelation.ErrDynamicsReadOnly)
	require.Nil(t, result)

	events, err := service.Events("org-a", created[0].ID, 10)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestAgentRelationServiceRejectsDuplicateAndSelfRelation(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	_, err := service.Create("org-a", validAgentRelationInput(source.ID, target.ID))
	require.NoError(t, err)

	_, err = service.Create("org-a", validAgentRelationInput(source.ID, target.ID))
	require.ErrorIs(t, err, agentrelation.ErrAlreadyExists)

	_, err = service.Create("org-a", validAgentRelationInput(source.ID, source.ID))
	require.ErrorIs(t, err, agentrelation.ErrSelfRelation)
}

func TestAgentRelationServiceRejectsAsymmetricBidirectionalShortcut(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	input := validAgentRelationInput(source.ID, target.ID)
	input.RelationType = "reports_to"
	input.Bidirectional = true

	_, err := service.Create("org-a", input)
	require.ErrorIs(t, err, agentrelation.ErrBidirectionalType)
}

func TestAgentRelationServiceUpdateKeepsDirectionIndependent(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	input := validAgentRelationInput(source.ID, target.ID)
	input.Bidirectional = true
	created, err := service.Create("org-a", input)
	require.NoError(t, err)

	hostile := "hostile"
	actions := []string{"challenge"}
	updated, err := service.Update("org-a", created[0].ID, &UpdateAgentRelationInput{
		Stance:         &hostile,
		AllowedActions: &actions,
	})
	require.NoError(t, err)
	require.Equal(t, "neutral", updated.Stance)
	require.Equal(t, 0, updated.RelationshipScore)
	events, err := service.Events("org-a", created[0].ID, 10)
	require.NoError(t, err)
	require.Empty(t, events)

	relations, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, relations, 2)
	require.Equal(t, "neutral", relations[1].Stance)
}

func TestAgentRelationServiceDeprecatedStanceEditIsIgnored(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	created, err := service.Create("org-a", validAgentRelationInput(source.ID, target.ID))
	require.NoError(t, err)
	friendly := "friendly"
	constraint := "只修改约束"
	updated, err := service.Update("org-a", created[0].ID, &UpdateAgentRelationInput{
		Stance: &friendly, Constraint: &constraint,
	})
	require.NoError(t, err)
	require.Equal(t, 0, updated.RelationshipScore)
	require.Equal(t, "neutral", updated.Stance)
	events, err := service.Events("org-a", created[0].ID, 10)
	require.NoError(t, err)
	require.Empty(t, events, "平台连接编辑不能产生人物关系事件")
}

func TestAgentRelationServiceRequiresTenantOwnedAgents(t *testing.T) {
	service, db, source, _ := setupAgentRelationService(t)
	foreign := agent.AgentConfig{Name: "outsider", TenantID: "org-b"}
	require.NoError(t, db.Create(&foreign).Error)

	_, err := service.Create("org-a", validAgentRelationInput(source.ID, foreign.ID))
	require.ErrorIs(t, err, agentrelation.ErrAgentNotFound)
}

func TestAgentRelationServiceCanCreateDisabledRelation(t *testing.T) {
	service, _, source, target := setupAgentRelationService(t)
	input := validAgentRelationInput(source.ID, target.ID)
	input.Enabled = false

	created, err := service.Create("org-a", input)
	require.NoError(t, err)
	require.Len(t, created, 1)
	require.False(t, created[0].Enabled)
}

func TestAgentRelationServiceDeleteNotFound(t *testing.T) {
	service, _, _, _ := setupAgentRelationService(t)
	err := service.Delete("org-a", 999)
	require.True(t, errors.Is(err, agentrelation.ErrNotFound))
}
