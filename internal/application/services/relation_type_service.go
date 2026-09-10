package services

import (
	"errors"
	"fmt"
	"strings"

	"control-panel/internal/domain/agentrelation"
	repository "control-panel/internal/infrastructure/persistence"
	"gorm.io/gorm"
)

var ErrRelationTypeInUse = errors.New("关系类型仍被关系使用")

type RelationTypeService struct {
	repo *repository.RelationTypeRepository
}

func NewRelationTypeService() *RelationTypeService {
	return &RelationTypeService{repo: repository.NewRelationTypeRepository()}
}

type RelationTypeInput struct {
	Name, Title, Description, BaseType, DirectionPolicy, DefaultStance                   string
	DefaultAllowedActions                                                                []string
	DefaultContextPolicy, DefaultDeliveryPolicy, DefaultConstraint, LineColor, LineStyle string
	Enabled                                                                              *bool
}

type RelationTypeDTO struct {
	*agentrelation.RelationTypeTemplate
	UsageCount int64 `json:"usageCount"`
}

func (s *RelationTypeService) List(tenantID string) ([]*RelationTypeDTO, error) {
	if err := s.seed(tenantID); err != nil {
		return nil, err
	}
	rows, err := s.repo.List(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]*RelationTypeDTO, 0, len(rows))
	for _, row := range rows {
		n, _ := s.repo.CountUsage(tenantID, row.Name)
		out = append(out, &RelationTypeDTO{row, n})
	}
	return out, nil
}

func (s *RelationTypeService) Create(tenantID string, in *RelationTypeInput) (*RelationTypeDTO, error) {
	if err := validateRelationType(in); err != nil {
		return nil, err
	}
	if _, err := s.repo.Get(tenantID, in.Name); err == nil {
		return nil, fmt.Errorf("关系类型已存在")
	}
	row := relationTypeFromInput(in)
	row.CurrentVersion = 1
	if err := s.repo.Create(tenantID, row, &agentrelation.RelationTypeVersion{Version: 1, Snapshot: relationTypeSnapshot(row), ChangeNote: "创建关系类型"}); err != nil {
		return nil, err
	}
	return &RelationTypeDTO{RelationTypeTemplate: row}, nil
}

func (s *RelationTypeService) Update(tenantID, name string, in *RelationTypeInput) (*RelationTypeDTO, error) {
	if err := validateRelationType(in); err != nil {
		return nil, err
	}
	row, err := s.repo.Get(tenantID, name)
	if err != nil {
		return nil, err
	}
	updated := relationTypeFromInput(in)
	id, rowTenant, created, builtin, version := row.ID, row.TenantID, row.CreatedAt, row.IsBuiltin, row.CurrentVersion
	*row = *updated
	row.ID, row.TenantID, row.CreatedAt, row.IsBuiltin, row.CurrentVersion = id, rowTenant, created, builtin, version+1
	if err := s.repo.Update(tenantID, row, &agentrelation.RelationTypeVersion{Version: row.CurrentVersion, Snapshot: relationTypeSnapshot(row), ChangeNote: "更新关系类型"}); err != nil {
		return nil, err
	}
	n, _ := s.repo.CountUsage(tenantID, name)
	return &RelationTypeDTO{row, n}, nil
}

func (s *RelationTypeService) Delete(tenantID, name string) error {
	row, err := s.repo.Get(tenantID, name)
	if err != nil {
		return err
	}
	if row.IsBuiltin {
		return fmt.Errorf("系统内置关系类型不能删除")
	}
	n, _ := s.repo.CountUsage(tenantID, name)
	if n > 0 {
		return ErrRelationTypeInUse
	}
	return s.repo.Delete(tenantID, row)
}

