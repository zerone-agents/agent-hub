package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/provider"
	mcpprobe "control-panel/internal/infrastructure/mcp"
	repository "control-panel/internal/infrastructure/persistence"
)

type McpService struct {
	repo          *repository.McpRepository
	agentRepo     *repository.AgentRepository
	encryptionKey string
	probeClient   *mcpprobe.ProbeClient
}

func NewMcpService(encryptionKey string) *McpService {
	return &McpService{
		repo:          repository.NewMcpRepository(),
		agentRepo:     repository.NewAgentRepository(),
		encryptionKey: encryptionKey,
		probeClient:   mcpprobe.NewProbeClient(),
	}
}

// ==================== DTO ====================

type McpTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// McpDTO 是列表/概览返回的结构，不含敏感字段（headers 已加密存储不返回）。
type McpDTO struct {
	ID              uint64    `json:"id"`
	Name            string    `json:"name"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	TransportType   string    `json:"transportType"`
	URL             string    `json:"url,omitempty"`
	HasHeaders      bool      `json:"hasHeaders"`
	IsBuiltin       bool      `json:"isBuiltin"`
	Tools           []McpTool `json:"tools,omitempty"`
	ProbeStatus     string    `json:"probeStatus"`
	LastProbedAt    *string   `json:"lastProbedAt,omitempty"`
	RetryMaxRetries *int      `json:"retryMaxRetries"`
	RetryTimeoutMs  *int      `json:"retryTimeoutMs"`
	CreatedAt       string    `json:"createdAt"`
	UpdatedAt       string    `json:"updatedAt"`
}

// McpDetailDTO 是单条详情，含解密后的 headers（供编辑回填）。
type McpDetailDTO struct {
	McpDTO
	Headers map[string]string `json:"headers"`
}

// McpClientDTO 是客户端拉取接口的输出，符合 SDK 的 McpServerConfig 形态。
// 当前仅支持 sse / http，stdio 已移除。
type McpClientDTO struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	RetryPolicy *RetryPolicyDTO   `json:"retryPolicy,omitempty"`
	// Tools is deployment-only metadata. The runtime discovers schemas from
	// the MCP server itself, but Hub needs the probed names to extend the
	// agent's allowedTools whitelist with SDK-qualified MCP tool names.
	Tools []McpTool `json:"-"`
}

type RetryPolicyDTO struct {
	MaxRetries *int `json:"maxRetries,omitempty"`
	TimeoutMs  *int `json:"timeoutMs,omitempty"`
}

// ==================== Input ====================

type CreateMcpInput struct {
	Name            string            `json:"name" binding:"required"`
	Title           string            `json:"title" binding:"required"`
	Description     string            `json:"description"`
	TransportType   string            `json:"transportType" binding:"required"`
	URL             string            `json:"url"`
	Headers         map[string]string `json:"headers"`
	Tools           []McpTool         `json:"tools"`
	RetryMaxRetries *int              `json:"retryMaxRetries"`
	RetryTimeoutMs  *int              `json:"retryTimeoutMs"`
}

type UpdateMcpInput struct {
	Title           *string            `json:"title"`
	Description     *string            `json:"description"`
	TransportType   *string            `json:"transportType"`
	URL             *string            `json:"url"`
	Headers         *map[string]string `json:"headers"`
	RetryMaxRetries *int               `json:"retryMaxRetries"`
	RetryTimeoutMs  *int               `json:"retryTimeoutMs"`
}

type McpProbeInput struct {
	TransportType string            `json:"transportType" binding:"required"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
}

// ==================== 内部转换辅助 ====================

func validTransportType(t string) bool {
	return t == mcp.TransportSSE || t == mcp.TransportHTTP
}

// encryptMap 将 map 序列化为 JSON 并加密，空 map 返回空字符串（不存储）。
func (s *McpService) encryptMap(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("序列化失败: %w", err)
	}
	return provider.Encrypt(string(raw), s.encryptionKey)
}

// decryptMap 解密并反序列化为 map，空字符串返回空 map。
func (s *McpService) decryptMap(stored string) (map[string]string, error) {
	out := map[string]string{}
	if stored == "" {
		return out, nil
	}
	plain, err := provider.Decrypt(stored, s.encryptionKey)
	if err != nil {
		return nil, err
	}
	if plain == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(plain), &out); err != nil {
		return nil, fmt.Errorf("反序列化失败: %w", err)
	}
	return out, nil
}

