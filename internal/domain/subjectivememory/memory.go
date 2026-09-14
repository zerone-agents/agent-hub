package subjectivememory

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
)

// Capability pack: io.zerone.subjective-memory (H6 动态人物能力包 · 记忆).
// Every memory is a subjective interpretation an agent wrote for itself about
// a fact or event. Entries live per-Run in run_states (schema memory-entry v1,
// subject=agent, subjectID=memory ID); this package holds only the pure
// retrieval rules so they stay deterministic and replayable.

const (
	Namespace     = "io.zerone.subjective-memory"
	SchemaName    = "memory-entry"
	SchemaVersion = "v1"
	SubjectType   = "agent"

	MinImportance = 0
	MaxImportance = 100

	// MaxInterpretationRunes mirrors the contract: the agent-written
	// interpretation is capped at 200 characters.
	MaxInterpretationRunes = 200

	// DefaultRecallLimit is used when a recall query passes limit <= 0.
	DefaultRecallLimit = 5

	// Retrieval scoring weights: score = importance*0.5 + recency*0.3
	// + relevance*0.2. All three components are normalized to 0..100.
	WeightImportance = 0.5
	WeightRecency    = 0.3
	WeightRelevance  = 0.2

	// Soft forgetting: a memory the agent judged unimportant, has not
	// recalled, and recorded long ago sinks below the retrieval cutoff and
	// no longer enters prompts. The data is retained for audit replay.
	SoftForgetImportanceMax = 20
	SoftForgetAgeDays       = 7.0
	SoftForgetRecallCount   = 1

	// recencyHalfLifeDays drives the deterministic recency decay: a memory
	// loses half of its recency score every 3 days after recordedAt.
	recencyHalfLifeDays = 3.0
)

var (
	ErrEmptyFactRef          = errors.New("事实引用不能为空")
	ErrEmptyInterpretation   = errors.New("记忆解释不能为空")
	ErrInterpretationTooLong = errors.New("记忆解释不能超过 200 字")
)

// MemoryEntry is one subjective memory. Importance is the agent's
// self-assessment clamped to 0..100; EmotionTag optionally references the
// emotion pack vocabulary; RecallCount counts best-effort recall increments.
type MemoryEntry struct {
	ID             string    `json:"id"`
	AgentID        uint64    `json:"agentId"`
	FactRef        string    `json:"factRef"`
	Interpretation string    `json:"interpretation"`
	Importance     int       `json:"importance"`
	EmotionTag     string    `json:"emotionTag,omitempty"`
	RecordedAt     time.Time `json:"recordedAt"`
	RecallCount    int       `json:"recallCount"`
}

// ClampImportance pins an agent self-assessment into the 0..100 range.
func ClampImportance(v int) int {
	if v < MinImportance {
		return MinImportance
	}
	if v > MaxImportance {
		return MaxImportance
	}
	return v
}

// Validate checks the agent-written fields. Importance is clamped, not
// rejected, per the "self-assessment + rule clamp" convention.
func Validate(factRef, interpretation string) error {
	if strings.TrimSpace(factRef) == "" {
		return ErrEmptyFactRef
	}
	if strings.TrimSpace(interpretation) == "" {
		return ErrEmptyInterpretation
	}
	if len([]rune(interpretation)) > MaxInterpretationRunes {
		return ErrInterpretationTooLong
	}
	return nil
}

// RecencyScore is a deterministic decay in 0..100 resolved lazily from the
// recorded event time: 100 at recording, halving every recencyHalfLifeDays.
// Event-time based, so replaying the same event sequence reproduces it.
func RecencyScore(recordedAt, at time.Time) float64 {
	age := at.Sub(recordedAt).Hours() / 24
	if age <= 0 {
		return 100
	}
	return 100 * math.Pow(0.5, age/recencyHalfLifeDays)
}

// tokenize splits text into lowercase word tokens; CJK runs are kept whole
// so a substring of contiguous CJK characters matches as one token.
func tokenize(s string) []string {
	var tokens []string
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			tokens = append(tokens, string(buf))
			buf = nil
		}
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			buf = append(buf, r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// RelevanceScore measures query overlap against the memory text
// (factRef + interpretation) as matchedDistinctQueryTokens/totalQueryTokens
// in 0..100. A query token matches when it equals a text token or appears
// inside one, so both space-separated words and CJK substrings inside a
// longer run match. An empty query is topic-neutral and scores 50 — recall
// without a query ranks by importance and recency alone, keeping the
// relevance component at its midpoint.
func RelevanceScore(query, factRef, interpretation string) float64 {
	q := tokenize(query)
	if len(q) == 0 {
		return 50
	}
	var text []string
	for _, t := range tokenize(factRef + " " + interpretation) {
		text = append(text, t)
	}
	matched := 0
	seen := map[string]bool{}
	for _, t := range q {
		if seen[t] {
			continue
		}
		seen[t] = true
		for _, tok := range text {
			if tok == t || strings.Contains(tok, t) {
				matched++
				break
			}
		}
	}
	return 100 * float64(matched) / float64(len(seen))
}

// Score is the pure retrieval score in 0..100:
// importance×0.5 + recency×0.3 + relevance×0.2.
func Score(e MemoryEntry, query string, at time.Time) float64 {
	return WeightImportance*float64(ClampImportance(e.Importance)) +
		WeightRecency*RecencyScore(e.RecordedAt, at) +
		WeightRelevance*RelevanceScore(query, e.FactRef, e.Interpretation)
}

// SoftForgotten reports whether the entry sank below the retrieval cutoff:
// low importance, old, and rarely recalled. Data is kept; it just stops
// being surfaced.
func SoftForgotten(e MemoryEntry, at time.Time) bool {
	ageDays := at.Sub(e.RecordedAt).Hours() / 24
	return e.Importance <= SoftForgetImportanceMax &&
		ageDays >= SoftForgetAgeDays &&
		e.RecallCount <= SoftForgetRecallCount
}

// Rank is the deterministic top-N selection used by recall: drop soft
// forgotten entries, sort by score descending, break ties by importance,
// then recency, then ID so equal inputs always replay to the same order.
func Rank(entries []MemoryEntry, query string, at time.Time, limit int) []MemoryEntry {
	if limit <= 0 {
		limit = DefaultRecallLimit
	}
	filtered := make([]MemoryEntry, 0, len(entries))
	for _, e := range entries {
		if !SoftForgotten(e, at) {
			filtered = append(filtered, e)
		}
	}
	ranked := make([]scored, 0, len(filtered))
	for _, e := range filtered {
		ranked = append(ranked, scored{entry: e, score: Score(e, query, at)})
	}
	// insertion sort: small N, stable and allocation-free comparisons
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && lessRanked(ranked[j], ranked[j-1]); j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]MemoryEntry, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.entry)
	}
	return out
}

// scored pairs an entry with its lazily settled retrieval score.
type scored struct {
	entry MemoryEntry
	score float64
}

func lessRanked(a, b scored) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if a.entry.Importance != b.entry.Importance {
		return a.entry.Importance > b.entry.Importance
	}
	if !a.entry.RecordedAt.Equal(b.entry.RecordedAt) {
		return a.entry.RecordedAt.After(b.entry.RecordedAt)
	}
	return a.entry.ID < b.entry.ID
}
