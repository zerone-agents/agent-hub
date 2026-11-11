package services

import (
	"context"
	"strings"
	"testing"

	"control-panel/internal/domain/knowledge"
	"control-panel/internal/memory"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// registryTestIndex 是 memory.Index 的空桩：Service 构造与 Reserve 断言
// 不触达索引；检索路径由 memory 包自身的测试覆盖。
type registryTestIndex struct{}

func (registryTestIndex) EnsureDocument(ctx context.Context, dataset, name string, allow bool) (string, error) {
	return name, nil
}
func (registryTestIndex) Put(ctx context.Context, dataset, doc, content string) (string, error) {
	return content, nil
}
func (registryTestIndex) Remove(ctx context.Context, dataset, doc, id string) error { return nil }
func (registryTestIndex) DropDocument(ctx context.Context, dataset, doc string) error {
	return nil
}
func (registryTestIndex) Search(ctx context.Context, dataset, doc, query string, n int) ([]memory.Hit, error) {
	return nil, nil
}
func (registryTestIndex) Find(ctx context.Context, dataset, doc, content string) (string, error) {
	return "", nil
}

func newAgentMemoryRegistryForTest(t *testing.T) (*AgentMemoryRegistry, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent-memory-registry.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&knowledge.ReservedDataset{}))
	// DatasetGuard 裸查 agent_knowledge_datasets（防挂载冲突）；最小建表即可。
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS agent_knowledge_datasets (
		agent_id INTEGER NOT NULL DEFAULT 0,
		dataset_id TEXT NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, memory.Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("01", 32))
	require.NoError(t, err)
	return NewAgentMemoryRegistry(db, wrapper, NewDatasetGuard(db), registryTestIndex{}), db
}

func TestAgentMemoryRegistryPerTenantIsolation(t *testing.T) {
	r, db := newAgentMemoryRegistryForTest(t)
	sa, err := r.Service("tenant-a")
	require.NoError(t, err)
	sb, err := r.Service("tenant-b")
	require.NoError(t, err)
	require.NotSame(t, sa, sb, "每租户独立 Service 实例")
	// dataset 名按租户派生并各自预留：MultiRAG 库名全局唯一，同名会跨租户
	// 混检索副本（spec §6「按租户预留」）
	require.NotEqual(t, AgentMemoryDatasetID("tenant-a"), AgentMemoryDatasetID("tenant-b"))
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		var n int64
		require.NoError(t, db.Model(&knowledge.ReservedDataset{}).
			Where("dataset_id = ?", AgentMemoryDatasetID(tenant)).Count(&n).Error)
		require.Equal(t, int64(1), n, tenant)
	}
	// 缓存复用：同租户重复解析返回同一实例
	again, err := r.Service("tenant-a")
	require.NoError(t, err)
	require.Same(t, sa, again)
}

func TestAgentMemoryRegistryRejectsEmptyTenant(t *testing.T) {
	r, _ := newAgentMemoryRegistryForTest(t)
	_, err := r.Service("  ")
	require.Error(t, err, "空租户拒绝（防御 AgentRuntimeAuth 之外的直调）")
}
