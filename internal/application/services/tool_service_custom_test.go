package services

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupToolCustomServiceDB 全局 DB 替换（ToolRepository 读 database.GetDB()，
// 模式同 tool_service_test.go 的 setupToolServiceTestDB）。
func setupToolCustomServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func newCustomToolService(t *testing.T) (*ToolService, *mockUploader) {
	t.Helper()
	up := &mockUploader{data: map[string][]byte{}}
	return NewToolService(up), up
}

const tsContent = "export default { name: 'SayHello', description: 'd' }\n"

func customFileInput() (*bytes.Buffer, ToolFileInput) {
	buf := bytes.NewBufferString(tsContent)
	return buf, ToolFileInput{FileName: "say_hello.ts", File: buf, FileSize: int64(buf.Len())}
}

func TestCreateCustomTool_Success(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	dto, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name:          "SayHello",
		Title:         "问候",
		ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize},
	})
	require.NoError(t, err)
	require.Equal(t, agent.ToolSourceCustom, dto.Source)
	require.Equal(t, agent.ToolArtifactReady, dto.ArtifactStatus)
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(tsContent)))
	require.Equal(t, sum, dto.FileHash)
	require.Equal(t, "tools/acme/SayHello/"+sum+".ts", dto.FileURL)
	_, ok := up.data["tools/acme/SayHello/"+sum+".ts"]
	require.True(t, ok, "对象必须写入内容寻址 key")
}

func TestCreateCustomTool_Validations(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)

	// 扩展名不支持（含大写 .TS）
	buf := bytes.NewBufferString("x")
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", ToolFileInput: ToolFileInput{FileName: "a.py", File: buf, FileSize: 1}})
	require.ErrorIs(t, err, agent.ErrInvalidToolFile)
	buf2 := bytes.NewBufferString("x")
	_, err = svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", ToolFileInput: ToolFileInput{FileName: "a.TS", File: buf2, FileSize: 1}})
	require.ErrorIs(t, err, agent.ErrInvalidToolFile)

	// 空文件
	buf3 := bytes.NewBufferString("")
	_, err = svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", ToolFileInput: ToolFileInput{FileName: "a.ts", File: buf3, FileSize: 0}})
	require.ErrorIs(t, err, agent.ErrToolFileEmpty)

	// 超限 5 MiB + 1
	big := bytes.Repeat([]byte("a"), MaxToolFileSize+1)
	_, err = svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", ToolFileInput: ToolFileInput{FileName: "a.ts", File: bytes.NewReader(big), FileSize: int64(len(big))}})
	require.ErrorIs(t, err, agent.ErrToolFileTooLarge)

	// 与可见内置工具重名（共享行 Bash 由 seed 直接插库模拟）
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	_, err = svc.CreateCustomTool("acme", CreateToolInputLike("Bash"))
	require.ErrorIs(t, err, agent.ErrToolNameExists)
}

// CreateToolInputLike 是测试辅助：按 CreateCustomToolInput 组同名请求。
func CreateToolInputLike(name string) *CreateCustomToolInput {
	buf := bytes.NewBufferString(tsContent)
	return &CreateCustomToolInput{Name: name, ToolFileInput: ToolFileInput{FileName: "x.ts", File: buf, FileSize: int64(buf.Len())}}
}

func TestCreateCustomTool_StorageDisabled(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc := NewToolService(nil)
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize}})
	require.ErrorIs(t, err, agent.ErrToolStorageDisabled)
}

func TestUploadToolFile_ReplaceCleansOldKey(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	created, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize}})
	require.NoError(t, err)
	oldKey := created.FileURL

	// 替换为新内容 → 新 hash → 新 key；旧 key 被清理
	buf2 := bytes.NewBufferString("export default { name: 'SayHello' } // v2\n")
	dto, err := svc.UploadToolFile("acme", "SayHello", &ToolFileInput{FileName: "v2.mjs", File: buf2, FileSize: int64(buf2.Len())})
	require.NoError(t, err)
	require.Equal(t, "v2.mjs", dto.FileName)
	require.NotEqual(t, oldKey, dto.FileURL)
	_, oldExists := up.data[oldKey]
	require.False(t, oldExists, "旧对象应尽力清理")
	_, newExists := up.data[dto.FileURL]
	require.True(t, newExists)
}

