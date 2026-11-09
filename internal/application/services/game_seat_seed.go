package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/gameseat"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var wolfchaResidents = []struct {
	Key, Name, Role, Bio string
}{
	{"zhou", "老周", "茶馆老板", "稳重，爱和稀泥，口头禅‘大家先别急’"},
	{"zhen", "阿珍", "本地记者", "犀利多疑，爱追问细节"},
	{"lin", "小林", "大学生", "冲动，爱跟风，被怀疑时会急"},
	{"chen", "陈伯", "退休警察", "爱推理，讲证据，偶尔倚老卖老"},
	{"meimei", "美美", "主播", "爱演，情绪外放，喜欢带节奏"},
	{"jie", "阿杰", "程序员", "话少，记仇，开口就是逻辑链"},
}

func residentAvatar(name, color string) string {
	initial := string([]rune(name)[0])
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96"><rect width="96" height="96" rx="48" fill="#%s"/><text x="48" y="64" fill="white" font-size="43" text-anchor="middle">%s</text></svg>`, color, initial)
	return "data:image/svg+xml," + url.PathEscape(svg)
}

// SeedWolfchaResidents creates or updates six stable Agents and their game
// roster bindings. Voice is intentionally blank until the voice task.
func (s *GameSeatService) SeedWolfchaResidents() error {
	var template agent.AgentConfig
	if err := s.db.Where("tenant_id = ? AND LOWER(model_id) LIKE ? AND provider_id IS NOT NULL", s.tenantID, "%deepseek%").Order("id").First(&template).Error; err != nil {
		return fmt.Errorf("configure a DeepSeek Agent before seeding residents: %w", err)
	}
	colors := []string{"725945", "884c5d", "4f7391", "626b4b", "a26778", "596574"}
	for i, resident := range wolfchaResidents {
		name := "wolfcha-" + resident.Key
		prompt := fmt.Sprintf("你是%s，%s。你的说话方式：%s。你是游戏中的固定居民，记得和同一玩家相处的经历。", resident.Name, resident.Role, resident.Bio)
		var residentAgent agent.AgentConfig
		err := s.db.Where("tenant_id = ? AND name = ?", s.tenantID, name).First(&residentAgent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			config := map[string]interface{}{"systemPrompt": prompt, "providerId": float64(*template.ProviderID), "modelId": template.ModelID,
				"title": map[string]interface{}{"zh-CN": resident.Name}, "description": map[string]interface{}{"zh-CN": resident.Role + "，" + resident.Bio}}
			if _, err = s.agents.CreateAgent(s.tenantID, &CreateAgentInput{Name: name, Config: config}); err != nil {
				return err
			}
		} else if err == nil {
			config := map[string]interface{}{"systemPrompt": prompt}
			if _, err = s.agents.UpdateAgent(s.tenantID, name, &UpdateAgentInput{Config: &config}); err != nil {
				return err
			}
		} else {
			return err
		}
		if err := s.db.Where("tenant_id = ? AND name = ?", s.tenantID, name).First(&residentAgent).Error; err != nil {
			return err
		}
		row := gameseat.Character{TenantID: s.tenantID, GameKey: "wolfcha", Key: resident.Key, AgentID: residentAgent.ID,
			Name: resident.Name, AvatarURL: residentAvatar(resident.Name, colors[i]),
			Bio: strings.TrimSpace(resident.Role + "，" + resident.Bio), Voice: ""}
		if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "game_key"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"agent_id", "name", "avatar_url", "bio"})}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
