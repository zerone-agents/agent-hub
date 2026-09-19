// Package emotion 实现 io.zerone.emotion 能力包的领域纯函数：
// 心情事件词表（v1 锁定）、衰减结算、强度钳制与确定性叙述句式。
// 所有状态转移都是 (上一状态, 事件, 事件时间) 的纯函数，可重放。
package emotion

import (
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	// Namespace 是状态命名空间，对应 RunState.namespace。
	Namespace = "io.zerone.emotion"
	// SchemaName / SchemaVersion 锁定 emotion-state v1。
	SchemaName    = "emotion-state"
	SchemaVersion = "v1"
	// SubjectType 表明心情状态挂在单个 Agent 上。
	SubjectType = "agent"
	// RuleSource 标识规则归属，便于审计。
	RuleSource = "builtin:emotion"
	// RuleV1 是词表版本，与 CapabilityBinding 快照锁定一致。
	RuleV1 = "v1"

	// 强度为有符号值：正值 = 正向情绪，负值 = 负面情绪，0 = 回到基线。
	MinIntensity = -100
	MaxIntensity = 100
	// 单事件上限：正向 +30，负向 -40。
	MaxPositiveDelta = 30
	MaxNegativeDelta = -40
	// DefaultDecayPerDay 是每天向基线回落的强度点数。
	DefaultDecayPerDay = 20
	// DefaultBaseline 是人格底色未声明默认心情时的取值。
	DefaultBaseline = "calm"
)

// 心情取值（状态 mood / baseline 的枚举）。
const (
	MoodCalm     = "calm"
	MoodWary     = "wary"
	MoodTense    = "tense"
	MoodAngry    = "angry"
	MoodElated   = "elated"
	MoodGrieving = "grieving"
)

var ErrUnknownEventType = errors.New("未知的心情事件类型")

// moodVocabulary 是包私有的 v1 心情事件词表：每个事件一个基础 delta
// 和一个确定性心情指向（用于强度分档后的 mood 投影）。
var moodVocabulary = map[string]struct {
	delta int
	mood  string
}{
	"betrayed":       {-30, MoodGrieving},
	"trusted":        {+15, MoodCalm},
	"threatened":     {-20, MoodTense},
	"comforted":      {+12, MoodCalm},
	"task_failed":    {-10, MoodTense},
	"task_completed": {+10, MoodElated},
	"insulted":       {-20, MoodAngry},
	"praised":        {+12, MoodElated},
}

// State 是 emotion-state v1 的运行态（与 RunState.data 对应）。
type State struct {
	Mood        string    `json:"mood"`
	Intensity   int       `json:"intensity"`
	Baseline    string    `json:"baseline"`
	UpdatedAt   time.Time `json:"updatedAt"`
	DecayPerDay int       `json:"decayPerDay"`
	Narration   string    `json:"narration"`
}

// DefaultState 返回基线默认状态：人格底色未声明默认心情时 baseline 为 calm。
func DefaultState(baseline string) *State {
	if baseline == "" {
		baseline = DefaultBaseline
	}
	s := &State{Mood: baseline, Baseline: baseline, DecayPerDay: DefaultDecayPerDay}
	s.Narration = NarrationFor(s.Mood, s.Intensity)
	return s
}

// EventDelta 返回 v1 词表事件的确定性强度增量。
// severity 是 1..3 序数：2 → ×1.5，3 → ×2；结果受 +30 / -40 单事件上限约束。
func EventDelta(eventType string, severity int) (int, bool) {
	entry, ok := moodVocabulary[eventType]
	if !ok {
		return 0, false
	}
	multiplier := 1.0
	switch severity {
	case 2:
		multiplier = 1.5
	case 3:
		multiplier = 2
	}
	delta := int(math.Round(float64(entry.delta) * multiplier))
	if delta > MaxPositiveDelta {
		return MaxPositiveDelta, true
	}
	if delta < MaxNegativeDelta {
		return MaxNegativeDelta, true
	}
	return delta, true
}

