package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"control-panel/internal/domain/agentrelation"
	repository "control-panel/internal/infrastructure/persistence"

	"gorm.io/gorm"
)

var relationScopePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

type AgentRelationService struct {
	repo      *repository.AgentRelationRepository
	agentRepo *repository.AgentRepository
}

func NewAgentRelationService() *AgentRelationService {
	return &AgentRelationService{
		repo:      repository.NewAgentRelationRepository(),
		agentRepo: repository.NewAgentRepository(),
	}
}

type CreateAgentRelationInput struct {
	SourceAgentID  uint64
	TargetAgentID  uint64
	Scope          string
	RelationType   string
	Stance         string
	AllowedActions []string
	ContextPolicy  string
	DeliveryPolicy string
	Constraint     string
	Enabled        bool
	Bidirectional  bool
}

type UpdateAgentRelationInput struct {
	Scope          *string
	RelationType   *string
	Stance         *string
	AllowedActions *[]string
	ContextPolicy  *string
	DeliveryPolicy *string
	Constraint     *string
	Enabled        *bool
}

type AgentRelationDTO struct {
	ID              uint64   `json:"id"`
	Scope           string   `json:"scope"`
	SourceAgentID   uint64   `json:"sourceAgentId"`
	SourceAgentName string   `json:"sourceAgentName"`
	TargetAgentID   uint64   `json:"targetAgentId"`
	TargetAgentName string   `json:"targetAgentName"`
	RelationType    string   `json:"relationType"`
	Stance          string   `json:"stance"`
	AllowedActions  []string `json:"allowedActions"`
	ContextPolicy   string   `json:"contextPolicy"`
	DeliveryPolicy  string   `json:"deliveryPolicy"`
	Constraint      string   `json:"constraint"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

func (s *AgentRelationService) List(tenantID string) ([]*AgentRelationDTO, error) {
	relations, err := s.repo.ListAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("获取 Agent 关系列表失败: %w", err)
	}
	result := make([]*AgentRelationDTO, 0, len(relations))
	for _, relation := range relations {
		result = append(result, relationToDTO(relation))
	}
	return result, nil
}

func (s *AgentRelationService) Create(tenantID string, input *CreateAgentRelationInput) ([]*AgentRelationDTO, error) {
	normalizeCreateRelation(input)
	if err := validateRelation(input.SourceAgentID, input.TargetAgentID, input.Scope, input.RelationType, input.Stance, input.AllowedActions, input.ContextPolicy, input.DeliveryPolicy, input.Constraint); err != nil {
		return nil, err
	}
	if input.Bidirectional {
		if _, ok := agentrelation.BidirectionalRelationTypes[input.RelationType]; !ok {
			return nil, agentrelation.ErrBidirectionalType
		}
	}
	if err := s.ensureAgentsExist(tenantID, input.SourceAgentID, input.TargetAgentID); err != nil {
		return nil, err
	}

	edges := [][2]uint64{{input.SourceAgentID, input.TargetAgentID}}
	if input.Bidirectional {
		edges = append(edges, [2]uint64{input.TargetAgentID, input.SourceAgentID})
	}
	for _, edge := range edges {
		exists, err := s.repo.ExistsEdge(tenantID, input.Scope, edge[0], edge[1])
		if err != nil {
			return nil, fmt.Errorf("检查 Agent 关系失败: %w", err)
		}
		if exists {
			return nil, agentrelation.ErrAlreadyExists
		}
	}

	relations := make([]*agentrelation.AgentRelation, 0, len(edges))
	for _, edge := range edges {
		relations = append(relations, &agentrelation.AgentRelation{
			Scope:          input.Scope,
			SourceAgentID:  edge[0],
			TargetAgentID:  edge[1],
			RelationType:   input.RelationType,
			Stance:         input.Stance,
			AllowedActions: append([]string(nil), input.AllowedActions...),
			ContextPolicy:  input.ContextPolicy,
			DeliveryPolicy: input.DeliveryPolicy,
			Constraint:     input.Constraint,
			Enabled:        input.Enabled,
		})
	}
	if err := s.repo.CreateMany(tenantID, relations); err != nil {
		return nil, fmt.Errorf("创建 Agent 关系失败: %w", err)
	}

	result := make([]*AgentRelationDTO, 0, len(relations))
	for _, relation := range relations {
		created, err := s.repo.GetByID(tenantID, relation.ID)
		if err != nil {
			return nil, fmt.Errorf("读取新建 Agent 关系失败: %w", err)
		}
		result = append(result, relationToDTO(created))
	}
	return result, nil
}

func (s *AgentRelationService) Update(tenantID string, id uint64, input *UpdateAgentRelationInput) (*AgentRelationDTO, error) {
	relation, err := s.repo.GetByID(tenantID, id)
	if err != nil {
		return nil, agentrelation.ErrNotFound
	}

	if input.Scope != nil {
		relation.Scope = strings.TrimSpace(*input.Scope)
	}
	if input.RelationType != nil {
		relation.RelationType = strings.TrimSpace(*input.RelationType)
	}
	if input.Stance != nil {
		relation.Stance = strings.TrimSpace(*input.Stance)
	}
	if input.AllowedActions != nil {
		relation.AllowedActions = normalizeActions(*input.AllowedActions)
	}
	if input.ContextPolicy != nil {
		relation.ContextPolicy = strings.TrimSpace(*input.ContextPolicy)
	}
	if input.DeliveryPolicy != nil {
		relation.DeliveryPolicy = strings.TrimSpace(*input.DeliveryPolicy)
	}
	if input.Constraint != nil {
		relation.Constraint = strings.TrimSpace(*input.Constraint)
	}
	if input.Enabled != nil {
		relation.Enabled = *input.Enabled
	}

	if err := validateRelation(relation.SourceAgentID, relation.TargetAgentID, relation.Scope, relation.RelationType, relation.Stance, relation.AllowedActions, relation.ContextPolicy, relation.DeliveryPolicy, relation.Constraint); err != nil {
		return nil, err
	}

	relations, err := s.repo.ListAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("检查 Agent 关系失败: %w", err)
	}
	for _, candidate := range relations {
		if candidate.ID != relation.ID && candidate.Scope == relation.Scope && candidate.SourceAgentID == relation.SourceAgentID && candidate.TargetAgentID == relation.TargetAgentID {
			return nil, agentrelation.ErrAlreadyExists
		}
	}

	if err := s.repo.Update(tenantID, relation); err != nil {
		return nil, fmt.Errorf("更新 Agent 关系失败: %w", err)
	}
	updated, err := s.repo.GetByID(tenantID, relation.ID)
	if err != nil {
		return nil, fmt.Errorf("读取更新后的 Agent 关系失败: %w", err)
	}
	return relationToDTO(updated), nil
}

func (s *AgentRelationService) Delete(tenantID string, id uint64) error {
	if err := s.repo.Delete(tenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agentrelation.ErrNotFound
		}
		return fmt.Errorf("删除 Agent 关系失败: %w", err)
	}
	return nil
}

func (s *AgentRelationService) ensureAgentsExist(tenantID string, sourceAgentID, targetAgentID uint64) error {
	for _, id := range []uint64{sourceAgentID, targetAgentID} {
		exists, err := s.agentRepo.Exists(tenantID, id)
		if err != nil {
			return fmt.Errorf("检查 Agent 存在性失败: %w", err)
		}
		if !exists {
			return agentrelation.ErrAgentNotFound
		}
	}
	return nil
}

func normalizeCreateRelation(input *CreateAgentRelationInput) {
	input.Scope = strings.TrimSpace(input.Scope)
	if input.Scope == "" {
		input.Scope = agentrelation.DefaultScope
	}
	input.RelationType = strings.TrimSpace(input.RelationType)
	input.Stance = strings.TrimSpace(input.Stance)
	if input.Stance == "" {
		input.Stance = "neutral"
	}
	input.AllowedActions = normalizeActions(input.AllowedActions)
	input.ContextPolicy = strings.TrimSpace(input.ContextPolicy)
	if input.ContextPolicy == "" {
		input.ContextPolicy = agentrelation.DefaultContextPolicy
	}
	input.DeliveryPolicy = strings.TrimSpace(input.DeliveryPolicy)
	if input.DeliveryPolicy == "" {
		input.DeliveryPolicy = agentrelation.DefaultDeliveryPolicy
	}
	input.Constraint = strings.TrimSpace(input.Constraint)
}

func normalizeActions(actions []string) []string {
	result := make([]string, 0, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		result = append(result, action)
	}
	return result
}

func validateRelation(sourceAgentID, targetAgentID uint64, scope, relationType, stance string, actions []string, contextPolicy, deliveryPolicy, constraint string) error {
	if sourceAgentID == targetAgentID {
		return agentrelation.ErrSelfRelation
	}
	if !relationScopePattern.MatchString(scope) {
		return agentrelation.ErrInvalidScope
	}
	if _, ok := agentrelation.RelationTypes[relationType]; !ok {
		return agentrelation.ErrInvalidType
	}
	if _, ok := agentrelation.Stances[stance]; !ok {
		return agentrelation.ErrInvalidStance
	}
	if len(actions) == 0 {
		return agentrelation.ErrActionsRequired
	}
	for _, action := range actions {
		if _, ok := agentrelation.Actions[action]; !ok {
			return fmt.Errorf("%w: %s", agentrelation.ErrInvalidAction, action)
		}
	}
	if _, ok := agentrelation.ContextPolicies[contextPolicy]; !ok {
		return agentrelation.ErrInvalidContext
	}
	if _, ok := agentrelation.DeliveryPolicies[deliveryPolicy]; !ok {
		return agentrelation.ErrInvalidDelivery
	}
	if utf8.RuneCountInString(constraint) > 2000 {
		return agentrelation.ErrConstraintTooLong
	}
	return nil
}

func relationToDTO(relation *agentrelation.AgentRelation) *AgentRelationDTO {
	return &AgentRelationDTO{
		ID:              relation.ID,
		Scope:           relation.Scope,
		SourceAgentID:   relation.SourceAgentID,
		SourceAgentName: relation.SourceAgent.Name,
		TargetAgentID:   relation.TargetAgentID,
		TargetAgentName: relation.TargetAgent.Name,
		RelationType:    relation.RelationType,
		Stance:          relation.Stance,
		AllowedActions:  append([]string(nil), relation.AllowedActions...),
		ContextPolicy:   relation.ContextPolicy,
		DeliveryPolicy:  relation.DeliveryPolicy,
		Constraint:      relation.Constraint,
		Enabled:         relation.Enabled,
		CreatedAt:       relation.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       relation.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