func TestUploadToolFile_MissingBackfill_AndBuiltinRejected(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	// missing 存量行（迁移产物）
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Legacy", TenantID: "acme", Source: agent.ToolSourceCustom}).Error)
	buf := bytes.NewBufferString(tsContent)
	dto, err := svc.UploadToolFile("acme", "Legacy", &ToolFileInput{FileName: "l.ts", File: buf, FileSize: int64(buf.Len())})
	require.NoError(t, err)
	require.Equal(t, agent.ToolArtifactReady, dto.ArtifactStatus)

	// builtin 拒绝补传
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	buf2 := bytes.NewBufferString(tsContent)
	_, err = svc.UploadToolFile("acme", "Bash", &ToolFileInput{FileName: "b.ts", File: buf2, FileSize: int64(buf2.Len())})
	require.ErrorIs(t, err, agent.ErrToolIsBuiltin)
}

func TestUpdateTool_MetadataOnly(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize}})
	require.NoError(t, err)
	title := "新标题"
	dto, err := svc.Update("acme", "SayHello", &UpdateToolInput{Title: &title})
	require.NoError(t, err)
	require.Equal(t, "新标题", dto.Title)

	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	_, err = svc.Update("acme", "Bash", &UpdateToolInput{Title: &title})
	require.ErrorIs(t, err, agent.ErrToolIsBuiltin)
}

func TestDeleteTool_Protections(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)

	// builtin 拒绝删除
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	require.ErrorIs(t, svc.Delete("acme", "Bash"), agent.ErrToolIsBuiltin)

	// 有关联 → ToolInUseError 带名单
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize}})
	require.NoError(t, err)
	a := &agent.AgentConfig{Name: "bot", TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	var tool agent.Tool
	require.NoError(t, database.GetDB().Where("name = 'SayHello'").First(&tool).Error)
	require.NoError(t, database.GetDB().Create(&agent.AgentTool{AgentID: a.ID, ToolID: tool.ID}).Error)
	err = svc.Delete("acme", "SayHello")
	var inUse *agent.ToolInUseError
	require.ErrorAs(t, err, &inUse)
	require.Equal(t, []string{"bot"}, inUse.Agents)

	// 解除关联后删除成功且清理对象
	require.NoError(t, database.GetDB().Where("agent_id = ?", a.ID).Delete(&agent.AgentTool{}).Error)
	require.NoError(t, svc.Delete("acme", "SayHello"))
	_, exists := up.data["tools/acme/SayHello/"+tool.FileHash+".ts"]
	require.False(t, exists)
}

// TestDeleteTool_DBFailureKeepsObjectAndRow 锁定删除顺序契约（expert review
// Fix 1）：DB 行删除失败时 OSS 对象必须原封不动——若先删对象后删行失败，会
// 留下指向已删对象的 ready 行（false-ready）。经 GORM Delete 回调注入，强制
// tools 表的删除语句失败。
func TestDeleteTool_DBFailureKeepsObjectAndRow(t *testing.T) {
	db := setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	created, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name:          "SayHello",
		ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize},
	})
	require.NoError(t, err)

	forced := errors.New("forced tools delete failure")
	require.NoError(t, db.Callback().Delete().Before("gorm:before_delete").Register("test:force_tools_delete_fail", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "tools" {
			_ = tx.AddError(forced)
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Delete().Remove("test:force_tools_delete_fail")
	})

	err = svc.Delete("acme", "SayHello")
	require.ErrorIs(t, err, forced)
	require.Contains(t, err.Error(), "delete tool failed")

	// OSS 对象保留：行删除失败时不得先清对象（行+对象保持一致）
	_, objExists := up.data[created.FileURL]
	require.True(t, objExists, "DB 删除失败时 OSS 对象必须原封不动")

	// DB 行保留
	var cnt int64
	require.NoError(t, db.Model(&agent.Tool{}).Where("name = ?", "SayHello").Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)
}