func (s *McpService) toDTO(m *mcp.McpServer) *McpDTO {
	dto := &McpDTO{
		ID:              m.ID,
		Name:            m.Name,
		Title:           m.Title,
		Description:     m.Description,
		TransportType:   m.TransportType,
		URL:             m.URL,
		HasHeaders:      m.Headers != "",
		IsBuiltin:       m.IsBuiltin,
		ProbeStatus:     m.ProbeStatus,
		RetryMaxRetries: m.RetryMaxRetries,
		RetryTimeoutMs:  m.RetryTimeoutMs,
		CreatedAt:       m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       m.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if m.ToolsJSON != "" {
		var tools []McpTool
		if err := json.Unmarshal([]byte(m.ToolsJSON), &tools); err == nil {
			dto.Tools = tools
		}
	}

	if m.LastProbedAt != nil {
		formatted := m.LastProbedAt.UTC().Format(time.RFC3339)
		dto.LastProbedAt = &formatted
	}

	return dto
}

func (s *McpService) toDetailDTO(m *mcp.McpServer) (*McpDetailDTO, error) {
	headersMap, err := s.decryptMap(m.Headers)
	if err != nil {
		return nil, fmt.Errorf("解密 headers 失败: %w", err)
	}
	// Mask sensitive values; the frontend compares against these masked forms
	// to decide whether the user actually changed a value before saving.
	for k, v := range headersMap {
		if v != "" {
			headersMap[k] = maskSecret(v)
		}
	}
	return &McpDetailDTO{
		McpDTO:  *s.toDTO(m),
		Headers: headersMap,
	}, nil
}

func (s *McpService) toClientDTO(m *mcp.McpServer) (*McpClientDTO, error) {
	headersMap, err := s.decryptMap(m.Headers)
	if err != nil {
		return nil, fmt.Errorf("解密 headers 失败: %w", err)
	}

	dto := &McpClientDTO{
		Name:    m.Name,
		Type:    m.TransportType,
		URL:     m.URL,
		Headers: headersMap,
	}
	if m.ToolsJSON != "" {
		if err := json.Unmarshal([]byte(m.ToolsJSON), &dto.Tools); err != nil {
			return nil, fmt.Errorf("解析 MCP %q tools 失败: %w", m.Name, err)
		}
	}
	if m.RetryMaxRetries != nil || m.RetryTimeoutMs != nil {
		dto.RetryPolicy = &RetryPolicyDTO{
			MaxRetries: m.RetryMaxRetries,
			TimeoutMs:  m.RetryTimeoutMs,
		}
	}
	return dto, nil
}

// validateMcpConfig 校验 transport 与对应必填字段。
// 当前仅支持 sse / http。
func validateMcpConfig(transport, url string) error {
	if !validTransportType(transport) {
		return fmt.Errorf("transportType 必须是 sse / http 之一，当前: %q", transport)
	}
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("%s 类型必须填写 url", transport)
	}
	return nil
}

// ==================== Service 方法 ====================

func (s *McpService) ListAll(tenantID string) ([]*McpDTO, error) {
	items, err := s.repo.ListAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("获取 MCP 列表失败: %w", err)
	}
	dtos := make([]*McpDTO, 0, len(items))
	for _, m := range items {
		dtos = append(dtos, s.toDTO(m))
	}
	return dtos, nil
}

func (s *McpService) GetByName(tenantID, name string) (*McpDetailDTO, error) {
	m, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("MCP 不存在: %w", err)
	}
	return s.toDetailDTO(m)
}

