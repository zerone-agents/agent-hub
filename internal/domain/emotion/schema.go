package emotion

import (
	"fmt"
	"time"
)

// SchemaDocument 返回 emotion-state v1 的 JSON Schema 文档，
// 供 RunService.RegisterStateSchema 注册（scope=run，subject=agent）。
func SchemaDocument() map[string]any {
	moodEnum := []any{MoodCalm, MoodWary, MoodTense, MoodAngry, MoodElated, MoodGrieving}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"mood":        map[string]any{"type": "string", "enum": moodEnum},
			"intensity":   map[string]any{"type": "integer", "minimum": -100.0, "maximum": 100.0},
			"baseline":    map[string]any{"type": "string", "enum": moodEnum},
			"updatedAt":   map[string]any{"type": "string", "format": "date-time"},
			"decayPerDay": map[string]any{"type": "integer", "minimum": 1.0},
			"narration":   map[string]any{"type": "string"},
		},
		"required": []any{"mood", "intensity", "baseline", "updatedAt", "decayPerDay", "narration"},
	}
}

// ToMap 把状态编码为 RunState.data 的 JSON 形态。
func (s *State) ToMap() map[string]any {
	return map[string]any{
		"mood":        s.Mood,
		"intensity":   s.Intensity,
		"baseline":    s.Baseline,
		"updatedAt":   s.UpdatedAt.UTC().Format(time.RFC3339),
		"decayPerDay": s.DecayPerDay,
		"narration":   s.Narration,
	}
}

// StateFromData 解析 RunState.data（JSON 反序列化后的 map）。
// 容忍数值以浮点承载；缺省字段回落到默认状态，避免脏数据导致读取失败。
func StateFromData(data map[string]any) (*State, error) {
	if data == nil {
		return DefaultState(""), nil
	}
	s := DefaultState("")
	if v, ok := data["baseline"].(string); ok && v != "" {
		s.Baseline = v
		s.Mood = v
	}
	if v, ok := data["mood"].(string); ok && v != "" {
		s.Mood = v
	}
	if v, ok := data["intensity"].(float64); ok {
		s.Intensity = ClampIntensity(int(v))
	} else if v, ok := data["intensity"].(int); ok {
		s.Intensity = ClampIntensity(v)
	}
	if v, ok := data["decayPerDay"].(float64); ok && v >= 1 {
		s.DecayPerDay = int(v)
	}
	if v, ok := data["narration"].(string); ok {
		s.Narration = v
	} else {
		s.Narration = NarrationFor(s.Mood, s.Intensity)
	}
	if v, ok := data["updatedAt"].(string); ok && v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, fmt.Errorf("心情状态 updatedAt 解析失败: %w", err)
		}
		s.UpdatedAt = parsed
	}
	return s, nil
}