// forceToolsUpdateFail 经 GORM Update 回调注入，强制 tools 表的更新语句失败
// （模式同 TestDeleteTool_DBFailureKeepsObjectAndRow 的 Delete 注入）。
func forceToolsUpdateFail(t *testing.T, db *gorm.DB) error {
	t.Helper()
	forced := errors.New("forced tools update failure")
	require.NoError(t, db.Callback().Update().Before("gorm:before_update").Register("test:force_tools_update_fail", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "tools" {
			_ = tx.AddError(forced)
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Update().Remove("test:force_tools_update_fail")
	})
	return forced
}

// TestUploadToolFile_DBFailureRollsBackNewObject 锁定补传/替换的回滚契约
// （expert review round 2 Fix A）：repo.Update 失败时，刚上传的 newKey 必须
// 回滚删除，而旧行及其 oldKey 对象原封不动（issue #88「数据库更新失败及旧
// 制品清理」覆盖）。
func TestUploadToolFile_DBFailureRollsBackNewObject(t *testing.T) {
	db := setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	created, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name:          "SayHello",
		ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize},
	})
	require.NoError(t, err)
	oldKey := created.FileURL

	forced := forceToolsUpdateFail(t, db)

	// 替换为不同内容 → 新 hash → 新 key；DB 更新失败 → 新对象必须回滚删除
	v2 := "export default { name: 'SayHello' } // v2\n"
	buf := bytes.NewBufferString(v2)
	_, err = svc.UploadToolFile("acme", "SayHello", &ToolFileInput{FileName: "v2.mjs", File: buf, FileSize: int64(buf.Len())})
	require.ErrorIs(t, err, forced)
	require.Contains(t, err.Error(), "update tool artifact failed")

	newKey := BuildToolOSSKey("acme", "SayHello", fmt.Sprintf("%x", sha256.Sum256([]byte(v2))), ".mjs")
	_, newObjExists := up.data[newKey]
	require.False(t, newObjExists, "DB 更新失败时刚上传的新对象必须回滚删除")
	_, oldObjExists := up.data[oldKey]
	require.True(t, oldObjExists, "旧行的 oldKey 对象必须原封不动")

	// DB 行仍持有旧制品三元组（ready、旧 hash）
	var row agent.Tool
	require.NoError(t, db.Where("name = ?", "SayHello").First(&row).Error)
	require.Equal(t, agent.ToolArtifactReady, row.ArtifactStatus())
	require.Equal(t, created.FileHash, row.FileHash)
	require.Equal(t, oldKey, row.FileURL)
}

// TestUploadToolFile_DBFailureSameKeyKeepsObject 同内容重传（同 key）+ DB
// 更新失败：共享 key 对象绝不能删——它正是完好旧行仍在引用的 live artifact
// （回滚删除必须以 newKey != oldKey 为界）。
func TestUploadToolFile_DBFailureSameKeyKeepsObject(t *testing.T) {
	db := setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	created, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name:          "SayHello",
		ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize},
	})
	require.NoError(t, err)

	forced := forceToolsUpdateFail(t, db)

	// 同内容重传 → 同 key；DB 更新失败 → 该对象必须保留
	buf := bytes.NewBufferString(tsContent)
	_, err = svc.UploadToolFile("acme", "SayHello", &ToolFileInput{FileName: "same.ts", File: buf, FileSize: int64(buf.Len())})
	require.ErrorIs(t, err, forced)
	_, objExists := up.data[created.FileURL]
	require.True(t, objExists, "同内容同 key 时对象是旧行的 live artifact，不得删除")
}

func TestDownloadTool(t *testing.T) {
	setupToolCustomServiceDB(t)
	up := &mockUploader{data: map[string][]byte{}}
	svc := NewToolService(up)
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize}})
	require.NoError(t, err)
	dto, err := svc.Download("acme", "SayHello")
	require.NoError(t, err)
	// 管理端恒 presigned（issue #88）：公共 CDN 链接仅保留给 deployer 侧制品
	// 来源（agent_deployer.go buildArtifactURL）；URL 为 mockUploader.
	// GetPresignedURL 的 presigned 形态。
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(tsContent)))
	require.Equal(t, "https://example.com/tools/acme/SayHello/"+sum+".ts", dto.URL)
	require.Equal(t, int64(3600), dto.ExpiresIn)

	// missing → 明确错误
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Legacy", TenantID: "acme", Source: agent.ToolSourceCustom}).Error)
	_, err = svc.Download("acme", "Legacy")
	require.ErrorIs(t, err, agent.ErrToolArtifactMissing)
}