func validateRelationType(in *RelationTypeInput) error {
	if err := validateIdentifier("关系类型", in.Name); err != nil {
		return err
	}
	if strings.TrimSpace(in.Title) == "" {
		return fmt.Errorf("显示名称不能为空")
	}
	if _, ok := agentrelation.RelationTypes[in.BaseType]; !ok {
		return fmt.Errorf("基础协议无效")
	}
	if in.DirectionPolicy != "one_way" && in.DirectionPolicy != "bidirectional_allowed" {
		return fmt.Errorf("方向策略无效")
	}
	if _, ok := agentrelation.Stances[in.DefaultStance]; !ok {
		return fmt.Errorf("默认立场无效")
	}
	for _, a := range in.DefaultAllowedActions {
		if _, ok := agentrelation.Actions[a]; !ok {
			return fmt.Errorf("动作 %s 无效", a)
		}
	}
	if _, ok := agentrelation.ContextPolicies[in.DefaultContextPolicy]; !ok {
		return fmt.Errorf("上下文策略无效")
	}
	if _, ok := agentrelation.DeliveryPolicies[in.DefaultDeliveryPolicy]; !ok {
		return fmt.Errorf("投递策略无效")
	}
	return nil
}
func relationTypeFromInput(in *RelationTypeInput) *agentrelation.RelationTypeTemplate {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return &agentrelation.RelationTypeTemplate{Name: strings.TrimSpace(in.Name), Title: strings.TrimSpace(in.Title), Description: strings.TrimSpace(in.Description), BaseType: in.BaseType, DirectionPolicy: in.DirectionPolicy, DefaultStance: in.DefaultStance, DefaultAllowedActions: in.DefaultAllowedActions, DefaultContextPolicy: in.DefaultContextPolicy, DefaultDeliveryPolicy: in.DefaultDeliveryPolicy, DefaultConstraint: strings.TrimSpace(in.DefaultConstraint), LineColor: in.LineColor, LineStyle: in.LineStyle, Enabled: enabled}
}
func relationTypeSnapshot(r *agentrelation.RelationTypeTemplate) map[string]any {
	return map[string]any{"title": r.Title, "baseType": r.BaseType, "directionPolicy": r.DirectionPolicy, "stance": r.DefaultStance, "actions": r.DefaultAllowedActions, "contextPolicy": r.DefaultContextPolicy, "deliveryPolicy": r.DefaultDeliveryPolicy, "constraint": r.DefaultConstraint, "lineColor": r.LineColor, "lineStyle": r.LineStyle}
}

func (s *RelationTypeService) seed(tenantID string) error {
	seeds := []RelationTypeInput{{Name: "reports-to", Title: "上下级汇报", BaseType: "reports_to", DirectionPolicy: "one_way", DefaultStance: "neutral", DefaultAllowedActions: []string{"inform", "report", "submit", "escalate"}, DefaultContextPolicy: "summary_only", DefaultDeliveryPolicy: "async", LineColor: "#2f6fbd", LineStyle: "solid"}, {Name: "peer-collaboration", Title: "同事协作", BaseType: "peer", DirectionPolicy: "bidirectional_allowed", DefaultStance: "friendly", DefaultAllowedActions: []string{"inform", "consult", "handoff"}, DefaultContextPolicy: "summary_only", DefaultDeliveryPolicy: "async", LineColor: "#2e8b78", LineStyle: "solid"}, {Name: "opponent", Title: "竞争对手", BaseType: "opponent", DirectionPolicy: "bidirectional_allowed", DefaultStance: "competitive", DefaultAllowedActions: []string{"inform", "challenge"}, DefaultContextPolicy: "none", DefaultDeliveryPolicy: "async", LineColor: "#c75b4b", LineStyle: "dashed"}}
	for _, in := range seeds {
		_, err := s.repo.Get(tenantID, in.Name)
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := relationTypeFromInput(&in)
		row.CurrentVersion = 1
		row.IsBuiltin = true
		if err := s.repo.Create(tenantID, row, &agentrelation.RelationTypeVersion{Version: 1, Snapshot: relationTypeSnapshot(row), ChangeNote: "系统初始版本"}); err != nil {
			return err
		}
	}
	return nil
}
