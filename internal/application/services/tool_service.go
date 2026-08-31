package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/oss"

	"gorm.io/gorm"
)

type ToolService struct {
	repo      *repository.ToolRepository
	agentRepo *repository.AgentRepository
	uploader  oss.OSSUploader
	// cdnHost 管理端下载已不使用（Download 恒 presigned，issue #88）；公共 CDN
	// 仅供 deployer 制品来源（agent_deployer.go buildArtifactURL，独立配置）。
	// 保留构造签名与测试注入点：TestDownloadTool 传入非空值以证明被忽略。
	cdnHost string
}

func NewToolService(uploader oss.OSSUploader, cdnHost string) *ToolService {
	return &ToolService{
		repo:      repository.NewToolRepository(),
		agentRepo: repository.NewAgentRepository(),
		uploader:  uploader,
		cdnHost:   cdnHost,
	}
}

// presetToolSpec 是预设工具的单源定义：名字集合必须与 agent.PresetToolNames
// 一一对应（seedAlways=true 的三条走 SeedBuiltins 幂等补种，其余走
// SeedIfEmpty 空表首种）。新增预设工具时改 agent.PresetToolNames + 这里。
type presetToolSpec struct {
	tool       agent.Tool
	seedAlways bool
}

var presetToolSpecs = []presetToolSpec{
	{agent.Tool{Name: "Skill", Title: "技能加载", Description: "加载专门的技能，为特定任务提供领域专用指令和工作流程", IsDefault: false}, true},
	{agent.Tool{Name: "Task", Title: "任务派发", Description: "将子任务派发给指定的子 Agent 执行，获取其结果", IsDefault: false}, true},
	{agent.Tool{Name: "MultiTask", Title: "并行任务派发", Description: "一次性将多个子任务并行派发给子 Agent 执行", IsDefault: false}, true},
	{agent.Tool{Name: "Bash", Title: "执行命令", Description: "在持久化的 shell 会话中执行 bash 命令，支持超时和工作目录设置", IsDefault: false}, false},
	{agent.Tool{Name: "Read", Title: "读取文件", Description: "读取文件内容，支持文本文件、图片和 PDF，带行号显示", IsDefault: false}, false},
	{agent.Tool{Name: "Write", Title: "写入文件", Description: "将内容写入指定文件，不存在则创建，存在则覆盖", IsDefault: false}, false},
	{agent.Tool{Name: "Edit", Title: "编辑文件", Description: "对文件执行精确的字符串替换，支持多行匹配", IsDefault: false}, false},
	{agent.Tool{Name: "Glob", Title: "搜索文件", Description: "按 glob 模式匹配查找文件，支持递归搜索", IsDefault: false}, false},
	{agent.Tool{Name: "Grep", Title: "搜索内容", Description: "使用正则表达式搜索文件内容，支持文件类型过滤和上下文行", IsDefault: false}, false},
	{agent.Tool{Name: "WebFetch", Title: "获取网页", Description: "从 URL 获取内容并返回文本。支持 HTML 页面、JSON API 和普通文本，自动去除 HTML 标签以便阅读。", IsDefault: false}, false},
	{agent.Tool{Name: "WebSearch", Title: "网络搜索", Description: "使用 Exa AI 搜索实时网络信息，返回标题、URL 和摘要。适用于当前事件、最新数据或知识截止日期之后的信息。", IsDefault: false}, false},
	{agent.Tool{Name: "AskUserQuestion", Title: "用户提问", Description: "向用户提出问题并要求选择答案，支持单选和多选，适用于需要用户做决策的场景。", IsDefault: false}, false},
	{agent.Tool{Name: "CronCreate", Title: "创建定时任务", Description: "创建定时任务，支持周期性任务（cron 表达式）和一次性任务（延迟秒数）。", IsDefault: false}, false},
	{agent.Tool{Name: "CronDelete", Title: "删除定时任务", Description: "删除一个已创建的定时任务。", IsDefault: false}, false},
	{agent.Tool{Name: "CronList", Title: "列出定时任务", Description: "列出所有已创建的定时任务。", IsDefault: false}, false},
	{agent.Tool{Name: "Config", Title: "配置管理", Description: "获取或设置配置值，支持会话级别的设置管理。", IsDefault: false}, false},
	{agent.Tool{Name: "TodoWrite", Title: "待办事项", Description: "创建并管理当前会话的结构化任务列表，跟踪任务进度和状态。", IsDefault: false}, false},
	{agent.Tool{Name: "FindTool", Title: "查找工具", Description: "查找尚未加载的可用工具，支持关键词搜索或精确名称选择。", IsDefault: false}, false},
}

