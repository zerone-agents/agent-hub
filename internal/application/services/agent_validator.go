package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	providerdomain "control-panel/internal/domain/provider"
	"control-panel/pkg/database"
)

var validPermissionModes = map[string]bool{
	"auto":              true,
	"plan":              true,
	"bypassPermissions": true,
}

// invalidNameChars mirrors deployer's naming.SanitizeName regex: any run of
// non-alphanumeric characters collapses to a single hyphen.
var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9]+`)

var validAgentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func ValidateAgentName(name string) error {
	if name == "" {
		return fmt.Errorf("Agent 标识不能为空")
	}
	if len(name) > 64 {
		return fmt.Errorf("Agent 标识长度不能超过 64 个字符")
	}
	if !validAgentNamePattern.MatchString(name) {
		return fmt.Errorf("Agent 标识只能包含小写字母、数字和连字符，必须以字母开头，连字符不能连续或出现在首尾")
	}
	return nil
}

// NormalizeAgentName reproduces deployer's naming.SanitizeName exactly.
// Deployer silently case-folds and hyphenates the agent identifier before
// writing it into agents.yaml, the docker container name, and the on-disk
// data directories. If control-panel round-trips the user's original
// spelling, every state-reconciliation call (GetStatus / Stop / Start /
// Delete) 404s because the runtime is registered under the sanitised name
// while control-panel keeps sending the original.
//
// Sanitisation rules (must stay in lockstep with deployer):
//   - any run of [^a-zA-Z0-9] → single "-"
//   - trim leading/trailing "-"
//   - lowercase
//
// Examples (must match deployer output bit-for-bit):
//
//	MyAgent    → myagent
//	my_agent   → my-agent
//	My.Agent   → my-agent
//	" agent "  → agent
//	my--agent  → my-agent
//
// Apply at every public service entry that crosses into deployer or
// runtime. MySQL's default collation is case-insensitive so internal
// storage and lookup keep working regardless; only the deployer boundary
// needs this. For new agents we also normalise before INSERT so the DB
// canonical key and the deployed key match.
func NormalizeAgentName(name string) string {
	s := invalidNameChars.ReplaceAllString(name, "-")
	s = strings.Trim(s, "-")
	return strings.ToLower(s)
}

func ValidateCreateConfig(config map[string]interface{}) error {
	if config == nil {
		return fmt.Errorf("config 不能为空")
	}
	if v, ok := config["systemPrompt"].(string); !ok || v == "" {
		return fmt.Errorf("systemPrompt 不能为空")
	}
	return ValidateConfig(config)
}

func ValidateConfig(config map[string]interface{}) error {
	if config == nil {
		return fmt.Errorf("config 不能为空")
	}

	if pm, ok := config["permissionMode"].(string); ok && pm != "" {
		if !validPermissionModes[pm] {
			return fmt.Errorf("无效的 permissionMode: %s，可选值: auto, plan, bypassPermissions", pm)
		}
	}

	// maxTurnsUpperBound is the shared upper limit for maxTurns; the frontend
	// form (AgentForm InputNumber) enforces the same value. 0 keeps the
	// runtime default, so only the negative range and values above the bound
	// are rejected.
	const maxTurnsUpperBound = 500
	if v, ok := config["maxTurns"].(float64); ok {
		if v < 0 {
			return fmt.Errorf("maxTurns 不能为负数")
		}
		if v > maxTurnsUpperBound {
			return fmt.Errorf("maxTurns 不能超过 %d", maxTurnsUpperBound)
		}
	}

	// disallowedTools（issue #111 agent-local deny list）: 形态校验全部
	// 收在 parseDisallowedTools 单一来源，与 unpackConfigToModel 共享，
	// 防两处规则漂移。
	if v, ok := config["disallowedTools"].([]interface{}); ok {
		if _, err := parseDisallowedTools(v); err != nil {
			return err
		}
	}

	if v, ok := config["icon"].(string); ok && len(v) > 512 {
		return fmt.Errorf("icon URL 长度不能超过 512 个字符")
	}

	if v, ok := config["iconName"].(string); ok && len(v) > 64 {
		return fmt.Errorf("iconName 长度不能超过 64 个字符")
	}

	if v, ok := config["iconColor"].(string); ok && len(v) > 32 {
		return fmt.Errorf("iconColor 长度不能超过 32 个字符")
	}

	if v, ok := config["iconBgColor"].(string); ok && len(v) > 64 {
		return fmt.Errorf("iconBgColor 长度不能超过 64 个字符")
	}

	// 模型绑定校验
	var providerID *uint64
	if v, ok := config["providerId"].(float64); ok {
		pid := uint64(v)
		providerID = &pid
	}

	var modelID string
	if v, ok := config["modelId"].(string); ok {
		modelID = v
	}
	var modelSelectionID string
	if v, ok := config["modelSelectionId"].(string); ok {
		modelSelectionID = v
	}

	if providerID != nil {
		if err := validateProviderModel(*providerID, modelID, modelSelectionID); err != nil {
			return err
		}
	}

	// fieldOverrides validation
	var fieldOverrides map[string]interface{}
	if v, ok := config["fieldOverrides"].(map[string]interface{}); ok {
		fieldOverrides = v
	}

	if len(fieldOverrides) > 0 {
		if providerID == nil {
			return fmt.Errorf("fieldOverrides 需要 providerId 同时存在")
		}
		if err := validateFieldOverridesKeys(*providerID, fieldOverrides); err != nil {
			return err
		}
	}

	return nil
}

// parseDisallowedTools is the single source of truth for the issue #111
// agent-local deny-list shape rules, shared by ValidateConfig (boundary
// enforcement) and unpackConfigToModel (normalization before store/deploy)
// so the two sites cannot drift. Entries are tool names (built-in like
// "Bash", MCP-qualified like "mcp__knowledge__lookup"); referential
// integrity against mounted tools is deliberately not checked. Each entry
// is trimmed because the runtime matches tool names exactly — shipping an
// un-trimmed entry would silently fail-open the deny list; trim-based
// emptiness and dedup follow from that. Check order is part of the
// contract (count first, then per item: string → non-blank → length →
// duplicate) since callers surface these errors to API clients. The
// returned list is trimmed and duplicate-free in input order; an absent
// or wrong-typed config key stays each caller's concern (both skip
// silently for legacy payloads).
func parseDisallowedTools(raw []interface{}) ([]string, error) {
	const (
		maxDisallowedTools     = 64
		maxDisallowedToolChars = 128
	)
	if len(raw) > maxDisallowedTools {
		return nil, fmt.Errorf("disallowedTools 条目数不能超过 %d", maxDisallowedTools)
	}
	seen := make(map[string]bool, len(raw))
	items := make([]string, 0, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("disallowedTools[%d] 必须是字符串", i)
		}
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return nil, fmt.Errorf("disallowedTools[%d] trim 后不能为空", i)
		}
		if len(trimmed) > maxDisallowedToolChars {
			return nil, fmt.Errorf("disallowedTools[%d] 长度不能超过 %d 个字符", i, maxDisallowedToolChars)
		}
		if seen[trimmed] {
			return nil, fmt.Errorf("disallowedTools[%d] 与其他条目重复：%s", i, trimmed)
		}
		seen[trimmed] = true
		items = append(items, trimmed)
	}
	return items, nil
}

// validateProviderModel checks that the referenced provider exists and,
// when a modelId is provided, that the bound model belongs to that
// provider and is of type 'llm' or 'vlm'. An agent can only bind to an
// LLM/VLM model; rejecting at the model level (rather than the provider's
// top-level type) prevents binding to e.g. an embedding model that happens
// to live under an LLM-class provider.
//
// When modelSelectionID is provided it takes precedence over modelID for
// the catalog lookup, since multiple catalog rows can share the same
// modelID (e.g. "k3" with 256K vs 1M contextWindow) and only selectionID
// disambiguates them.
//
// TODO: refactor to use a repository interface instead of database.DB global.
func validateProviderModel(providerID uint64, modelID, modelSelectionID string) error {
	if database.DB == nil {
		return nil
	}

	var exists int64
	if err := database.DB.Table("provider_summaries").
		Where("id = ?", providerID).Count(&exists).Error; err != nil {
		return fmt.Errorf("读取 provider 失败: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("providerId %d 不存在", providerID)
	}

	if modelID == "" && modelSelectionID == "" {
		return nil // modelId optional; nothing more to check
	}

	query := database.DB.Table("provider_models").
		Where("provider_id = ?", providerID)
	if modelSelectionID != "" {
		query = query.Where("selection_id = ?", modelSelectionID)
	} else {
		query = query.Where("model_id = ?", modelID)
	}

	var modelType string
	err := query.Select("model_type").Row().Scan(&modelType)
	if err == sql.ErrNoRows {
		if modelSelectionID != "" {
			return fmt.Errorf("providerId %d 下不存在 selection_id 为 %s 的模型", providerID, modelSelectionID)
		}
		return fmt.Errorf("providerId %d 下不存在模型 %s", providerID, modelID)
	}
	if err != nil {
		return fmt.Errorf("读取 provider_models 失败: %w", err)
	}
	if modelType != string(providerdomain.TypeLLM) && modelType != string(providerdomain.TypeVLM) {
		return fmt.Errorf("模型 %s 不是 LLM/VLM 类型（实际: %s），无法绑定到 Agent", modelID, modelType)
	}
	return nil
}

// validateFieldOverridesKeys checks that fieldOverrides keys are a subset of Provider.fields keys.
func validateFieldOverridesKeys(providerID uint64, overrides map[string]interface{}) error {
	if database.DB == nil {
		return nil
	}

	var fieldsJSON string
	err := database.DB.Table("provider_summaries").
		Where("id = ?", providerID).
		Select("fields").
		Row().Scan(&fieldsJSON)
	if err == sql.ErrNoRows {
		return fmt.Errorf("providerId %d 不存在，无法验证 fieldOverrides", providerID)
	}
	if err != nil {
		return fmt.Errorf("读取 Provider fields 失败: %w", err)
	}

	var fields []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return fmt.Errorf("解析 Provider fields 失败: %w", err)
	}

	allowedKeys := make(map[string]bool)
	for _, f := range fields {
		allowedKeys[f.Key] = true
	}

	for k := range overrides {
		if !allowedKeys[k] {
			return fmt.Errorf("fieldOverrides 包含非法 key: %s", k)
		}
	}

	return nil
}
