package services

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/provider"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"
	"log"
)

// AgentService provides business logic for managing agent configurations.
type AgentService struct {
	repo          *repository.AgentRepository
	toolRepo      *repository.ToolRepository
	skillRepo     *repository.SkillRepository
	mcpRepo       *repository.McpRepository
	providerSvc   *ProviderService
	encryptionKey string
}

// NewAgentService creates a new AgentService with default repositories.
func NewAgentService(encryptionKey string) *AgentService {
	return &AgentService{
		repo:          repository.NewAgentRepository(),
		toolRepo:      repository.NewToolRepository(),
		skillRepo:     repository.NewSkillRepository(),
		mcpRepo:       repository.NewMcpRepository(),
		providerSvc:   NewProviderService(encryptionKey),
		encryptionKey: encryptionKey,
	}
}

// CreateAgentInput holds the parameters for creating a new agent.
type CreateAgentInput struct {
	Name           string
	Config         map[string]interface{}
	DesktopEnabled *bool
	MobileEnabled  *bool
	IsDefault      *bool
}

// UpdateAgentInput holds the optional parameters for updating an existing agent.
type UpdateAgentInput struct {
	Config         *map[string]interface{}
	DesktopEnabled *bool
	MobileEnabled  *bool
	IsDefault      *bool
	Source         string
}

// ManifestDTO represents the agent manifest with content hashes for cache invalidation.
type ManifestDTO struct {
	Agents    []ManifestAgentDTO `json:"agents"`
	UpdatedAt string             `json:"updatedAt"`
}

// ManifestAgentDTO is a lightweight agent entry in the manifest.
type ManifestAgentDTO struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	ContentHash string `json:"contentHash"`
}

// AgentsDTO wraps a list of AgentDTO for API responses.
type AgentsDTO struct {
	Agents []AgentDTO `json:"agents"`
}