func init() {
	byName := make(map[string]presetToolSpec, len(presetToolSpecs))
	for _, s := range presetToolSpecs {
		byName[s.tool.Name] = s
	}
	if len(byName) != len(agent.PresetToolNames) {
		panic("presetToolSpecs and agent.PresetToolNames are out of sync")
	}
	for _, name := range agent.PresetToolNames {
		if _, ok := byName[name]; !ok {
			panic("presetToolSpecs and agent.PresetToolNames are out of sync")
		}
	}
}

// builtinTools lists the built-in tools that must always exist in the database.
// Order matters only for determinism; each row is seeded idempotently.
var builtinTools = func() []agent.Tool {
	tools := make([]agent.Tool, 0, len(presetToolSpecs))
	for _, s := range presetToolSpecs {
		if s.seedAlways {
			tools = append(tools, s.tool)
		}
	}
	return tools
}()

// SeedBuiltins ensures built-in tools exist in the database. Idempotent: rows
// that already exist (matched by Name) are left untouched. This covers both
// first-time bootstrap and the upgrade path where only "Skill" pre-exists.
// SeedBuiltins 是系统路径（tenantID=”）：内置行写入为共享行。
func (s *ToolService) SeedBuiltins() error {
	const sysTenant = ""
	for i := range builtinTools {
		t := builtinTools[i]
		t.Source = agent.ToolSourceBuiltin
		exists, err := s.repo.ExistsByName(sysTenant, t.Name)
		if err != nil {
			return fmt.Errorf("检查内置 %s tool 失败: %w", t.Name, err)
		}
		if exists {
			continue
		}
		if err := s.repo.Create(sysTenant, &t); err != nil {
			return fmt.Errorf("创建内置 %s tool 失败: %w", t.Name, err)
		}
	}
	return nil
}

type ToolDTO struct {
	ID             uint64 `json:"id"`
	Name           string `json:"name"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	IsDefault      bool   `json:"isDefault"`
	Source         string `json:"source"`
	ArtifactStatus string `json:"artifactStatus"`
	FileName       string `json:"fileName,omitempty"`
	FileURL        string `json:"-"` // issue #88: object key 不外泄给前端；字段保留供服务层测试与内部使用
	FileHash       string `json:"fileHash,omitempty"`
	FileSize       int64  `json:"fileSize,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func toolToDTO(t *agent.Tool) *ToolDTO {
	dto := &ToolDTO{
		ID:             t.ID,
		Name:           t.Name,
		Title:          t.Title,
		Description:    t.Description,
		IsDefault:      t.IsDefault,
		Source:         t.Source,
		ArtifactStatus: t.ArtifactStatus(),
		CreatedAt:      t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      t.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if t.Source == agent.ToolSourceCustom {
		dto.FileName = t.FileName
		dto.FileURL = t.FileURL
		dto.FileHash = t.FileHash
		dto.FileSize = t.FileSize
	}
	return dto
}

// CreateCustomToolInput 创建请求：展示元数据 + 内嵌制品三元组（ToolFileInput）。
type CreateCustomToolInput struct {
	Name        string
	Title       string
	Description string
	ToolFileInput
}

type UpdateToolInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

func (s *ToolService) ListAll(tenantID string) ([]*ToolDTO, error) {
	tools, err := s.repo.ListAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("获取 Tool 列表失败: %w", err)
	}
	dtos := make([]*ToolDTO, 0, len(tools))
	for _, t := range tools {
		dtos = append(dtos, toolToDTO(t))
	}
	return dtos, nil
}

