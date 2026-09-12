package agent

// BehaviorProfileVersion is incremented when the persisted behavior schema
// changes in a way that requires a different runtime projection.
const BehaviorProfileVersion = 1

// BehaviorProfile is a deprecated compatibility projection of a prompt-first
// personality. New runtime capabilities must not use it as their source of
// truth; it remains persisted only for legacy clients, filtering, and stable
// prompt rendering while those callers migrate. All scores use a 0-100 scale.
//
// The profile is deliberately descriptive rather than permissive: it may
// influence how an agent makes a decision, but it never grants tools, data
// access, or permission to bypass organization relation policies.
type BehaviorProfile struct {
	Version             int `json:"version"`
	HierarchyCompliance int `json:"hierarchyCompliance"`
	Ambition            int `json:"ambition"`
	Whistleblowing      int `json:"whistleblowing"`
	RiskTolerance       int `json:"riskTolerance"`
	ConflictAvoidance   int `json:"conflictAvoidance"`
	Secrecy             int `json:"secrecy"`
	SelfInterest        int `json:"selfInterest"`
	EscalationThreshold int `json:"escalationThreshold"`
}

// DefaultBehaviorProfile returns the legacy compatibility projection used by
// built-in personality seeds. New Agent creation flows should select a
// prompt-first personality template instead of editing this projection.
func DefaultBehaviorProfile() BehaviorProfile {
	return BehaviorProfile{
		Version:             BehaviorProfileVersion,
		HierarchyCompliance: 75,
		Ambition:            30,
		Whistleblowing:      55,
		RiskTolerance:       35,
		ConflictAvoidance:   60,
		Secrecy:             55,
		SelfInterest:        35,
		EscalationThreshold: 75,
	}
}