// AgentDTO is the full agent representation returned by the API.
type AgentDTO struct {
	ID             uint64                 `json:"id"`
	Name           string                 `json:"name"`
	ContentHash    string                 `json:"contentHash"`
	Config         map[string]interface{} `json:"config"`
	Subagents      []string               `json:"subagents"`
	Tools          []string               `json:"tools"`
	Skills         []string               `json:"skills"`
	Mcps           []string               `json:"mcps"`
	Datasets       []string               `json:"datasets"`
	DesktopEnabled bool                   `json:"desktopEnabled"`
	MobileEnabled  bool                   `json:"mobileEnabled"`
	IsDefault      bool                   `json:"isDefault"`
	Group          string                 `json:"group"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// GetManifest returns the agent manifest for the given client platform
// (agent.PlatformDesktop / agent.PlatformMobile; empty defaults to desktop),
// containing the agents enabled for that platform and their content hashes.
func (s *AgentService) GetManifest(platform string) (*ManifestDTO, error) {
	configs, err := s.repo.ListForPlatform(platform)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent 列表失败: %w", err)
	}

	var maxUpdatedAt time.Time
	agents := make([]ManifestAgentDTO, 0, len(configs))
	for _, cfg := range configs {
		agents = append(agents, ManifestAgentDTO{
			ID:          cfg.ID,
			Name:        cfg.Name,
			ContentHash: cfg.ContentHash,
		})
		if cfg.UpdatedAt.After(maxUpdatedAt) {
			maxUpdatedAt = cfg.UpdatedAt
		}
	}

	updatedAt := ""
	if !maxUpdatedAt.IsZero() {
		updatedAt = maxUpdatedAt.UTC().Format(time.RFC3339)
	}

	return &ManifestDTO{Agents: agents, UpdatedAt: updatedAt}, nil
}

// GetDesktopAgents returns all desktop-enabled agents with their full details.
func (s *AgentService) GetDesktopAgents() (*AgentsDTO, error) {
	configs, err := s.repo.ListForPlatform(agent.PlatformDesktop)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent 列表失败: %w", err)
	}

	return s.buildAgentsDTO(configs)
}

// GetAllAgentsAdmin returns all agents (including disabled) with their full details.
func (s *AgentService) GetAllAgentsAdmin() (*AgentsDTO, error) {
	configs, err := s.repo.ListAll()
	if err != nil {
		return nil, fmt.Errorf("获取 Agent 列表失败: %w", err)
	}

	return s.buildAgentsDTO(configs)
}

// buildAgentsDTO converts a list of agent configs into an AgentsDTO, enriching with subagent/tool/skill/mcp relations.
func (s *AgentService) buildAgentsDTO(configs []*agent.AgentConfig) (*AgentsDTO, error) {
	subagentsMap, err := s.repo.GetAllSubagents()
	if err != nil {
		return nil, fmt.Errorf("获取子 Agent 关系失败: %w", err)
	}

	toolsMap, err := s.toolRepo.GetAllAgentTools()
	if err != nil {
		return nil, fmt.Errorf("获取 Agent Tool 关系失败: %w", err)
	}

	skillsMap, err := s.skillRepo.GetAllAgentSkills()
	if err != nil {
		return nil, fmt.Errorf("获取 Agent Skill 关系失败: %w", err)
	}

	mcpsMap, err := s.mcpRepo.GetAllAgentMcpNames()
	if err != nil {
		return nil, fmt.Errorf("获取 Agent MCP 关系失败: %w", err)
	}

	datasetsMap, err := s.repo.GetAllAgentKnowledgeDatasetIDs()
	if err != nil {
		return nil, fmt.Errorf("获取 Agent 知识库绑定失败: %w", err)
	}

	agents := make([]AgentDTO, 0, len(configs))
	for _, cfg := range configs {
		subs := subagentsMap[cfg.Name]
		if subs == nil {
			subs = []string{}
		}
		toolIDs := toolsMap[cfg.Name]
		if toolIDs == nil {
			toolIDs = []string{}
		}
		skillIDs := skillsMap[cfg.Name]
		if skillIDs == nil {
			skillIDs = []string{}
		}
		mcpNames := mcpsMap[cfg.Name]
		if mcpNames == nil {
			mcpNames = []string{}
		}
		datasetIDs := datasetsMap[cfg.Name]
		if datasetIDs == nil {
			datasetIDs = []string{}
		}
		agents = append(agents, s.buildAgentDTO(cfg, subs, toolIDs, skillIDs, mcpNames, datasetIDs))
	}

	return &AgentsDTO{Agents: agents}, nil
}

// GetAgent returns a single agent by name with full details.
func (s *AgentService) GetAgent(name string) (*AgentDTO, error) {
	cfg, err := s.repo.GetByName(name)
	if err != nil {
		return nil, fmt.Errorf("Agent 不存在: %w", err)
	}

	subs, err := s.repo.GetSubagents(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("获取子 Agent 关系失败: %w", err)
	}
	if subs == nil {
		subs = []string{}
	}

	toolIDs, err := s.toolRepo.GetToolsByAgent(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent Tool 关系失败: %w", err)
	}
	if toolIDs == nil {
		toolIDs = []string{}
	}

	skillIDs, err := s.skillRepo.GetAgentSkills(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent Skill 关系失败: %w", err)
	}
	if skillIDs == nil {
		skillIDs = []string{}
	}

	mcpNames, err := s.mcpRepo.GetMcpNamesByAgent(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent MCP 关系失败: %w", err)
	}
	if mcpNames == nil {
		mcpNames = []string{}
	}

	datasetIDs, err := s.repo.GetKnowledgeDatasetIDsByAgent(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent 知识库绑定失败: %w", err)
	}
	if datasetIDs == nil {
		datasetIDs = []string{}
	}

	dto := s.buildAgentDTO(cfg, subs, toolIDs, skillIDs, mcpNames, datasetIDs)
	return &dto, nil
}

func (s *AgentService) ProbeAgent(name string, providerID *uint64, apiKey, baseURL string) (*ProbeResult, error) {
	a, err := s.repo.GetByName(name)
	if err != nil {
		return nil, fmt.Errorf("Agent 不存在: %w", err)
	}

	// Prefer explicit providerID from the request (supports testing before
	// saving the model binding); fall back to the agent's stored provider.
	resolvedProviderID := providerID
	if resolvedProviderID == nil {
		resolvedProviderID = a.ProviderID
	}
	if resolvedProviderID == nil {
		return nil, fmt.Errorf("Agent 未绑定 Provider")
	}

	p, err := s.providerSvc.repo.GetByID(*resolvedProviderID)
	if err != nil {
		return nil, fmt.Errorf("Provider 不存在: %w", err)
	}

	storedKey, err := provider.Decrypt(p.LockedAPIKey, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密 Provider API Key 失败: %w", err)
	}

	overrides := map[string]string{}
	if a.FieldOverrides != "" {
		overrides, _ = decryptFieldOverrides(a.FieldOverrides, *resolvedProviderID, s.encryptionKey)
	}
	overrideKey := overrides["api_key"]
	overrideBaseURL := overrides["base_url"]

	resolvedKey := apiKey
	if resolvedKey == "" {
		resolvedKey = overrideKey
		if resolvedKey == "" {
			resolvedKey = storedKey
		}
	} else if resolvedKey == maskSecret(overrideKey) {
		resolvedKey = overrideKey
	} else if resolvedKey == maskSecret(storedKey) {
		resolvedKey = storedKey
	}

	resolvedBaseURL := baseURL
	if resolvedBaseURL == "" {
		resolvedBaseURL = overrideBaseURL
		if resolvedBaseURL == "" {
			resolvedBaseURL = p.BaseURL
		}
	}

	// Load models from the normalized provider_models table (Task 4+).
	// The legacy default_models JSON column on provider_summaries was
	// dropped in Task 7.
	modelRows, err := s.providerSvc.repo.ListModels(*resolvedProviderID)
	if err != nil {
		return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
	}
	models := toCatalogModels(modelRows)

	return s.providerSvc.ProbeConfig(resolvedBaseURL, resolvedKey, p.Protocol, p.AuthStyle, models), nil
}

// buildAgentDTO converts a single agent config into an AgentDTO.
func (s *AgentService) buildAgentDTO(cfg *agent.AgentConfig, subs, tools, skills, mcps, datasets []string) AgentDTO {
	return AgentDTO{
		ID:             cfg.ID,
		Name:           cfg.Name,
		ContentHash:    cfg.ContentHash,
		Config:         modelToConfigMap(cfg, s.encryptionKey),
		Subagents:      subs,
		Tools:          tools,
		Skills:         skills,
		Mcps:           mcps,
		Datasets:       datasets,
		DesktopEnabled: cfg.DesktopEnabled,
		MobileEnabled:  cfg.MobileEnabled,
		IsDefault:      cfg.IsDefault,
		Group:          cfg.Group,
		CreatedAt:      cfg.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      cfg.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// CreateAgent validates, creates, and returns a new agent configuration.
func (s *AgentService) CreateAgent(input *CreateAgentInput) (*AgentDTO, error) {
	if err := ValidateAgentName(input.Name); err != nil {
		return nil, err
	}

	exists, err := s.repo.ExistsByName(input.Name)
	if err != nil {
		return nil, fmt.Errorf("检查 Agent 存在性失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("Agent '%s' 已存在", input.Name)
	}

	if err := ValidateCreateConfig(input.Config); err != nil {
		return nil, err
	}

	cfg := s.prepareCreateConfig(input)

	if cfg.IsDefault {
		if err := s.repo.ClearAllDefault(); err != nil {
			return nil, fmt.Errorf("清除默认 Agent 失败: %w", err)
		}
	}

	s.applyCreateDefaults(cfg)

	contentHash, err := computeContentHash(modelToConfigMap(cfg, s.encryptionKey))
	if err != nil {
		return nil, fmt.Errorf("计算内容哈希失败: %w", err)
	}
	cfg.ContentHash = contentHash

	if err := s.repo.Create(cfg); err != nil {
		return nil, fmt.Errorf("创建 Agent 失败: %w", err)
	}

	if err := s.toolRepo.BindDefaultToolsToAgent(cfg.ID); err != nil {
		return nil, fmt.Errorf("绑定默认 Tool 失败: %w", err)
	}

	return s.GetAgent(input.Name)
}

// prepareCreateConfig builds an AgentConfig from creation input with enabled/isDefault resolution.
func (s *AgentService) prepareCreateConfig(input *CreateAgentInput) *agent.AgentConfig {
	desktop := false
	if input.DesktopEnabled != nil {
		desktop = *input.DesktopEnabled
	}
	mobile := false
	if input.MobileEnabled != nil {
		mobile = *input.MobileEnabled
	}
	isDefault := false
	if input.IsDefault != nil {
		isDefault = *input.IsDefault
	}

	cfg := &agent.AgentConfig{
		Name:           input.Name,
		Source:         "remote",
		DesktopEnabled: desktop,
		MobileEnabled:  mobile,
		IsDefault:      isDefault,
	}
	unpackConfigToModel(input.Config, cfg, s.encryptionKey)
	return cfg
}

// applyCreateDefaults sets default values for permission mode and max turns.
func (s *AgentService) applyCreateDefaults(cfg *agent.AgentConfig) {
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = "auto"
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}
}

// UpdateAgent modifies an existing agent configuration by name.
func (s *AgentService) UpdateAgent(name string, input *UpdateAgentInput) (*AgentDTO, error) {
	cfg, err := s.repo.GetByName(name)
	if err != nil {
		return nil, fmt.Errorf("Agent 不存在: %w", err)
	}

	if err := s.applyUpdateConfig(cfg, input); err != nil {
		return nil, err
	}

	contentHash, err := computeContentHash(modelToConfigMap(cfg, s.encryptionKey))
	if err != nil {
		return nil, fmt.Errorf("计算内容哈希失败: %w", err)
	}
	cfg.ContentHash = contentHash

	if err := s.repo.Update(cfg); err != nil {
		return nil, fmt.Errorf("更新 Agent 失败: %w", err)
	}

	return s.GetAgent(name)
}

// applyUpdateConfig applies the update input fields to an existing agent config.
func (s *AgentService) applyUpdateConfig(cfg *agent.AgentConfig, input *UpdateAgentInput) error {
	if input.Config != nil {
		if err := ValidateConfig(*input.Config); err != nil {
			return err
		}
		unpackConfigToModel(*input.Config, cfg, s.encryptionKey)
	}

	if input.DesktopEnabled != nil {
		cfg.DesktopEnabled = *input.DesktopEnabled
	}
	if input.MobileEnabled != nil {
		cfg.MobileEnabled = *input.MobileEnabled
	}
	if input.IsDefault != nil {
		if err := s.handleDefaultUpdate(cfg.ID, *input.IsDefault); err != nil {
			return err
		}
		cfg.IsDefault = *input.IsDefault
	}
	if input.Source != "" {
		cfg.Source = input.Source
	}
	return nil
}

// handleDefaultUpdate clears other defaults when setting a new default agent.
func (s *AgentService) handleDefaultUpdate(agentID uint64, isDefault bool) error {
	if !isDefault {
		return nil
	}
	if err := s.repo.ClearDefaultExcept(agentID); err != nil {
		return fmt.Errorf("清除默认 Agent 失败: %w", err)
	}
	return nil
}

// DeleteAgent removes an agent configuration by name.
func (s *AgentService) DeleteAgent(name string) error {
	cfg, err := s.repo.GetByName(name)
	if err != nil {
		return fmt.Errorf("Agent '%s' 不存在", name)
	}

	return s.repo.Delete(cfg.ID)
}

// UpdateSubagents replaces the subagent list for the specified agent and
// automatically mounts/unmounts the built-in Task and MultiTask tools so that
// any agent with subagents gains the ability to dispatch work to them. This
// mirrors the Skill/Knowledge auto-mount patterns.
func (s *AgentService) UpdateSubagents(agentName string, subagentNames []string) error {
	cfg, err := s.repo.GetByName(agentName)
	if err != nil {
		return fmt.Errorf("Agent '%s' 不存在", agentName)
	}

	subagentIDs := make([]uint64, 0, len(subagentNames))
	for _, subName := range subagentNames {
		subCfg, err := s.repo.GetByName(subName)
		if err != nil {
			return fmt.Errorf("子 Agent '%s' 不存在", subName)
		}
		subagentIDs = append(subagentIDs, subCfg.ID)
	}

	for _, subName := range subagentNames {
		if subName == agentName {
			return fmt.Errorf("子 Agent 不能与主 Agent 相同")
		}
	}

	if err := s.repo.ReplaceSubagents(cfg.ID, subagentIDs); err != nil {
		return err
	}

	// Auto-mount/unmount Task + MultiTask based on whether the agent now has
	// any subagents. Idempotent (EnsureAgentToolBinding counts before insert).
	// s.toolRepo is the existing field on AgentService (see agent_service.go:21).
	return syncSubagentToolBindings(s.repo, s.toolRepo, cfg.ID, len(subagentNames) > 0)
}

func unpackConfigToModel(config map[string]interface{}, cfg *agent.AgentConfig, encryptionKey string) {
	if v, ok := config["systemPrompt"].(string); ok {
		cfg.SystemPrompt = v
	}
	if v, ok := config["permissionMode"].(string); ok {
		cfg.PermissionMode = v
	}
	if v, ok := config["maxTurns"].(float64); ok {
		cfg.MaxTurns = int(v)
	}
	if v, ok := config["icon"].(string); ok {
		cfg.Icon = v
	}
	if v, ok := config["iconName"].(string); ok {
		cfg.IconName = v
	}
	if v, ok := config["iconColor"].(string); ok {
		cfg.IconColor = v
	}
	if v, ok := config["iconBgColor"].(string); ok {
		cfg.IconBgColor = v
	}
	if v, ok := config["providerId"].(float64); ok {
		pid := uint64(v)
		cfg.ProviderID = &pid
	}
	if v, ok := config["modelId"].(string); ok {
		cfg.ModelID = v
	}
	if v, ok := config["title"].(map[string]interface{}); ok {
		cfg.Title = ifaceMapToStrMap(v)
	}
	if v, ok := config["description"].(map[string]interface{}); ok {
		cfg.Description = ifaceMapToStrMap(v)
	}

	// Handle group field
	if v, ok := config["group"].(string); ok {
		cfg.Group = v
	}

	// Handle maxSessionTurns field
	if v, ok := config["maxSessionTurns"].(float64); ok {
		n := int(v)
		cfg.MaxSessionTurns = &n
	}

	// Handle fieldOverrides
	if v, ok := config["fieldOverrides"].(map[string]interface{}); ok && len(v) > 0 {
		overrides := make(map[string]string)
		for k, val := range v {
			if s, ok := val.(string); ok {
				overrides[k] = s
			}
		}

		if cfg.ProviderID != nil && len(overrides) > 0 {
			encryptedJSON, err := encryptFieldOverrides(overrides, *cfg.ProviderID, encryptionKey)
			if err != nil {
				// Log and store plaintext; validator catches invalid
				jsonBytes, _ := json.Marshal(overrides)
				cfg.FieldOverrides = string(jsonBytes)
			} else {
				cfg.FieldOverrides = encryptedJSON
			}
		} else {
			jsonBytes, _ := json.Marshal(overrides)
			cfg.FieldOverrides = string(jsonBytes)
		}
	}
}

func modelToConfigMap(cfg *agent.AgentConfig, encryptionKey string) map[string]interface{} {
	m := map[string]interface{}{
		"systemPrompt":   cfg.SystemPrompt,
		"permissionMode": cfg.PermissionMode,
		"maxTurns":       cfg.MaxTurns,
		"icon":           cfg.Icon,
		"iconName":       cfg.IconName,
		"iconColor":      cfg.IconColor,
		"iconBgColor":    cfg.IconBgColor,
		"group":          cfg.Group,
	}

	if cfg.MaxSessionTurns != nil {
		m["maxSessionTurns"] = *cfg.MaxSessionTurns
	} else {
		m["maxSessionTurns"] = nil
	}

	if cfg.ProviderID != nil {
		m["providerId"] = *cfg.ProviderID
	} else {
		m["providerId"] = nil
	}
	m["modelId"] = cfg.ModelID

	// Handle fieldOverrides (decrypt secret fields, mask api_key for display)
	if cfg.FieldOverrides != "" && cfg.ProviderID != nil {
		overrides, err := decryptFieldOverrides(cfg.FieldOverrides, *cfg.ProviderID, encryptionKey)
		if err != nil {
			m["fieldOverrides"] = nil
		} else if overrides != nil {
			if v, ok := overrides["api_key"]; ok && v != "" {
				overrides["api_key"] = maskSecret(v)
			}
			m["fieldOverrides"] = overrides
		} else {
			m["fieldOverrides"] = nil
		}
	} else if cfg.FieldOverrides != "" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(cfg.FieldOverrides), &overrides); err == nil {
			if v, ok := overrides["api_key"]; ok && v != "" {
				overrides["api_key"] = maskSecret(v)
			}
			m["fieldOverrides"] = overrides
		} else {
			m["fieldOverrides"] = nil
		}
	} else {
		m["fieldOverrides"] = nil
	}

	if cfg.Title != nil {
		m["title"] = cfg.Title
	}
	if cfg.Description != nil {
		m["description"] = cfg.Description
	}

	return m
}

func ifaceMapToStrMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// encryptFieldOverrides encrypts secret fields in the overrides map.
// It queries the Provider's fields schema to determine which keys are secret.
func encryptFieldOverrides(overrides map[string]string, providerID uint64, encryptionKey string) (string, error) {
	if encryptionKey == "" {
		// No encryption configured, store as plaintext JSON
		jsonBytes, err := json.Marshal(overrides)
		if err != nil {
			return "", fmt.Errorf("序列化 fieldOverrides 失败: %w", err)
		}
		return string(jsonBytes), nil
	}

	// Query Provider's fields schema to find secret keys
	var fieldsJSON string
	err := database.DB.Table("provider_summaries").
		Where("id = ?", providerID).
		Select("fields").
		Row().Scan(&fieldsJSON)
	if err != nil {
		// Provider not found or query failed; store plaintext
		log.Printf("Warning: encryptFieldOverrides failed to query provider %d fields: %v, storing plaintext", providerID, err)
		jsonBytes, _ := json.Marshal(overrides)
		return string(jsonBytes), nil
	}

	var fields []provider.PresetField
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		log.Printf("Warning: encryptFieldOverrides failed to parse fields for provider %d: %v, storing plaintext", providerID, err)
		jsonBytes, _ := json.Marshal(overrides)
		return string(jsonBytes), nil
	}

	secretKeys := make(map[string]bool)
	for _, f := range fields {
		if f.Secret {
			secretKeys[f.Key] = true
		}
	}

	// Encrypt secret fields
	encrypted := make(map[string]string)
	for k, v := range overrides {
		if secretKeys[k] && v != "" {
			encVal, err := provider.Encrypt(v, encryptionKey)
			if err != nil {
				return "", fmt.Errorf("加密字段 %s 失败: %w", k, err)
			}
			encrypted[k] = encVal
		} else {
			encrypted[k] = v
		}
	}

	jsonBytes, err := json.Marshal(encrypted)
	if err != nil {
		return "", fmt.Errorf("序列化加密后的 fieldOverrides 失败: %w", err)
	}
	return string(jsonBytes), nil
}

// decryptFieldOverrides decrypts secret fields in the stored JSON.
func decryptFieldOverrides(storedJSON string, providerID uint64, encryptionKey string) (map[string]string, error) {
	if storedJSON == "" {
		return nil, nil
	}

	var overrides map[string]string
	if err := json.Unmarshal([]byte(storedJSON), &overrides); err != nil {
		return nil, fmt.Errorf("解析 fieldOverrides 失败: %w", err)
	}

	if encryptionKey == "" {
		return overrides, nil
	}

	// Query Provider's fields schema
	var fieldsJSON string
	err := database.DB.Table("provider_summaries").
		Where("id = ?", providerID).
		Select("fields").
		Row().Scan(&fieldsJSON)
	if err != nil {
		return overrides, nil
	}

	var fields []provider.PresetField
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return overrides, nil
	}

	secretKeys := make(map[string]bool)
	for _, f := range fields {
		if f.Secret {
			secretKeys[f.Key] = true
		}
	}

	// Decrypt secret fields
	for k, v := range overrides {
		if secretKeys[k] && strings.HasPrefix(v, "enc:") {
			decVal, err := provider.Decrypt(v, encryptionKey)
			if err != nil {
				log.Printf("Warning: decryptFieldOverrides failed to decrypt field %s for provider %d: %v", k, providerID, err)
				overrides[k] = ""
				continue
			}
			overrides[k] = decVal
		}
	}

	return overrides, nil
}

func computeContentHash(config map[string]interface{}) (string, error) {
	canonical, err := canonicalJSON(config)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(canonical)
	return fmt.Sprintf("sha256:%x", hash), nil
}

func canonicalJSON(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		parts := make([]string, 0, len(val))
		for _, k := range keys {
			childJSON, err := canonicalJSON(val[k])
			if err != nil {
				return nil, err
			}
			keyBytes, _ := json.Marshal(k)
			parts = append(parts, string(keyBytes)+":"+string(childJSON))
		}
		return []byte("{" + strings.Join(parts, ",") + "}"), nil

	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			childJSON, err := canonicalJSON(item)
			if err != nil {
				return nil, err
			}
			parts = append(parts, string(childJSON))
		}
		return []byte("[" + strings.Join(parts, ",") + "]"), nil

	default:
		return json.Marshal(val)
	}
}

// GetAgentKnowledgeDatasets returns the dataset IDs bound to an agent.
func (s *AgentService) GetAgentKnowledgeDatasets(agentName string) ([]string, error) {
	agentCfg, err := s.repo.GetByName(agentName)
	if err != nil {
		return nil, fmt.Errorf("Agent '%s' 不存在: %w", agentName, err)
	}
	return s.repo.GetKnowledgeDatasetIDsByAgent(agentCfg.ID)
}

// UpdateAgentKnowledgeDatasets replaces the dataset bindings for an agent and
// automatically enables/disables the built-in 'knowledge' MCP binding.
func (s *AgentService) UpdateAgentKnowledgeDatasets(agentName string, datasetIDs []string) error {
	agentCfg, err := s.repo.GetByName(agentName)
	if err != nil {
		return fmt.Errorf("Agent '%s' 不存在: %w", agentName, err)
	}

	cleaned := make([]string, 0, len(datasetIDs))
	for _, id := range datasetIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			cleaned = append(cleaned, id)
		}
	}

	if err := s.repo.ReplaceAgentKnowledgeDatasets(agentCfg.ID, cleaned); err != nil {
		return fmt.Errorf("替换 Agent knowledge dataset 失败: %w", err)
	}

	knowledgeMcp, err := s.mcpRepo.GetByName("knowledge")
	if err != nil {
		return fmt.Errorf("内置 MCP 'knowledge' 不存在: %w", err)
	}

	if len(cleaned) > 0 {
		return s.repo.EnsureAgentMcpBinding(agentCfg.ID, knowledgeMcp.ID)
	}
	return s.repo.RemoveAgentMcpBinding(agentCfg.ID, knowledgeMcp.ID)
}

// subagentToolNames lists the built-in tools that must be bound to an agent
// whenever it has one or more subagents. Names must match the rows seeded by
// ToolService.SeedBuiltins and the tool names exposed by agent-sdk.
var subagentToolNames = []string{"Task", "MultiTask"}

// syncSubagentToolBindings mirrors the Skill/Knowledge auto-mount pattern:
// when hasSubagents is true, EnsureAgentToolBinding is called for each name in
// subagentToolNames; when false, RemoveAgentToolBinding is called for each.
// All operations are idempotent at the repository layer.
func syncSubagentToolBindings(
	agentRepo *repository.AgentRepository,
	toolRepo *repository.ToolRepository,
	agentID uint64,
	hasSubagents bool,
) error {
	for _, name := range subagentToolNames {
		t, err := toolRepo.GetByName(name)
		if err != nil {
			return fmt.Errorf("内置 %s tool 不存在: %w", name, err)
		}
		if hasSubagents {
			if err := agentRepo.EnsureAgentToolBinding(agentID, t.ID); err != nil {
				return fmt.Errorf("挂载 %s tool 失败: %w", name, err)
			}
		} else {
			if err := agentRepo.RemoveAgentToolBinding(agentID, t.ID); err != nil {
				return fmt.Errorf("卸载 %s tool 失败: %w", name, err)
			}
		}
	}
	return nil
}
