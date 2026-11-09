package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/gameseat"
	"control-panel/internal/domain/run"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GameSeatService struct {
	db        *gorm.DB
	tenantID  string
	runs      *RunService
	agents    *AgentService
	memory    *MemoryService
	emotion   *EmotionService
	relation  *RelationDynamicsService
	providers *ProviderService
	client    *http.Client
	budget    int
	modelCall func(context.Context, agent.AgentConfig, []json.RawMessage, GameSeatGenerateInput) (*http.Response, error)
}

type GameSeatRosterItem struct {
	CharacterID string `json:"characterId"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatarUrl"`
	Bio         string `json:"bio"`
	Voice       string `json:"voice"`
}

type GameSeatSessionInput struct {
	GameKey    string `json:"gameKey"`
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
	Seats      []struct {
		Seat        int    `json:"seat"`
		CharacterID string `json:"characterId"`
	} `json:"seats"`
}

type GameSeatGenerateInput struct {
	Seat           int               `json:"seat"`
	CharacterID    string            `json:"characterId"`
	Phase          string            `json:"phase"`
	Messages       []json.RawMessage `json:"messages"`
	Temperature    *float64          `json:"temperature,omitempty"`
	ResponseFormat json.RawMessage   `json:"response_format,omitempty"`
	Stream         bool              `json:"stream,omitempty"`
}

