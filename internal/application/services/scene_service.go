package services

import (
	"fmt"

	"control-panel/internal/domain/scene"
	repository "control-panel/internal/infrastructure/persistence"
)

// SceneService provides business logic for managing scenes.
type SceneService struct {
	repo      *repository.SceneRepository
	agentRepo *repository.AgentRepository
}

// NewSceneService creates a new SceneService with default repositories.
func NewSceneService() *SceneService {
	return &SceneService{
		repo:      repository.NewSceneRepository(),
		agentRepo: repository.NewAgentRepository(),
	}
}

// CreateSceneInput holds the parameters for creating a new scene.
type CreateSceneInput struct {
	Name     string
	AgentID  uint64
	Title    string
	TitleEn  string
	Prompt   string
	PromptEn string
}

// UpdateSceneInput holds the optional parameters for updating a scene.
type UpdateSceneInput struct {
	AgentID  *uint64
	Title    string
	TitleEn  string
	Prompt   string
	PromptEn string
	Enabled  *bool
}

// SceneDTO is the scene representation returned by the API.
type SceneDTO struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Agent     string `json:"agent"`
	Title     string `json:"title"`
	TitleEn   string `json:"titleEn"`
	Prompt    string `json:"prompt"`
	PromptEn  string `json:"promptEn"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// List returns scenes, optionally filtered by agent ID.
// tenantID 仅用于解析关联 agent 的展示名（agent 表已租户隔离）。
func (s *SceneService) List(tenantID string, agentID uint64) ([]*SceneDTO, error) {
	var scenes []*scene.Scene
	var err error

	if agentID > 0 {
		scenes, err = s.repo.ListByAgent(tenantID, agentID)
	} else {
		scenes, err = s.repo.ListAll(tenantID)
	}

	if err != nil {
		return nil, fmt.Errorf("获取场景列表失败: %w", err)
	}

	result := make([]*SceneDTO, 0, len(scenes))
	for _, sc := range scenes {
		result = append(result, s.sceneToDTO(tenantID, sc))
	}
	return result, nil
}

// ListAll returns all scenes without filtering.
func (s *SceneService) ListAll(tenantID string) ([]*SceneDTO, error) {
	return s.List(tenantID, 0)
}

// GetScene returns a single scene by name.
func (s *SceneService) GetScene(tenantID, name string) (*SceneDTO, error) {
	sc, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, scene.ErrSceneNotFound
	}
	return s.sceneToDTO(tenantID, sc), nil
}

// CreateScene validates and creates a new scene.
func (s *SceneService) CreateScene(tenantID string, input *CreateSceneInput) (*SceneDTO, error) {
	if err := ValidateSceneName(input.Name); err != nil {
		return nil, err
	}
	if err := ValidateSceneTitle(input.Title); err != nil {
		return nil, err
	}
	if err := ValidateScenePrompt(input.Prompt); err != nil {
		return nil, err
	}

	exists, err := s.repo.ExistsByName(tenantID, input.Name)
	if err != nil {
		return nil, fmt.Errorf("检查场景存在性失败: %w", err)
	}
	if exists {
		return nil, scene.ErrSceneExists
	}

	agentExists, err := s.agentRepo.Exists(tenantID, input.AgentID)
	if err != nil {
		return nil, fmt.Errorf("检查 Agent 存在性失败: %w", err)
	}
	if !agentExists {
		return nil, scene.ErrAgentNotFound
	}

	sc := &scene.Scene{
		Name:     input.Name,
		AgentID:  input.AgentID,
		Title:    input.Title,
		TitleEn:  input.TitleEn,
		Prompt:   input.Prompt,
		PromptEn: input.PromptEn,
		Enabled:  true,
	}

	if err := s.repo.Create(tenantID, sc); err != nil {
		return nil, fmt.Errorf("创建场景失败: %w", err)
	}

	return s.sceneToDTO(tenantID, sc), nil
}

// UpdateScene modifies an existing scene by name.
func (s *SceneService) UpdateScene(tenantID, name string, input *UpdateSceneInput) (*SceneDTO, error) {
	sc, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, scene.ErrSceneNotFound
	}

	if err := s.validateAndUpdateSceneFields(tenantID, sc, input); err != nil {
		return nil, err
	}

	if err := s.repo.Update(tenantID, sc); err != nil {
		return nil, fmt.Errorf("更新场景失败: %w", err)
	}

	return s.sceneToDTO(tenantID, sc), nil
}

// validateAndUpdateSceneFields validates and applies update fields to a scene entity.
func (s *SceneService) validateAndUpdateSceneFields(tenantID string, sc *scene.Scene, input *UpdateSceneInput) error {
	if input.AgentID != nil {
		agentExists, err := s.agentRepo.Exists(tenantID, *input.AgentID)
		if err != nil {
			return fmt.Errorf("检查 Agent 存在性失败: %w", err)
		}
		if !agentExists {
			return scene.ErrAgentNotFound
		}
		sc.AgentID = *input.AgentID
	}

	if input.Title != "" {
		if err := ValidateSceneTitle(input.Title); err != nil {
			return err
		}
		sc.Title = input.Title
	}
	if input.TitleEn != "" {
		sc.TitleEn = input.TitleEn
	}

	if input.Prompt != "" {
		if err := ValidateScenePrompt(input.Prompt); err != nil {
			return err
		}
		sc.Prompt = input.Prompt
	}
	if input.PromptEn != "" {
		sc.PromptEn = input.PromptEn
	}

	if input.Enabled != nil {
		sc.Enabled = *input.Enabled
	}
	return nil
}

// DeleteScene removes a scene by name.
func (s *SceneService) DeleteScene(tenantID, name string) error {
	sc, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return scene.ErrSceneNotFound
	}

	if err := s.repo.Delete(tenantID, sc.ID); err != nil {
		return fmt.Errorf("删除场景失败: %w", err)
	}

	return nil
}

// sceneToDTO converts a Scene domain entity to a SceneDTO.
func (s *SceneService) sceneToDTO(tenantID string, sc *scene.Scene) *SceneDTO {
	agentName := ""
	if cfg, err := s.agentRepo.GetByID(tenantID, sc.AgentID); err == nil {
		agentName = cfg.Name
	}
	return &SceneDTO{
		ID:        sc.ID,
		Name:      sc.Name,
		Agent:     agentName,
		Title:     sc.Title,
		TitleEn:   sc.TitleEn,
		Prompt:    sc.Prompt,
		PromptEn:  sc.PromptEn,
		Enabled:   sc.Enabled,
		CreatedAt: sc.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: sc.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
