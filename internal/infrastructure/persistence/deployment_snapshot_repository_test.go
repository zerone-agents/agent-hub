package repository

import (
	"context"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newSnapshotDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&agent.DeploymentSnapshot{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSnapshotUpsertAndGet(t *testing.T) {
	db := newSnapshotDB(t)
	ctx := context.Background()
	repo := NewDeploymentSnapshotRepositoryWithDB(db)

	want := &agent.DeploymentSnapshot{
		AgentID:     7,
		TenantID:    "acme",
		DeployedAt:  time.Now(),
		ToolHashes:  map[string]string{"calc": "abc123"},
		SkillHashes: map[string]string{},
	}
	if err := repo.Upsert(ctx, want); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// 同 agent 再次 upsert 覆盖旧哈希
	if err := repo.Upsert(ctx, &agent.DeploymentSnapshot{
		AgentID: 7, TenantID: "acme", DeployedAt: time.Now(),
		ToolHashes: map[string]string{"calc": "new456"},
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.GetByAgent(ctx, "acme", 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ToolHashes["calc"] != "new456" {
		t.Fatalf("hash = %q, want new456 (upsert must overwrite)", got.ToolHashes["calc"])
	}
}

func TestSnapshotGetByAgent_OtherTenantNotFound(t *testing.T) {
	db := newSnapshotDB(t)
	repo := NewDeploymentSnapshotRepositoryWithDB(db)
	ctx := context.Background()
	if err := repo.Upsert(ctx, &agent.DeploymentSnapshot{
		AgentID: 7, TenantID: "acme", ToolHashes: map[string]string{},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := repo.GetByAgent(ctx, "other", 7); err != gorm.ErrRecordNotFound {
		t.Fatalf("cross-tenant get err = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestSnapshotGetByAgent_MissingRow(t *testing.T) {
	db := newSnapshotDB(t)
	repo := NewDeploymentSnapshotRepositoryWithDB(db)
	if _, err := repo.GetByAgent(context.Background(), "acme", 999); err != gorm.ErrRecordNotFound {
		t.Fatalf("missing row err = %v, want gorm.ErrRecordNotFound", err)
	}
}
