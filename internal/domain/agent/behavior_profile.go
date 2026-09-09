package agent

// BehaviorProfileVersion is incremented when the persisted behavior schema
// changes in a way that requires a different runtime projection.
const BehaviorProfileVersion = 1

// BehaviorProfile stores stable behavioral tendencies independently from an
// agent's authored identity and task prompt. All scores use a 0-100 scale.
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

// DefaultBehaviorProfile is the conservative baseline used by the creation
// UI. Existing agents keep a nil profile until an administrator explicitly
// saves one, so upgrading the Hub does not silently change deployed behavior.
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
