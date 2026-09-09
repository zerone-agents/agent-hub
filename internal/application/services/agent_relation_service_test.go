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
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}))

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

	otherTenant, err := service.List("org-b")
	require.NoError(t, err)
	require.Empty(t, otherTenant)
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
	require.Equal(t, "hostile", updated.Stance)

	relations, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, relations, 2)
	require.Equal(t, "friendly", relations[1].Stance)
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