func (s *McpService) Create(tenantID string, input *CreateMcpInput) (*McpDTO, error) {
	if err := validateMcpConfig(input.TransportType, input.URL); err != nil {
		return nil, err
	}

	exists, err := s.repo.ExistsByName(tenantID, input.Name)
	if err != nil {
		return nil, fmt.Errorf("检查 MCP 存在性失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("MCP '%s' 已存在", input.Name)
	}

	headersEnc, err := s.encryptMap(input.Headers)
	if err != nil {
		return nil, fmt.Errorf("加密 headers 失败: %w", err)
	}

	var toolsJSON string
	probeStatus := "pending"
	var lastProbedAt *time.Time
	if input.Tools != nil {
		raw, err := json.Marshal(input.Tools)
		if err != nil {
			return nil, fmt.Errorf("序列化 tools 失败: %w", err)
		}
		toolsJSON = string(raw)
		probeStatus = "success"
		now := time.Now()
		lastProbedAt = &now
	}

	m := &mcp.McpServer{
		Name:            input.Name,
		Title:           input.Title,
		Description:     input.Description,
		TransportType:   input.TransportType,
		URL:             input.URL,
		Headers:         headersEnc,
		ToolsJSON:       toolsJSON,
		ProbeStatus:     probeStatus,
		LastProbedAt:    lastProbedAt,
		IsBuiltin:       false,
		RetryMaxRetries: input.RetryMaxRetries,
		RetryTimeoutMs:  input.RetryTimeoutMs,
	}

	if err := s.repo.Create(tenantID, m); err != nil {
		return nil, fmt.Errorf("创建 MCP 失败: %w", err)
	}
	return s.toDTO(m), nil
}

func (s *McpService) Update(tenantID, name string, input *UpdateMcpInput) (*McpDTO, error) {
	m, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("MCP 不存在: %w", err)
	}

	if m.IsBuiltin {
		if input.TransportType != nil && *input.TransportType != m.TransportType {
			return nil, fmt.Errorf("内置 MCP 不可修改 transportType")
		}
	}

	if input.Title != nil {
		m.Title = *input.Title
	}
	if input.Description != nil {
		m.Description = *input.Description
	}
	if input.TransportType != nil {
		m.TransportType = *input.TransportType
	}
	if input.URL != nil {
		m.URL = *input.URL
	}
	if input.Headers != nil {
		// Merge with existing headers: values matching the masked form of the
		// stored value mean "unchanged", so keep the real value from DB.
		existingHeaders, _ := s.decryptMap(m.Headers)
		merged := make(map[string]string)
		for k, v := range *input.Headers {
			if existing, ok := existingHeaders[k]; ok && v == maskSecret(existing) {
				// User didn't change this value; keep existing
				merged[k] = existing
			} else {
				merged[k] = v
			}
		}
		enc, err := s.encryptMap(merged)
		if err != nil {
			return nil, fmt.Errorf("加密 headers 失败: %w", err)
		}
		m.Headers = enc
	}
	if input.RetryMaxRetries != nil {
		m.RetryMaxRetries = input.RetryMaxRetries
	}
	if input.RetryTimeoutMs != nil {
		m.RetryTimeoutMs = input.RetryTimeoutMs
	}

	if err := validateMcpConfig(m.TransportType, m.URL); err != nil {
		return nil, err
	}

	if err := s.repo.Update(tenantID, m); err != nil {
		return nil, fmt.Errorf("更新 MCP 失败: %w", err)
	}
	return s.toDTO(m), nil
}

func (s *McpService) Delete(tenantID, name string) error {
	m, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return fmt.Errorf("MCP '%s' 不存在", name)
	}
	if m.IsBuiltin {
		return fmt.Errorf("MCP '%s' 是内置服务，不可删除", name)
	}
	return s.repo.Delete(tenantID, m.ID)
}

// BuiltinKnowledgeAuthHeader is the Authorization header template seeded for the
// built-in "knowledge" MCP. The `$agent_runtime_token` placeholder is stored
// verbatim (not a real secret) and resolved into the deploying agent's runtime
// token by the deployer service at deploy time. See
// AgentDeployerService.resolveMcpHeaders.
const BuiltinKnowledgeAuthHeader = "Bearer $agent_runtime_token"

var builtinKnowledgeTools = []McpTool{
	{
		Name:        "knowledge_search",
		Description: "检索 Agent 已绑定的知识库，为文档问答提供相关文本片段",
	},
	{
		Name:        "knowledge_datasets",
		Description: "列出 Agent 绑定的知识库及实时元数据（文档数、分块数）",
	},
	{
		Name:        "knowledge_documents",
		Description: "分页列出知识库内文档（目录），仅返回元数据",
	},
	{
		Name:        "knowledge_chunks",
		Description: "按页读取文档分块原文，page/page_size 自控节奏",
	},
}