func TestUpdateAgentTools_RejectNewMissing(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	a := &agent.AgentConfig{Name: "bot", TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	missing := &agent.Tool{Name: "Legacy", TenantID: "acme", Source: agent.ToolSourceCustom}
	require.NoError(t, database.GetDB().Create(missing).Error)

	// 新增 missing → 拒绝且报出工具名
	err := svc.UpdateAgentTools("acme", "bot", []string{"Legacy"})
	require.ErrorIs(t, err, agent.ErrToolArtifactMissing)
	require.Contains(t, err.Error(), "Legacy")

	// 已有关联的 missing 保持挂载合法（用户仅未勾选移除其他工具）
	require.NoError(t, database.GetDB().Create(&agent.AgentTool{AgentID: a.ID, ToolID: missing.ID}).Error)
	require.NoError(t, svc.UpdateAgentTools("acme", "bot", []string{"Legacy"}))
}

// TestToolOps_NotFoundSentinel 锁定 not-found 契约（Task 3 review Fix 1）：
// 不存在的工具名必须 errors.Is(err, agent.ErrToolNotFound)（handler 据此
// 映射 404），且错误消息携带工具名。
func TestToolOps_NotFoundSentinel(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	title := "t"
	cases := []struct {
		name string
		op   func() error
	}{
		{"GetByName", func() error { _, err := svc.GetByName("acme", "Ghost"); return err }},
		{"Update", func() error { _, err := svc.Update("acme", "Ghost", &UpdateToolInput{Title: &title}); return err }},
		{"UploadToolFile", func() error {
			buf := bytes.NewBufferString(tsContent)
			_, err := svc.UploadToolFile("acme", "Ghost", &ToolFileInput{FileName: "g.ts", File: buf, FileSize: int64(buf.Len())})
			return err
		}},
		{"Delete", func() error { return svc.Delete("acme", "Ghost") }},
		{"Download", func() error { _, err := svc.Download("acme", "Ghost"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.op()
			require.ErrorIs(t, err, agent.ErrToolNotFound)
			require.Contains(t, err.Error(), "Ghost")
		})
	}

	// UpdateAgentTools 的 per-tool 查找同契约（Agent 本身存在，工具不存在）
	a := &agent.AgentConfig{Name: "bot", TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	require.ErrorIs(t, svc.UpdateAgentTools("acme", "bot", []string{"Ghost"}), agent.ErrToolNotFound)
}

// issue #123 收敛：工具 409 载荷与知识库/技能/MCP 同构——他租户挂载仅
// foreign 中性事实，不进入名单（对齐知识库 review P1）。
func TestDeleteTool_ForeignOnlyBlocks(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name:          "SayHello",
		ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize},
	})
	require.NoError(t, err)
	var tool agent.Tool
	require.NoError(t, database.GetDB().Where("name = 'SayHello'").First(&tool).Error)
	fb := &agent.AgentConfig{Name: "sneaky", TenantID: "other", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(fb).Error)
	require.NoError(t, database.GetDB().Create(&agent.AgentTool{AgentID: fb.ID, ToolID: tool.ID}).Error)

	err = svc.Delete("acme", "SayHello")
	var inUse *agent.ToolInUseError
	require.ErrorAs(t, err, &inUse)
	require.Empty(t, inUse.Agents)
	require.True(t, inUse.Foreign)
}

// TestDeleteTool_FKConflictMapsToInUse 并发后盾（review P1）：
// RESTRICT 拒绝删除的约束冲突映射为 ToolInUseError（重查绑定给出准确名单）。
func TestDeleteTool_FKConflictMapsToInUse(t *testing.T) {
	db := setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name:          "SayHello",
		ToolFileInput: ToolFileInput{FileName: in.FileName, File: in.File, FileSize: in.FileSize},
	})
	require.NoError(t, err)
	var tool agent.Tool
	require.NoError(t, database.GetDB().Where("name = 'SayHello'").First(&tool).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	require.NoError(t, database.GetDB().Create(&agent.AgentTool{AgentID: a.ID, ToolID: tool.ID}).Error)

	forcedFK := &mysql.MySQLError{Number: 1451, Message: "Cannot delete or update a parent row"}
	require.NoError(t, db.Callback().Delete().Before("gorm:before_delete").Register("test:force_tools_fk", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "tools" {
			_ = tx.AddError(forcedFK)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Delete().Remove("test:force_tools_fk") })

	err = svc.Delete("acme", "SayHello")
	var inUse *agent.ToolInUseError
	require.ErrorAs(t, err, &inUse)
	require.Equal(t, []string{"bot"}, inUse.Agents)
	require.False(t, inUse.Foreign)
}