// getTool 统一按名取 Tool 的错误契约（issue #88）：仓储返回
// gorm.ErrRecordNotFound 时映射为 agent.ErrToolNotFound（携带工具名，
// errors.Is 可判，handler 据此映射 404）；其余 DB 错误包装为内部错误，
// 绝不伪装成 not-found。
func (s *ToolService) getTool(tenantID, name string) (*agent.Tool, error) {
	t, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", agent.ErrToolNotFound, name)
		}
		return nil, fmt.Errorf("查询 Tool 失败: %w", err)
	}
	return t, nil
}

func (s *ToolService) GetByName(tenantID, name string) (*ToolDTO, error) {
	t, err := s.getTool(tenantID, name)
	if err != nil {
		return nil, err
	}
	return toolToDTO(t), nil
}

// CreateCustomTool 创建租户自定义工具（multipart 上传，issue #88）：先上传
// 内容寻址对象，再落库；落库失败回滚删对象（skill 先例）。
func (s *ToolService) CreateCustomTool(tenantID string, input *CreateCustomToolInput) (*ToolDTO, error) {
	if err := ValidateToolName(input.Name); err != nil {
		return nil, err
	}
	if s.uploader == nil {
		return nil, agent.ErrToolStorageDisabled
	}
	exists, err := s.repo.ExistsByName(tenantID, input.Name)
	if err != nil {
		return nil, fmt.Errorf("检查 Tool 存在性失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("%w: %s", agent.ErrToolNameExists, input.Name)
	}
	data, ext, hash, err := validateToolFileBytes(input.FileName, input.FileSize, input.File)
	if err != nil {
		return nil, err
	}
	ossKey := BuildToolOSSKey(tenantID, input.Name, hash, ext)
	ctx := context.Background()
	if _, err := s.uploader.Upload(ctx, ossKey, bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, fmt.Errorf("上传工具文件失败: %w", err)
	}
	t := &agent.Tool{
		Name:        input.Name,
		Title:       input.Title,
		Description: input.Description,
		Source:      agent.ToolSourceCustom,
		FileName:    input.FileName,
		FileURL:     ossKey,
		FileHash:    hash,
		FileSize:    int64(len(data)),
	}
	if err := s.repo.Create(tenantID, t); err != nil {
		_ = s.uploader.Delete(ctx, ossKey)
		return nil, fmt.Errorf("创建 Tool 失败: %w", err)
	}

	return toolToDTO(t), nil
}

func (s *ToolService) Update(tenantID, name string, input *UpdateToolInput) (*ToolDTO, error) {
	t, err := s.getTool(tenantID, name)
	if err != nil {
		return nil, err
	}
	if t.IsBuiltin() {
		return nil, agent.ErrToolIsBuiltin
	}
	if input.Title != nil {
		t.Title = *input.Title
	}
	if input.Description != nil {
		t.Description = *input.Description
	}
	if err := s.repo.Update(tenantID, t); err != nil {
		return nil, fmt.Errorf("更新 Tool 失败: %w", err)
	}

	return toolToDTO(t), nil
}

// UploadToolFile 同时承担存量 missing 补传与 ready 替换（issue #88）。
func (s *ToolService) UploadToolFile(tenantID, name string, input *ToolFileInput) (*ToolDTO, error) {
	t, err := s.getTool(tenantID, name)
	if err != nil {
		return nil, err
	}
	if t.IsBuiltin() {
		return nil, agent.ErrToolIsBuiltin
	}
	if s.uploader == nil {
		return nil, agent.ErrToolStorageDisabled
	}
	data, ext, hash, err := validateToolFileBytes(input.FileName, input.FileSize, input.File)
	if err != nil {
		return nil, err
	}
	newKey := BuildToolOSSKey(tenantID, t.Name, hash, ext)
	ctx := context.Background()
	if _, err := s.uploader.Upload(ctx, newKey, bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, fmt.Errorf("上传工具文件失败: %w", err)
	}
	oldKey := t.FileURL
	t.FileName = input.FileName
	t.FileURL = newKey
	t.FileHash = hash
	t.FileSize = int64(len(data))
	if err := s.repo.Update(tenantID, t); err != nil {
		return nil, fmt.Errorf("更新 Tool 制品失败: %w", err)
	}
	if oldKey != "" && oldKey != newKey {
		if err := s.uploader.Delete(ctx, oldKey); err != nil {
			log.Printf("清理旧工具文件失败 (tool=%s, key=%s): %v", t.Name, oldKey, err)
		}
	}
	return toolToDTO(t), nil
}

func (s *ToolService) Delete(tenantID, name string) error {
	t, err := s.getTool(tenantID, name)
	if err != nil {
		return err
	}
	if t.IsBuiltin() {
		return agent.ErrToolIsBuiltin
	}
	agents, err := s.repo.GetAgentNamesByToolID(t.ID)
	if err != nil {
		return fmt.Errorf("查询 Tool 关联 Agent 失败: %w", err)
	}
	if len(agents) > 0 {
		return &agent.ToolInUseError{ToolName: t.Name, Agents: agents}
	}
	// 顺序契约（expert review Fix 1）：先删 DB 行，后清 OSS 对象。若先删对象
	// 而行删除失败，会残留指向已删对象的 ready 行（false-ready），下载/部署
	// 随之断裂；先删行则最坏情况只是留下一个孤立对象（见下），行与对象始终一致。
	key := t.FileURL
	if err := s.repo.Delete(tenantID, t.ID); err != nil {
		return fmt.Errorf("删除 Tool 失败: %w", err)
	}
	// 行已删成功：残留对象只是无引用方的孤立内容寻址副本，删除失败无害，仅记日志。
	if key != "" && s.uploader != nil {
		if err := s.uploader.Delete(context.Background(), key); err != nil {
			log.Printf("清理 OSS 工具文件失败 (tool=%s, key=%s): %v", t.Name, key, err)
		}
	}
	return nil
}

// Download 对 ready 自定义工具恒返回 presigned 短期地址（issue #88：「下载仅
// 对 ready 自定义工具开放，返回短期有效地址」）。管理端刻意不走 CDN 永久链接：
// 公共 CDN URL 仅保留给 deployer 侧制品来源（agent_deployer.go buildArtifactURL）。
func (s *ToolService) Download(tenantID, name string) (*DownloadDTO, error) {
	t, err := s.getTool(tenantID, name)
	if err != nil {
		return nil, err
	}
	if t.IsBuiltin() {
		return nil, agent.ErrToolIsBuiltin
	}
	if t.ArtifactStatus() != agent.ToolArtifactReady {
		return nil, agent.ErrToolArtifactMissing
	}
	if s.uploader == nil {
		return nil, agent.ErrToolStorageDisabled
	}
	url, err := s.uploader.GetPresignedURL(context.Background(), t.FileURL)
	if err != nil {
		return nil, fmt.Errorf("生成下载链接失败: %w", err)
	}
	return &DownloadDTO{URL: url, ExpiresIn: 3600}, nil
}

func (s *ToolService) GetAgentTools(tenantID, agentName string) ([]string, error) {
	agentCfg, err := s.agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return nil, fmt.Errorf("Agent '%s' 不存在", agentName)
	}
	return s.repo.GetToolsByAgent(agentCfg.ID)
}