// BuiltinKnowledgeToolNames 返回内置 knowledge MCP 种子的工具名集合，
// 供 handler 层测试跨检 tools/list 广播与种子不漂移（两份手写副本）。
func BuiltinKnowledgeToolNames() []string {
	names := make([]string, 0, len(builtinKnowledgeTools))
	for _, t := range builtinKnowledgeTools {
		names = append(names, t.Name)
	}
	return names
}

func mustMarshalMcpTools(tools []McpTool) string {
	raw, err := json.Marshal(tools)
	if err != nil {
		panic(fmt.Sprintf("marshal builtin MCP tools: %v", err))
	}
	return string(raw)
}

// applyBuiltinMetadata refreshes system-owned fields without overwriting
// administrator-editable URL, headers, title, or description.
func applyBuiltinMetadata(existing, definition *mcp.McpServer) bool {
	changed := false
	// Shared built-ins cannot be edited through tenant-scoped APIs. Backfill an
	// empty URL from configuration so installations created before
	// KNOWLEDGE_MCP_URL was introduced become usable after a restart, while
	// preserving a non-empty URL explicitly stored by older versions.
	if strings.TrimSpace(existing.URL) == "" && strings.TrimSpace(definition.URL) != "" {
		existing.URL = strings.TrimSpace(definition.URL)
		changed = true
	}
	if !existing.IsBuiltin {
		existing.IsBuiltin = true
		changed = true
	}
	if existing.ToolsJSON != definition.ToolsJSON {
		existing.ToolsJSON = definition.ToolsJSON
		changed = true
	}
	if existing.ProbeStatus != "success" {
		existing.ProbeStatus = "success"
		changed = true
	}
	if existing.LastProbedAt != nil {
		existing.LastProbedAt = nil
		changed = true
	}
	return changed
}

// SeedBuiltins ensures built-in MCP servers exist in the database.
// It is idempotent and should be called once at service startup.
func (s *McpService) SeedBuiltins(knowledgeMCPURL ...string) error {
	mcpURL := ""
	if len(knowledgeMCPURL) > 0 {
		mcpURL = strings.TrimSpace(knowledgeMCPURL[0])
	}
	if mcpURL != "" {
		if err := validateMcpConfig(mcp.TransportHTTP, mcpURL); err != nil {
			return fmt.Errorf("invalid KNOWLEDGE_MCP_URL: %w", err)
		}
	}
	headersEnc, err := s.encryptMap(map[string]string{
		"Authorization": BuiltinKnowledgeAuthHeader,
	})
	if err != nil {
		return fmt.Errorf("encrypt builtin knowledge headers failed: %w", err)
	}
	if err := s.seedBuiltinMcp(&mcp.McpServer{
		Name:          "knowledge",
		Title:         "知识库检索",
		Description:   "基于知识库进行文本检索，为 Agent 提供文档问答能力",
		TransportType: mcp.TransportHTTP,
		URL:           mcpURL,
		Headers:       headersEnc,
		IsBuiltin:     true,
		ToolsJSON:     mustMarshalMcpTools(builtinKnowledgeTools),
		ProbeStatus:   "success",
	}); err != nil {
		return err
	}
	return nil
}

// seedBuiltinMcp 是系统路径（tenantID=”）：内置行写入/刷新为共享行。
func (s *McpService) seedBuiltinMcp(m *mcp.McpServer) error {
	const sysTenant = ""
	exists, err := s.repo.ExistsByName(sysTenant, m.Name)
	if err != nil {
		return fmt.Errorf("check builtin MCP %s failed: %w", m.Name, err)
	}
	if exists {
		existing, err := s.repo.GetByName(sysTenant, m.Name)
		if err != nil {
			return err
		}
		if applyBuiltinMetadata(existing, m) {
			if err := s.repo.Update(sysTenant, existing); err != nil {
				return fmt.Errorf("refresh builtin MCP %s failed: %w", m.Name, err)
			}
		}
		return nil
	}
	return s.repo.Create(sysTenant, m)
}

// ==================== Probe 探测 ====================

