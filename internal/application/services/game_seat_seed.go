package services

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/gameseat"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var wolfchaResidents = []struct {
	Key, Name, Role, Bio, Voice string
}{
	{"zhou", "老周", "茶馆老板", "稳重，爱和稀泥，口头禅‘大家先别急’", "longshu_v3.1"},
	{"zhen", "阿珍", "本地记者", "犀利多疑，爱追问细节", "longjing_v3.1"},
	{"lin", "小林", "大学生", "冲动，爱跟风，被怀疑时会急", "longshao_v3.1"},
	{"chen", "陈伯", "退休警察", "爱推理，讲证据，偶尔倚老卖老", "longnan_v3.1"},
	{"meimei", "美美", "主播", "爱演，情绪外放，喜欢带节奏", "longanhuan_v3.1"},
	{"jie", "阿杰", "程序员", "话少，记仇，开口就是逻辑链", "longtian_v3.1"},
	{"xiaoyu", "小雨", "咖啡师", "善于观察表情，说话温和但立场坚定", "longanlingxin_v3.1"},
	{"laowu", "老吴", "出租车司机", "见多识广，喜欢从生活经验判断人", "longlaotie_v3.1"},
	{"tang", "唐姐", "社区医生", "冷静细致，习惯核对时间和细节", "guyunshu_v3.1"},
}

func residentAvatar(name, color string) string {
	initial := string([]rune(name)[0])
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96"><rect width="96" height="96" rx="48" fill="#%s"/><text x="48" y="64" fill="white" font-size="43" text-anchor="middle">%s</text></svg>`, color, initial)
	return "data:image/svg+xml," + url.PathEscape(svg)
}

// SeedWolfchaResidents creates or updates nine stable Agents and their game roster bindings.
func (s *GameSeatService) SeedWolfchaResidents() error {
	var template agent.AgentConfig
	selectedName := strings.TrimSpace(os.Getenv("WOLFCHA_TEMPLATE_AGENT_NAME"))
	query := s.db.Where("tenant_id = ? AND provider_id IS NOT NULL AND model_id <> ''", s.tenantID)
	if selectedName != "" {
		query = query.Where("name = ?", selectedName)
	}
	if err := query.Order("id").First(&template).Error; err != nil {
		return fmt.Errorf("configure a model-backed Agent or set WOLFCHA_TEMPLATE_AGENT_NAME: %w", err)
	}
	colors := []string{"725945", "884c5d", "4f7391", "626b4b", "a26778", "596574", "547b84", "81725d", "6f698c"}
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
			config := map[string]interface{}{"systemPrompt": prompt, "providerId": float64(*template.ProviderID), "modelId": template.ModelID}
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
			Bio: strings.TrimSpace(resident.Role + "，" + resident.Bio), Voice: resident.Voice}
		if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "game_key"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"agent_id", "name", "avatar_url", "bio", "voice"})}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