func (s *ToolService) UpdateAgentTools(tenantID, agentName string, toolNames []string) error {
	agentCfg, err := s.agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return fmt.Errorf("Agent '%s' 不存在", agentName)
	}

	defaultToolNames, err := s.repo.GetDefaultToolNames(tenantID)
	if err != nil {
		return fmt.Errorf("获取默认 Tool 失败: %w", err)
	}
	toolNames = mergeStringSlices(toolNames, defaultToolNames)

	// 已有关联名单：存量 missing 工具保持挂载合法，仅拒绝「新增」（issue #88）。
	currentNames, err := s.repo.GetToolsByAgent(agentCfg.ID)
	if err != nil {
		return fmt.Errorf("获取 Agent 现有 Tool 失败: %w", err)
	}
	current := make(map[string]bool, len(currentNames))
	for _, n := range currentNames {
		current[n] = true
	}

	var missingNew []string
	toolIDs := make([]uint64, 0, len(toolNames))
	for _, toolName := range toolNames {
		t, err := s.getTool(tenantID, toolName)
		if err != nil {
			return err
		}
		if t.Source == agent.ToolSourceCustom &&
			t.ArtifactStatus() == agent.ToolArtifactMissing && !current[toolName] {
			missingNew = append(missingNew, toolName)
			continue
		}
		toolIDs = append(toolIDs, t.ID)
	}
	if len(missingNew) > 0 {
		return fmt.Errorf("%w：%s", agent.ErrToolArtifactMissing, strings.Join(missingNew, "、"))
	}
	return s.repo.ReplaceAgentTools(agentCfg.ID, toolIDs)
}