func (s *McpService) ProbeByConfig(ctx context.Context, input *McpProbeInput) (*mcpprobe.ProbeResult, error) {
	if err := validateMcpConfig(input.TransportType, input.URL); err != nil {
		return &mcpprobe.ProbeResult{Status: "failed", Error: err.Error()}, nil
	}
	result, err := s.probeClient.Probe(ctx, mcpprobe.ProbeConfig{
		TransportType: input.TransportType,
		URL:           input.URL,
		Headers:       input.Headers,
	})
	if err != nil {
		return &mcpprobe.ProbeResult{Status: "failed", Error: err.Error()}, nil
	}
	return result, nil
}

func (s *McpService) ProbeByName(ctx context.Context, tenantID, name string) (*mcpprobe.ProbeResult, error) {
	m, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("MCP 不存在: %w", err)
	}
	if m.IsBuiltin {
		var tools []mcpprobe.McpTool
		if err := json.Unmarshal([]byte(m.ToolsJSON), &tools); err != nil {
			return nil, fmt.Errorf("解析内置 MCP tools 失败: %w", err)
		}
		return &mcpprobe.ProbeResult{Status: "success", Tools: tools}, nil
	}
	headersMap, err := s.decryptMap(m.Headers)
	if err != nil {
		return nil, fmt.Errorf("解密 headers 失败: %w", err)
	}
	result, err := s.probeClient.Probe(ctx, mcpprobe.ProbeConfig{
		TransportType: m.TransportType,
		URL:           m.URL,
		Headers:       headersMap,
	})
	if err != nil {
		result = &mcpprobe.ProbeResult{Status: "failed", Error: err.Error()}
	}
	if err := s.saveProbeResult(tenantID, m, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *McpService) saveProbeResult(tenantID string, m *mcp.McpServer, result *mcpprobe.ProbeResult) error {
	m.ProbeStatus = result.Status
	now := time.Now().UTC()
	m.LastProbedAt = &now
	toolsJSON := ""
	if len(result.Tools) > 0 {
		raw, err := json.Marshal(result.Tools)
		if err != nil {
			return fmt.Errorf("序列化 tools 失败: %w", err)
		}
		toolsJSON = string(raw)
	}
	m.ToolsJSON = toolsJSON
	return s.repo.Update(tenantID, m)
}

// ==================== Agent ↔ MCP 绑定 ====================

func (s *McpService) GetAgentMcps(tenantID, agentName string) ([]string, error) {
	agentCfg, err := s.agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return nil, fmt.Errorf("Agent '%s' 不存在", agentName)
	}
	return s.repo.GetMcpNamesByAgent(agentCfg.ID)
}

func (s *McpService) UpdateAgentMcps(tenantID, agentName string, mcpNames []string) error {
	agentCfg, err := s.agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return fmt.Errorf("Agent '%s' 不存在", agentName)
	}

	mcpIDs := make([]uint64, 0, len(mcpNames))
	for _, mcpName := range mcpNames {
		m, err := s.repo.GetByName(tenantID, mcpName)
		if err != nil {
			return fmt.Errorf("MCP '%s' 不存在", mcpName)
		}
		mcpIDs = append(mcpIDs, m.ID)
	}
	return s.repo.ReplaceAgentMcps(agentCfg.ID, mcpIDs)
}

// ==================== 客户端拉取接口（公开） ====================

// GetClientMcpsByAgent 返回某 Agent 绑定的所有 MCP 配置（已解密，可直接喂给 SDK）。
func (s *McpService) GetClientMcpsByAgent(tenantID, agentName string) (map[string]*McpClientDTO, error) {
	agentCfg, err := s.agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return nil, fmt.Errorf("Agent '%s' 不存在", agentName)
	}
	items, err := s.repo.GetMcpServersByAgent(tenantID, agentCfg.ID)
	if err != nil {
		return nil, fmt.Errorf("查询 Agent MCP 失败: %w", err)
	}
	out := make(map[string]*McpClientDTO, len(items))
	for _, m := range items {
		dto, err := s.toClientDTO(m)
		if err != nil {
			return nil, err
		}
		out[m.Name] = dto
	}
	return out, nil
}

// ==================== Manifest 聚合（供 Agent Manifest 用） ====================

// GetAllAgentMcpNames 返回本租户内 agentName -> []mcpName 映射。
func (s *McpService) GetAllAgentMcpNames(tenantID string) (map[string][]string, error) {
	return s.repo.GetAllAgentMcpNames(tenantID)
}
