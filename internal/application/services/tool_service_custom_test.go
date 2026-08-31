package services

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
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
	return NewToolService(up, ""), up
}

const tsContent = "export default { name: 'SayHello', description: 'd' }\n"

func customFileInput() (*bytes.Buffer, CustomToolFileInput) {
	buf := bytes.NewBufferString(tsContent)
	return buf, CustomToolFileInput{FileName: "say_hello.ts", File: buf, FileSize: int64(buf.Len())}
}

func TestCreateCustomTool_Success(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	dto, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{
		Name: "SayHello", Title: "问候", FileName: in.FileName, File: in.File, FileSize: in.FileSize,
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
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", FileName: "a.py", File: buf, FileSize: 1})
	require.ErrorIs(t, err, agent.ErrInvalidToolFile)
	buf2 := bytes.NewBufferString("x")
	_, err = svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", FileName: "a.TS", File: buf2, FileSize: 1})
	require.ErrorIs(t, err, agent.ErrInvalidToolFile)

	// 空文件
	buf3 := bytes.NewBufferString("")
	_, err = svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", FileName: "a.ts", File: buf3, FileSize: 0})
	require.ErrorIs(t, err, agent.ErrToolFileEmpty)

	// 超限 5 MiB + 1
	big := bytes.Repeat([]byte("a"), MaxToolFileSize+1)
	_, err = svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", FileName: "a.ts", File: bytes.NewReader(big), FileSize: int64(len(big))})
	require.ErrorIs(t, err, agent.ErrToolFileTooLarge)

	// 与可见内置工具重名（共享行 Bash 由 seed 直接插库模拟）
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	_, err = svc.CreateCustomTool("acme", CreateToolInputLike("Bash"))
	require.ErrorIs(t, err, agent.ErrToolNameExists)
}

// CreateToolInputLike 是测试辅助：按 CreateCustomToolInput 组同名请求。
func CreateToolInputLike(name string) *CreateCustomToolInput {
	buf := bytes.NewBufferString(tsContent)
	return &CreateCustomToolInput{Name: name, FileName: "x.ts", File: buf, FileSize: int64(buf.Len())}
}

func TestCreateCustomTool_StorageDisabled(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc := NewToolService(nil, "")
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "A", FileName: in.FileName, File: in.File, FileSize: in.FileSize})
	require.ErrorIs(t, err, agent.ErrToolStorageDisabled)
}

func TestUploadToolFile_ReplaceCleansOldKey(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, up := newCustomToolService(t)
	_, in := customFileInput()
	created, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", FileName: in.FileName, File: in.File, FileSize: in.FileSize})
	require.NoError(t, err)
	oldKey := created.FileURL

	// 替换为新内容 → 新 hash → 新 key；旧 key 被清理
	buf2 := bytes.NewBufferString("export default { name: 'SayHello' } // v2\n")
	dto, err := svc.UploadToolFile("acme", "SayHello", &CustomToolFileInput{FileName: "v2.mjs", File: buf2, FileSize: int64(buf2.Len())})
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
	dto, err := svc.UploadToolFile("acme", "Legacy", &CustomToolFileInput{FileName: "l.ts", File: buf, FileSize: int64(buf.Len())})
	require.NoError(t, err)
	require.Equal(t, agent.ToolArtifactReady, dto.ArtifactStatus)

	// builtin 拒绝补传
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	buf2 := bytes.NewBufferString(tsContent)
	_, err = svc.UploadToolFile("acme", "Bash", &CustomToolFileInput{FileName: "b.ts", File: buf2, FileSize: int64(buf2.Len())})
	require.ErrorIs(t, err, agent.ErrToolIsBuiltin)
}

func TestUpdateTool_MetadataOnly(t *testing.T) {
	setupToolCustomServiceDB(t)
	svc, _ := newCustomToolService(t)
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", FileName: in.FileName, File: in.File, FileSize: in.FileSize})
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
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", FileName: in.FileName, File: in.File, FileSize: in.FileSize})
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

func TestDownloadTool(t *testing.T) {
	setupToolCustomServiceDB(t)
	up := &mockUploader{data: map[string][]byte{}}
	svc := NewToolService(up, "https://cdn.example.com")
	_, in := customFileInput()
	_, err := svc.CreateCustomTool("acme", &CreateCustomToolInput{Name: "SayHello", FileName: in.FileName, File: in.File, FileSize: in.FileSize})
	require.NoError(t, err)
	dto, err := svc.Download("acme", "SayHello")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(dto.URL, "https://cdn.example.com/tools/acme/SayHello/"))

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