// SettleDecay 是纯衰减结算：按 from 到 to 经过的天数（可为小数），
// 以 decayPerDay 的速率向基线（强度 0）回落，正强度向下收、负强度向上收。
// to 不早于 from 时原样返回语义等价的状态。
func SettleDecay(s *State, from, to time.Time) *State {
	out := *s
	if !to.After(from) || s.Intensity == 0 {
		return &out
	}
	days := to.Sub(from).Hours() / 24
	reduction := int(days * float64(s.DecayPerDay))
	if reduction <= 0 {
		return &out
	}
	if out.Intensity > 0 {
		out.Intensity -= reduction
		if out.Intensity < 0 {
			out.Intensity = 0
		}
	} else {
		out.Intensity += reduction
		if out.Intensity > 0 {
			out.Intensity = 0
		}
	}
	// 强度回落到平静档（|强度| ≤ 20）后，心情归位到基线心情，与叙述句
	// "情绪平稳"分档保持一致；未回落则保持当前心情。
	if abs(out.Intensity) <= 20 {
		out.Mood = out.Baseline
	}
	out.UpdatedAt = to
	out.Narration = NarrationFor(out.Mood, out.Intensity)
	return &out
}

// ApplyEvent 是纯事件结算：调用方需先自行 SettleDecay 到事件时间。
// 强度 = clamp(原强度 + delta)，mood 按强度分档投影，叙述句同步重建。
func ApplyEvent(s *State, eventType string, severity int, at time.Time) (*State, error) {
	entry, ok := moodVocabulary[eventType]
	if !ok {
		return nil, ErrUnknownEventType
	}
	delta, _ := EventDelta(eventType, severity)
	out := *s
	out.Intensity = ClampIntensity(s.Intensity + delta)
	if mood := projectMood(entry.mood, delta, out.Intensity, out.Baseline); mood != "" {
		out.Mood = mood
	}
	out.UpdatedAt = at
	out.Narration = NarrationFor(out.Mood, out.Intensity)
	return &out, nil
}

// projectMood 是强度分档投影：|强度| ≤ 20（平静档）回到基线心情，保证与
// 叙述句"情绪平稳"一致；其余采用事件指向的心情——正负事件在同一状态上
// 叠加时，以当前强度符号一侧的事件指向为准，避免"振奋 (-48)"这类矛盾。
func projectMood(eventMood string, delta, intensity int, baseline string) string {
	if abs(intensity) <= 20 {
		return baseline
	}
	if (intensity < 0) != (delta < 0) {
		// 强度符号与本次事件方向相反：沿用原状态的心情（若有）。
		return ""
	}
	return eventMood
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func ClampIntensity(v int) int {
	if v < MinIntensity {
		return MinIntensity
	}
	if v > MaxIntensity {
		return MaxIntensity
	}
	return v
}

var moodChinese = map[string]string{
	MoodCalm:     "平静",
	MoodWary:     "警惕",
	MoodTense:    "紧张",
	MoodAngry:    "愤怒",
	MoodElated:   "振奋",
	MoodGrieving: "悲伤",
}

func moodLabel(mood string) string {
	if label, ok := moodChinese[mood]; ok {
		return label
	}
	return mood
}

// NarrationFor 把强度映射为固定中文句式（确定性叙述，模型不可自由发挥）。
// 分档按 |强度|：0–20 平静 / 21–40 波动 / 41–60 显著 / 61–80 强烈 / 81–100 主导行为。
// 强度带符号：正值为正向情绪，负值为负面情绪。
func NarrationFor(mood string, intensity int) string {
	magnitude := abs(intensity)
	var band string
	switch {
	case magnitude <= 20:
		band = "你的情绪平稳，几乎不受干扰。"
	case magnitude <= 40:
		band = "你感到些许波动，但尚能自持。"
	case magnitude <= 60:
		band = "你的情绪显著起伏，开始影响你的判断。"
	case magnitude <= 80:
		band = "你情绪激动，强烈地影响着你的言行。"
	default:
		band = "你的情绪已经主导了你的行为。"
	}
	return fmt.Sprintf("你当前心情为%s（强度 %d/100）。%s", moodLabel(mood), intensity, band)
}
