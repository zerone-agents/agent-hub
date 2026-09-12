package agentrelation

import "testing"

func TestBuiltInRelationshipRuleKeepsCompatibleVersionAndDeclaresSource(t *testing.T) {
	if RelationshipRuleSource != "builtin:relationship-dynamics" {
		t.Fatalf("unexpected relationship rule source %q", RelationshipRuleSource)
	}
	if RelationshipRuleV1 != "v1" {
		t.Fatalf("persisted compatibility version changed to %q", RelationshipRuleV1)
	}
}

func TestRelationshipScoreProjectionAndBounds(t *testing.T) {
	tests := []struct {
		score  int
		stance string
	}{
		{100, "allied"}, {70, "allied"}, {69, "friendly"}, {25, "friendly"},
		{24, "neutral"}, {-24, "neutral"}, {-25, "wary"}, {-45, "competitive"},
		{-70, "hostile"}, {-100, "hostile"},
	}
	for _, tt := range tests {
		if got := StanceForScore(tt.score); got != tt.stance {
			t.Fatalf("StanceForScore(%d) = %q, want %q", tt.score, got, tt.stance)
		}
	}
	if ClampRelationshipScore(140) != 100 || ClampRelationshipScore(-140) != -100 {
		t.Fatal("relationship score must be clamped to [-100, 100]")
	}
}

func TestEventDeltaIsSeverityWeightedAndCapped(t *testing.T) {
	delta, ok := EventDelta("protected", 2)
	if !ok || delta != 27 {
		t.Fatalf("protected severity 2 = %d, %v; want 27, true", delta, ok)
	}
	delta, ok = EventDelta("betrayed", 3)
	if !ok || delta != -40 {
		t.Fatalf("betrayed severity 3 = %d, %v; want -40, true", delta, ok)
	}
}
