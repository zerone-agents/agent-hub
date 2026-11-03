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
//
// isFirstInTenant (issue #238 bootstrap): true when the tenant has no active
// member yet. The first user in a brand-new tenant then becomes its admin
// (founder) — no superadmin approval needed, which removes the
// "new tenant has no admin to approve its first user" deadlock. Only fires
// for unassigned records (role == ""): an existing admin/member/guest is
// never re-granted or revoked by bootstrap, and a bootstrap admin follows
// the same bidirectional revocation semantics as any other admin.
// Callers compute it (one COUNT query) only for new/pending records;
// query failure must fail closed (false).
func SynthesizeMembership(isAdmin bool, rec *authdom.UserIdentity, isFirstInTenant bool) MembershipDecision {
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
	case isFirstInTenant && role == "":
		// issue #238 bootstrap: first user in a tenant with no active member
		// becomes its admin. Restricted to unassigned records so an existing
		// admin is still revoked by casdoor demotion (case below) and members
		// are never re-granted.
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

// firstInTenant computes the bootstrap predicate (#238): whether the tenant
// currently has NO active member. Only worth querying for new/pending
// records — any active record means the tenant already has one (the record
// itself). Query failure returns an error; callers must fail closed (treat
// as not-first), never granting admin on an unknown count.
func firstInTenant(store MembershipStore, tenantID string, rec *authdom.UserIdentity) (bool, error) {
	if rec != nil && rec.Status == authdom.StatusActive {
		return false, nil
	}
	n, err := store.CountActiveByTenant(tenantID)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}