// SeedIfEmpty 是系统路径（tenantID=”）：预设行写入为共享行。
func (s *ToolService) SeedIfEmpty() error {
	const sysTenant = ""
	tools, err := s.repo.ListAll(sysTenant)
	if err != nil {
		return fmt.Errorf("获取 Tool 列表失败: %w", err)
	}
	if len(tools) > 0 {
		return nil
	}

	// 预设行单源：presetToolSpecs（与 agent.PresetToolNames 对齐），此处只取
	// SeedIfEmpty 负责的六条（seedAlways=false）。
	for _, p := range presetToolSpecs {
		if p.seedAlways {
			continue
		}
		t := &agent.Tool{
			Name:        p.tool.Name,
			Title:       p.tool.Title,
			Description: p.tool.Description,
			Source:      agent.ToolSourceBuiltin,
		}
		if err := s.repo.Create(sysTenant, t); err != nil {
			return fmt.Errorf("创建预设 Tool '%s' 失败: %w", t.Name, err)
		}
	}

	return nil
}

func mergeStringSlices(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	for _, v := range base {
		seen[v] = true
	}
	for _, v := range extra {
		seen[v] = true
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// BackfillSubagentToolBindings ensures every agent that has at least one
// subagent is also bound to the built-in Task and MultiTask tools. This is
// intended to run once at startup, after SeedBuiltins, to migrate agents
// created before the auto-mount logic existed. The operation is idempotent:
// agents already correctly bound are left as-is (EnsureAgentToolBinding
// counts before insert).
//
// 启动回填无租户上下文，显式跨租户全量（ListAllUnscoped）；关联表按 agent_id
// 过滤，agent 行本身的租户归属在业务写路径（主表先校验）保证。
func (s *ToolService) BackfillSubagentToolBindings() error {
	agents, err := s.agentRepo.ListAllUnscoped()
	if err != nil {
		return fmt.Errorf("列出 Agent 失败: %w", err)
	}
	for _, a := range agents {
		subs, err := s.agentRepo.GetSubagents(a.ID)
		if err != nil {
			return fmt.Errorf("获取 Agent %s 的 subagent 失败: %w", a.Name, err)
		}
		if len(subs) == 0 {
			continue
		}
		if err := syncSubagentToolBindings(s.agentRepo, s.repo, a.ID, true); err != nil {
			return err
		}
	}
	return nil
}
