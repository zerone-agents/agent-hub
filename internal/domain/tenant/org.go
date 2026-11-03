package tenant

import "regexp"

// OrgNamePattern is THE org naming contract (issue #235): lowercase letter
// start, lowercase letters/digits/hyphens, with hyphens only between
// alphanumerics — no leading, trailing, or consecutive hyphens.
//
// Single source of truth: BOTH the ops registration entry
// (handler/ops_tenant_client.go) and the runtime proxy gate
// (services/runtime_proxy.go) must reference this pattern. The contract
// used to be declared twice with only a comment linking the copies; during
// #235 the copies drifted (ops allowed hyphens while the proxy still
// rejected them), so hyphenated orgs could register and deploy but 404'd
// at the proxy — a half-shipped feature caught in review. Never duplicate
// this contract again; extend it here.
var OrgNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// MaxOrgNameLen caps org names at 63 characters — URL path segment and
// deploy-key segment safety.
const MaxOrgNameLen = 63

// ValidateOrgName reports whether org satisfies the naming contract
// (shape + length).
func ValidateOrgName(org string) bool {
	return len(org) > 0 && len(org) <= MaxOrgNameLen && OrgNamePattern.MatchString(org)
}