type GameSeatEventInput struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"`
	Day        int    `json:"day"`
	ActorSeat  *int   `json:"actorSeat,omitempty"`
	TargetSeat *int   `json:"targetSeat,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type GameSeatMemoryView struct {
	Attitude int      `json:"attitude"`
	Stance   string   `json:"stance"`
	Mood     string   `json:"mood"`
	Recent   []string `json:"recent"`
}

var ErrGameSeatBudget = errors.New("game seat model call budget exceeded")

func NewGameSeatService(db *gorm.DB, tenantID string, runs *RunService, agents *AgentService, providers *ProviderService, budget int) *GameSeatService {
	if budget <= 0 {
		budget = 120
	}
	return &GameSeatService{db: db, tenantID: tenantID, runs: runs, agents: agents,
		memory: NewMemoryService(runs), emotion: NewEmotionService(runs), relation: NewRelationDynamicsService(runs),
		providers: providers, client: &http.Client{Timeout: 90 * time.Second}, budget: budget}
}

func (s *GameSeatService) Migrate() error {
	return s.db.AutoMigrate(&gameseat.Character{}, &gameseat.PlayerRun{}, &gameseat.Session{}, &gameseat.SessionSeat{}, &gameseat.Event{})
}

func (s *GameSeatService) Roster(gameKey string) ([]GameSeatRosterItem, error) {
	var rows []gameseat.Character
	if err := s.db.Where("tenant_id = ? AND game_key = ?", s.tenantID, gameKey).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]GameSeatRosterItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, GameSeatRosterItem{row.Key, row.Name, row.AvatarURL, row.Bio, row.Voice})
	}
	return out, nil
}

func (s *GameSeatService) CreateSession(input GameSeatSessionInput) (string, error) {
	if strings.TrimSpace(input.GameKey) == "" || strings.TrimSpace(input.PlayerID) == "" || len(input.Seats) == 0 {
		return "", fmt.Errorf("gameKey, playerId and seats are required")
	}
	var binding gameseat.PlayerRun
	err := s.db.Where("tenant_id = ? AND game_key = ? AND player_id = ?", s.tenantID, input.GameKey, input.PlayerID).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		created, createErr := s.runs.Create(s.tenantID, CreateRunInput{Name: "game-seats:" + input.GameKey + ":" + input.PlayerID, CreatedBy: "game-seats", Metadata: map[string]any{"gameKey": input.GameKey, "playerId": input.PlayerID}})
		if createErr != nil {
			return "", createErr
		}
		var characters []gameseat.Character
		if createErr = s.db.Where("tenant_id = ? AND game_key = ?", s.tenantID, input.GameKey).Find(&characters).Error; createErr != nil {
			return "", createErr
		}
		for _, character := range characters {
			if _, createErr = s.runs.AddAgent(s.tenantID, created.ID, character.AgentID, "game-resident"); createErr != nil {
				return "", createErr
			}
		}
		if _, createErr = s.runs.Transition(s.tenantID, created.ID, run.StatusRunning); createErr != nil {
			return "", createErr
		}
		binding = gameseat.PlayerRun{TenantID: s.tenantID, GameKey: input.GameKey, PlayerID: input.PlayerID, RunID: created.ID}
		if createErr = s.db.Create(&binding).Error; createErr != nil {
			return "", createErr
		}
	} else if err != nil {
		return "", err
	}
	var session gameseat.Session
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&binding).UpdateColumn("games", gorm.Expr("games + 1")).Error; err != nil {
			return err
		}
		if err := tx.First(&binding, binding.ID).Error; err != nil {
			return err
		}
		session = gameseat.Session{ID: uuid.NewString(), TenantID: s.tenantID, GameKey: input.GameKey, PlayerID: input.PlayerID, PlayerName: input.PlayerName, RunID: binding.RunID, GameNumber: binding.Games}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		for _, seat := range input.Seats {
			var character gameseat.Character
			if err := tx.Where("tenant_id = ? AND game_key = ? AND key = ?", s.tenantID, input.GameKey, seat.CharacterID).First(&character).Error; err != nil {
				return err
			}
			if err := tx.Create(&gameseat.SessionSeat{SessionID: session.ID, Seat: seat.Seat, CharacterID: character.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return session.ID, err
}

func (s *GameSeatService) sessionCharacter(sessionID string, seat int, key string) (gameseat.Session, gameseat.Character, error) {
	var session gameseat.Session
	var character gameseat.Character
	if err := s.db.Where("tenant_id = ? AND id = ?", s.tenantID, sessionID).First(&session).Error; err != nil {
		return session, character, err
	}
	var seatRow gameseat.SessionSeat
	if err := s.db.Where("session_id = ? AND seat = ?", sessionID, seat).First(&seatRow).Error; err != nil {
		return session, character, err
	}
	if err := s.db.Where("id = ? AND key = ?", seatRow.CharacterID, key).First(&character).Error; err != nil {
		return session, character, err
	}
	return session, character, nil
}

func (s *GameSeatService) Memory(gameKey, playerID, characterKey string) (GameSeatMemoryView, error) {
	view := GameSeatMemoryView{Stance: "neutral", Mood: "平静", Recent: []string{}}
	var binding gameseat.PlayerRun
	if err := s.db.Where("tenant_id = ? AND game_key = ? AND player_id = ?", s.tenantID, gameKey, playerID).First(&binding).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return view, nil
	} else if err != nil {
		return view, err
	}
	var character gameseat.Character
	if err := s.db.Where("tenant_id = ? AND game_key = ? AND key = ?", s.tenantID, gameKey, characterKey).First(&character).Error; err != nil {
		return view, err
	}
	attitude, err := s.relation.View(s.tenantID, binding.RunID, character.AgentID, 0)
	if err != nil {
		return view, err
	}
	view.Attitude, view.Stance = attitude.Score, attitude.Stance
	mood, err := s.emotion.Status(s.tenantID, binding.RunID, character.AgentID)
	if err != nil {
		return view, err
	}
	view.Mood = mood.Narration
	memories, err := s.memory.Recall(s.tenantID, binding.RunID, character.AgentID, "", 5, time.Now().UTC())
	if err != nil {
		return view, err
	}
	for _, memory := range memories {
		view.Recent = append(view.Recent, memory.Interpretation)
	}
	return view, nil
}

func (s *GameSeatService) Generate(ctx context.Context, sessionID string, input GameSeatGenerateInput) (*http.Response, error) {
	if len(input.Messages) == 0 || input.Phase == "" {
		return nil, fmt.Errorf("phase and messages are required")
	}
	session, character, err := s.sessionCharacter(sessionID, input.Seat, input.CharacterID)
	if err != nil {
		return nil, err
	}
	result := s.db.Model(&gameseat.Session{}).Where("id = ? AND tenant_id = ? AND calls < ?", sessionID, s.tenantID, s.budget).UpdateColumn("calls", gorm.Expr("calls + 1"))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrGameSeatBudget
	}
	var hubAgent agent.AgentConfig
	if err := s.db.Where("tenant_id = ? AND id = ?", s.tenantID, character.AgentID).First(&hubAgent).Error; err != nil {
		return nil, err
	}
	memory, err := s.Memory(session.GameKey, session.PlayerID, character.Key)
	if err != nil {
		return nil, err
	}
	persona := fmt.Sprintf("人物设定：%s。%s\n说话方式：%s\n你记得的和玩家有关的事：%s\n此刻心情：%s\n对玩家的态度：%d（%s）。\n不得说出自己隐藏的身份牌和夜间信息，除非原提示词要求；输出格式必须严格遵守原提示词的要求。", character.Name, hubAgent.SystemPrompt, character.Bio, strings.Join(memory.Recent, "；"), memory.Mood, memory.Attitude, memory.Stance)
	first, _ := json.Marshal(map[string]any{"role": "system", "content": persona})
	messages := append([]json.RawMessage{first}, input.Messages...)
	if s.modelCall != nil {
		return s.modelCall(ctx, hubAgent, messages, input)
	}
	if hubAgent.ProviderID == nil || hubAgent.ModelID == "" {
		return nil, fmt.Errorf("resident model is not configured")
	}
	configs, err := s.providers.ListRuntimeConfigs(s.tenantID)
	if err != nil {
		return nil, err
	}
	var selected *ProviderRuntimeConfig
	for _, config := range configs {
		if config.ID == *hubAgent.ProviderID {
			selected = config
			break
		}
	}
	if selected == nil || selected.APIKey == "" || selected.Protocol != "openai" {
		return nil, fmt.Errorf("resident OpenAI provider is unavailable")
	}
	body := map[string]any{"model": hubAgent.ModelID, "messages": messages, "stream": input.Stream}
	if input.Temperature != nil {
		body["temperature"] = *input.Temperature
	}
	if len(input.ResponseFormat) > 0 {
		body["response_format"] = input.ResponseFormat
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(selected.BaseURL, "/") + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+selected.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("resident model returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

func (s *GameSeatService) Event(sessionID string, input GameSeatEventInput) error {
	valid := map[string]bool{"vote": true, "eliminated": true, "killed": true, "saved": true, "checked": true, "sheriff": true, "accused": true, "game_end": true}
	if !valid[input.Type] || input.Seq < 0 {
		return fmt.Errorf("invalid event")
	}
	var session gameseat.Session
	if err := s.db.Preload("Seats").Where("tenant_id = ? AND id = ?", s.tenantID, sessionID).First(&session).Error; err != nil {
		return err
	}
	// Votes, rescues and accusations by another AI are game facts, but they do
	// not change a resident's attitude toward the human player.
	affectsPlayer := true
	if input.ActorSeat != nil && (input.Type == "vote" || input.Type == "saved" || input.Type == "accused") {
		for _, seat := range session.Seats {
			if seat.Seat == *input.ActorSeat {
				affectsPlayer = false
			}
		}
	}
	var prior gameseat.Event
	if err := s.db.Where("session_id = ? AND seq = ?", sessionID, input.Seq).First(&prior).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	for _, seat := range session.Seats {
		if input.Type != "game_end" && (input.TargetSeat == nil || seat.Seat != *input.TargetSeat) {
			continue
		}
		var character gameseat.Character
		if err := s.db.First(&character, seat.CharacterID).Error; err != nil {
			return err
		}
		eventType := map[string]string{"vote": "insulted", "eliminated": "betrayed", "killed": "threatened", "saved": "comforted", "checked": "threatened", "sheriff": "praised", "accused": "threatened", "game_end": "task_completed"}[input.Type]
		relationEvent := eventType
		if input.Type == "saved" {
			relationEvent = "trusted"
		}
		if input.Type == "sheriff" {
			relationEvent = "trusted"
		}
		key := fmt.Sprintf("game-seat:%s:%d:%d", sessionID, input.Seq, character.AgentID)
		if affectsPlayer {
			if err := s.emotion.OnEvent(s.tenantID, session.RunID, character.AgentID, eventType, 1, time.Now().UTC(), key+":emotion"); err != nil {
				return err
			}
			if err := s.relation.OnEvent(s.tenantID, session.RunID, character.AgentID, 0, relationEvent, 1, time.Now().UTC(), key+":relation"); err != nil {
				return err
			}
		}
		story := fmt.Sprintf("第 %d 局第 %d 天，%s", session.GameNumber, input.Day, input.Detail)
		if strings.TrimSpace(input.Detail) == "" {
			story = fmt.Sprintf("第 %d 局第 %d 天发生了 %s 事件", session.GameNumber, input.Day, input.Type)
		}
		if _, err := s.memory.Record(s.tenantID, session.RunID, character.AgentID, key, story, 70, time.Now().UTC(), key+":memory"); err != nil {
			return err
		}
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&gameseat.Event{SessionID: sessionID, Seq: input.Seq, Type: input.Type}).Error
}
