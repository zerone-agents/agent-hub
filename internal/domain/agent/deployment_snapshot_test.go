package agent

import (
	"testing"
	"time"
)

func TestDeploymentSnapshotTableName(t *testing.T) {
	// 表名由 GORM 约定生成（snapshots → agent_deployment_snapshots），
	// 通过 TableName 显式固定，防止未来重命名产生漂移。
	s := DeploymentSnapshot{AgentID: 1}
	if got := s.TableName(); got != "agent_deployment_snapshots" {
		t.Fatalf("TableName() = %q, want agent_deployment_snapshots", got)
	}
}

func TestDeploymentSnapshotMaps(t *testing.T) {
	s := DeploymentSnapshot{
		TenantID:    "acme",
		ToolHashes:  map[string]string{"calc": "abc123"},
		SkillHashes: map[string]string{"qa": "def456"},
		DeployedAt:  time.Now(),
	}
	if s.ToolHashes["calc"] != "abc123" || s.SkillHashes["qa"] != "def456" {
		t.Fatal("hash maps must round-trip through the struct")
	}
}
