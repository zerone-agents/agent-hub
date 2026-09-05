package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/skill"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSkillHandlerRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skill.Skill{}, &agent.AgentConfig{}, &agent.AgentSkill{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	h := NewSkillHandler(services.NewSkillService(&toolUploaderMock{data: map[string][]byte{}}, "https://cdn.example.com"))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 生产租户来自 JWT 中间件 c.Set("tenant_id")（tool_custom_test 同款注入）
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	r.DELETE("/api/v1/admin/skills/:name", h.Delete)
	return r
}

func TestSkillHandler_DeleteConflict409(t *testing.T) {
	r := setupSkillHandlerRouter(t)
	sk := &skill.Skill{Name: "skill-a", TenantID: "tenant-a", Type: "expert"}
	require.NoError(t, database.GetDB().Create(sk).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "tenant-a", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	require.NoError(t, database.GetDB().Create(&agent.AgentSkill{AgentID: a.ID, SkillID: sk.ID}).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/skills/skill-a", nil))
	require.Equal(t, http.StatusConflict, resp.Code)
	require.Contains(t, resp.Body.String(), `"agents":["bot"]`)
	require.Contains(t, resp.Body.String(), `"foreign":false`)
}

func TestSkillHandler_DeleteConflict409_CrossTenantNoLeak(t *testing.T) {
	r := setupSkillHandlerRouter(t)
	sk := &skill.Skill{Name: "skill-a", TenantID: "tenant-a", Type: "expert"}
	require.NoError(t, database.GetDB().Create(sk).Error)
	fb := &agent.AgentConfig{Name: "sneaky", TenantID: "tenant-b", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(fb).Error)
	require.NoError(t, database.GetDB().Create(&agent.AgentSkill{AgentID: fb.ID, SkillID: sk.ID}).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/skills/skill-a", nil))
	require.Equal(t, http.StatusConflict, resp.Code)
	require.NotContains(t, resp.Body.String(), "sneaky")
	require.Contains(t, resp.Body.String(), `"foreign":true`)
}

func TestSkillHandler_DeleteUnboundOK(t *testing.T) {
	r := setupSkillHandlerRouter(t)
	sk := &skill.Skill{Name: "skill-a", TenantID: "tenant-a", Type: "expert"}
	require.NoError(t, database.GetDB().Create(sk).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/skills/skill-a", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), "技能已删除")
}
