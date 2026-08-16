package auth

import (
	authdom "control-panel/internal/domain/auth"
)

// MembershipOp is the persistence action implied by a MembershipDecision.
type MembershipOp int

const (
	OpNone MembershipOp = iota
	OpCreate
	OpUpdate
)

// MembershipDecision is the outcome of role synthesis for one login/lookup.
type MembershipDecision struct {
	Role   string // final role; "" means unassigned (pending)
	Status string // final status
	Op     MembershipOp
}

// SynthesizeMembership combines the casdoor organization-admin flag with the
// local membership record into the authoritative role/status. See spec
// decisions 1/2/3/8 for the rule matrix.
func SynthesizeMembership(isAdmin bool, rec *authdom.UserIdentity) MembershipDecision {
	role, status := "", authdom.StatusPending
	if rec != nil {
		role, status = rec.Role, rec.Status
		if role != "" && !authdom.IsValidRole(role) {
			// decision 8: any non-empty value outside admin/maintainer/member
			// is treated as unassigned.
			role = ""
		}
	}
	switch {
	case isAdmin && role == authdom.RoleAdmin && status == authdom.StatusActive:
		return MembershipDecision{Role: role, Status: status, Op: OpNone}
	case isAdmin:
		return MembershipDecision{Role: authdom.RoleAdmin, Status: authdom.StatusActive, Op: opFor(rec)}
	case role == authdom.RoleAdmin:
		// decision 3: casdoor org-admin demotion revokes local admin.
		return MembershipDecision{Role: "", Status: authdom.StatusPending, Op: OpUpdate}
	case role == "":
		return MembershipDecision{Role: "", Status: authdom.StatusPending, Op: opFor(rec)}
	default:
		return MembershipDecision{Role: role, Status: status, Op: OpNone}
	}
}

func opFor(rec *authdom.UserIdentity) MembershipOp {
	if rec == nil {
		return OpCreate
	}
	return OpUpdate
}
