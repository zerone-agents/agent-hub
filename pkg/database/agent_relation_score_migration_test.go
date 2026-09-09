package database

import (
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBackfillAgentRelationScoresOnlyInitializesLegacyRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}))

	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })

	source := agent.AgentConfig{Name: "agent-a", TenantID: "org-a"}
	target := agent.AgentConfig{Name: "agent-b", TenantID: "org-a"}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&target).Error)

	legacy := agentrelation.AgentRelation{
		TenantID: "org-a", Scope: "legacy", SourceAgentID: source.ID, TargetAgentID: target.ID,
		RelationType: "peer", Stance: "hostile", RelationshipScore: 0, Enabled: true,
	}
	changedAt := time.Now().UTC().Add(-time.Hour)
	evolved := agentrelation.AgentRelation{
		TenantID: "org-a", Scope: "evolved", SourceAgentID: source.ID, TargetAgentID: target.ID,
		RelationType: "peer", Stance: "neutral", RelationshipScore: 17, LastChangedAt: &changedAt, Enabled: true,
	}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Create(&evolved).Error)

	require.NoError(t, backfillAgentRelationScores())
	require.NoError(t, backfillAgentRelationScores(), "migration must be idempotent")

	var gotLegacy, gotEvolved agentrelation.AgentRelation
	require.NoError(t, db.First(&gotLegacy, legacy.ID).Error)
	require.NoError(t, db.First(&gotEvolved, evolved.ID).Error)
	require.Equal(t, -80, gotLegacy.RelationshipScore)
	require.NotNil(t, gotLegacy.LastChangedAt)
	require.Equal(t, 17, gotEvolved.RelationshipScore)
	require.WithinDuration(t, changedAt, *gotEvolved.LastChangedAt, time.Second)
}
