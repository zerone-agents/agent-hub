package agent

import (
	"reflect"
	"strings"
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

func TestDeploymentSnapshotAgentRelation(t *testing.T) {
	// 一级保护：关联字段必须存在且 FK 指向 AgentID（与 AgentSkill/AgentTool
	// 同款模式），防止重构时剥离级联删除关系。AgentConfig 的 GORM 主键是
	// ID，foreignKey:AgentID 即声明本表主键 AgentID 引用 agents 表同一
	// Agent，关联字段与主键同源。
	field, ok := reflect.TypeOf(DeploymentSnapshot{}).FieldByName("Agent")
	if !ok {
		t.Fatal("DeploymentSnapshot.Agent relation field missing, want foreignKey:AgentID + constraint:OnDelete:CASCADE")
	}
	if field.Type != reflect.TypeOf(AgentConfig{}) {
		t.Fatalf("Agent field type = %v, want %v", field.Type, reflect.TypeOf(AgentConfig{}))
	}
	tag := field.Tag.Get("gorm")
	for _, want := range []string{"foreignKey:AgentID", "constraint:OnDelete:CASCADE"} {
		if !strings.Contains(tag, want) {
			t.Fatalf("Agent gorm tag %q missing %q", tag, want)
		}
	}
}
