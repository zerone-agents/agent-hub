package repository

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/skill"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openFKExtendedDB 起 sqlite 内存库并开启外键强制（_pragma=foreign_keys(1)）。
// 仓库其余测试基建未开启 FK 强制；本组 RESTRICT 后盾测试需要真实约束，
// 因此用独立 DSN，不碰共享 helper（避免级联相关既有测试行为变化）。
func openFKExtendedDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(models...))
	return db
}

// 并发后盾回归（#123 review P1）：绑定行已存在的资源删除必须被
// ON DELETE RESTRICT 拒绝——模拟「守卫检查通过后被并发写入绑定」的
// 最坏情况（直接绕过守卫删除），DB 层仍失败且资源与绑定行原封不动，
// 绝不级联静默摘除。service 层将约束冲突映射为 409（另测）。
func TestRestrictBackstop_SkillDeleteRefusedWithBinding(t *testing.T) {
	db := openFKExtendedDB(t, &skill.Skill{}, &agent.AgentConfig{}, &agent.AgentSkill{})
	sk := &skill.Skill{Name: "s1", TenantID: "org-a", Type: "expert"}
	require.NoError(t, db.Create(sk).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "org-a", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(&agent.AgentSkill{AgentID: a.ID, SkillID: sk.ID}).Error)

	err := db.Where("id = ?", sk.ID).Delete(&skill.Skill{}).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "FOREIGN KEY constraint failed")

	var skillCnt, bindCnt int64
	require.NoError(t, db.Model(&skill.Skill{}).Where("id = ?", sk.ID).Count(&skillCnt).Error)
	require.Equal(t, int64(1), skillCnt)
	require.NoError(t, db.Model(&agent.AgentSkill{}).Where("skill_id = ?", sk.ID).Count(&bindCnt).Error)
	require.Equal(t, int64(1), bindCnt)
}

func TestRestrictBackstop_McpDeleteRefusedWithBinding(t *testing.T) {
	db := openFKExtendedDB(t, &mcp.McpServer{}, &agent.AgentConfig{}, &mcp.AgentMcpServer{})
	srv := &mcp.McpServer{Name: "fs", TenantID: "org-a", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, db.Create(srv).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "org-a", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: a.ID, McpServerID: srv.ID}).Error)

	err := db.Where("id = ?", srv.ID).Delete(&mcp.McpServer{}).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "FOREIGN KEY constraint failed")

	var srvCnt, bindCnt int64
	require.NoError(t, db.Model(&mcp.McpServer{}).Where("id = ?", srv.ID).Count(&srvCnt).Error)
	require.Equal(t, int64(1), srvCnt)
	require.NoError(t, db.Model(&mcp.AgentMcpServer{}).Where("mcp_server_id = ?", srv.ID).Count(&bindCnt).Error)
	require.Equal(t, int64(1), bindCnt)
}

func TestRestrictBackstop_ToolDeleteRefusedWithBinding(t *testing.T) {
	db := openFKExtendedDB(t, &agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{})
	tool := &agent.Tool{Name: "t1", TenantID: "org-a", Source: agent.ToolSourceCustom}
	require.NoError(t, db.Create(tool).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "org-a", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(&agent.AgentTool{AgentID: a.ID, ToolID: tool.ID}).Error)

	err := db.Where("id = ?", tool.ID).Delete(&agent.Tool{}).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "FOREIGN KEY constraint failed")

	var toolCnt, bindCnt int64
	require.NoError(t, db.Model(&agent.Tool{}).Where("id = ?", tool.ID).Count(&toolCnt).Error)
	require.Equal(t, int64(1), toolCnt)
	require.NoError(t, db.Model(&agent.AgentTool{}).Where("tool_id = ?", tool.ID).Count(&bindCnt).Error)
	require.Equal(t, int64(1), bindCnt)
}
