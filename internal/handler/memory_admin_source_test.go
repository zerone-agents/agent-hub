package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memory"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// adminAgentMemoryIndexStub：管理台 facts 分支不触检索索引，空桩即可
// （registry 构造需要 Index；检索路径由 memory 包测试覆盖）。
type adminAgentMemoryIndexStub struct{}

func (adminAgentMemoryIndexStub) EnsureDocument(ctx context.Context, dataset, name string, allow bool) (string, error) {
	return name, nil
}
func (adminAgentMemoryIndexStub) Put(ctx context.Context, dataset, doc, content string) (string, error) {
	return content, nil
}
func (adminAgentMemoryIndexStub) Remove(ctx context.Context, dataset, doc, id string) error {
	return nil
}
func (adminAgentMemoryIndexStub) DropDocument(ctx context.Context, dataset, doc string) error {
	return nil
}
func (adminAgentMemoryIndexStub) Search(ctx context.Context, dataset, doc, query string, n int) ([]memory.Hit, error) {
	return nil, nil
}
func (adminAgentMemoryIndexStub) Find(ctx context.Context, dataset, doc, content string) (string, error) {
	return "", nil
}

func newAdminAgentMemoryEnv(t *testing.T) (*MemoryAdminHandler, *services.AgentMemoryRegistry) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/admin-agent-memory.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&knowledge.ReservedDataset{}))
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS agent_knowledge_datasets (
		agent_id INTEGER NOT NULL DEFAULT 0,
		dataset_id TEXT NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, memory.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("01", 32))
	require.NoError(t, err)
	registry := services.NewAgentMemoryRegistry(db, wrapper, services.NewDatasetGuard(db), adminAgentMemoryIndexStub{})

	// 经 registry 解析的服务写入一条 agent 记忆（生产同路径）
	svc, err := registry.Service("t1")
	require.NoError(t, err)
	agentScope := memory.Scope{
		TenantID: "t1", ApplicationID: services.AgentMemoryApplication,
		UserID: "agent:counselor", LifecycleID: "agent-memory-v1",
	}
	require.NoError(t, svc.EnsureAgentLifecycle(context.Background(), agentScope))
	_, err = svc.RememberAgentFact(context.Background(), agentScope, memory.AgentMemoryInput{
		Subject: "用户", Category: "情绪", Content: "ADMIN_PRIVATE_CONTENT 对工作变动焦虑", Quote: "想到换工作就睡不着",
	})
	require.NoError(t, err)

	h := NewMemoryAdminHandler(nil, services.NewAuditRecorder(&memoryAdminAuditStore{}))
	return h, registry
}

func adminFactsRouter(h *MemoryAdminHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "t1")
		c.Set("roles", []string{"admin"})
		c.Set("user_id", "manager-1")
	})
	RegisterMemoryAdminRoutes(r.Group("/api/v1/admin"), r.Group("/api/v1/admin"), h)
	return r
}

const adminAgentFactsQuery = "/api/v1/admin/memory/facts?tenantId=t1&applicationId=" +
	services.AgentMemoryApplication + "&userId=agent:counselor&accountLifecycleId=agent-memory-v1"

func TestMemoryAdminFactsExposeAgentMemorySource(t *testing.T) {
	h, registry := newAdminAgentMemoryEnv(t)
	h.SetAgentMemoryRegistry(registry)
	r := adminFactsRouter(h)

	req := httptest.NewRequest(http.MethodGet, adminAgentFactsQuery, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "agent:counselor", "source 透出写入者（spec §4）")
	require.Contains(t, rec.Body.String(), "ADMIN_PRIVATE_CONTENT")
}

func TestMemoryAdminAgentFactsCrossTenantForbidden(t *testing.T) {
	h, registry := newAdminAgentMemoryEnv(t)
	h.SetAgentMemoryRegistry(registry)
	r := adminFactsRouter(h)

	// 管理租户 t1 查询他租户的 Agent 记忆 → 403（Agent 记忆归属 hub 租户自身）
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/memory/facts?tenantId=other&applicationId="+
		services.AgentMemoryApplication+"&userId=agent:counselor&accountLifecycleId=agent-memory-v1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code, "body=%s", rec.Body.String())
}

func TestMemoryAdminAgentFactsUnavailableWithoutRegistry(t *testing.T) {
	h, _ := newAdminAgentMemoryEnv(t)
	// 未 SetAgentMemoryRegistry（knowledge 连接未配置的部署形态）→ 503 可用性提示
	r := adminFactsRouter(h)

	req := httptest.NewRequest(http.MethodGet, adminAgentFactsQuery, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "body=%s", rec.Body.String())
}

func adminAgentRouter(h *MemoryAdminHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "t1")
		c.Set("roles", []string{"admin"})
		c.Set("user_id", "manager-1")
	})
	RegisterMemoryAdminRoutes(r.Group("/api/v1/admin"), r.Group("/api/v1/admin"), h)
	return r
}

func TestMemoryAdminAgentMemorySubjectUpdateDelete(t *testing.T) {
	h, registry := newAdminAgentMemoryEnv(t)
	h.SetAgentMemoryRegistry(registry)
	r := adminAgentRouter(h)

	// applications 含保留应用条目（前端导航可达——review 阻塞②）
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/memory/applications", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), services.AgentMemoryApplication, "保留应用出现在 applications 列表")

	// subjects：经 selectService 的 registry 分支列出 agent scope
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/memory/subjects?tenantId=t1&applicationId="+services.AgentMemoryApplication, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "agent:counselor", "agent scope 列为 subject")

	// 修正（quote 宽松的管理通道变体）+ 物理删除（池满腾位闭环）
	scope := `{"tenantId":"t1","applicationId":"` + services.AgentMemoryApplication + `","userId":"agent:counselor","accountLifecycleId":"agent-memory-v1"}`
	svc, err := registry.Service("t1")
	require.NoError(t, err)
	sc := memory.Scope{TenantID: "t1", ApplicationID: services.AgentMemoryApplication, UserID: "agent:counselor", LifecycleID: "agent-memory-v1"}
	records, err := svc.ListAgentMemories(context.Background(), sc, "", "", 50)
	require.NoError(t, err)
	require.NotEmpty(t, records)
	fact := records[len(records)-1]

	updateBody := `{"scope":` + scope + `,"operationId":"op-u","expectedVersion":` + strconvItoa(int(fact.Version)) + `,"content":"管理员修正"}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/memory/facts/"+fact.ID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "管理员修正", "修正生效")

	// 删除（先取修正后的新版本）
	records, err = svc.ListAgentMemories(context.Background(), sc, "", fact.ID, 50)
	require.NoError(t, err)
	require.Len(t, records, 1)
	updated := records[0]
	deleteBody := `{"scope":` + scope + `,"operationId":"op-d","expectedVersion":` + strconvItoa(int(updated.Version)) + `}`
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/memory/facts/"+fact.ID, strings.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "physical", "物理删除语义可见")
	var cnt int64
	// 物理删除断言走 facts 视图：直取该 id 返回空
	gone, err := svc.ListAgentMemories(context.Background(), sc, "", fact.ID, 50)
	require.NoError(t, err)
	require.Empty(t, gone, "行已物理删除（直取为空）")
	_ = cnt
}

func strconvItoa(v int) string { return fmt.Sprintf("%d", v) }
