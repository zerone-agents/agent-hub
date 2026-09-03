package services

// knowledge_capability.go implements the server-verifiable per-agent
// capability for the built-in knowledge MCP (issue #111, reopened). It
// replaces the spoofable bare identity header: the hub now issues an
// HMAC-signed capability per graph node at deploy time, binding tenant ID,
// root deployment identity, requesting agent identity, and the runtime
// token's fingerprint. Only the hub holds the signing key (derived from the
// dedicated knowledge capability secret — see
// systemsetting.EnsureKnowledgeCapabilitySecret, decoupled from the
// provider credential encryption key), so a leaked token alone can no
// longer claim a sibling's or child's identity — the capability never
// leaves the deployment boundary and rotates with the token.
//
// Wire format: "v1." + base64url(payloadJSON) + "." + base64url(hmac).
// The HMAC covers the exact payload JSON bytes, so any content change —
// including single-character case flips — breaks verification.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// ErrKnowledgeCapabilityDenied marks a presented capability as invalid or
// binding-mismatched. The MCP handler maps it to the same neutral
// "无权访问" message as dataset-subset denials — the failure reason stays
// in server-side logs only (no probing oracle).
var ErrKnowledgeCapabilityDenied = errors.New("knowledge capability denied")

const (
	// knowledgeCapabilityHeader mirrors the handler-side constant of the
	// same name (internal/handler knowledge_mcp.go): the signed per-agent
	// capability carried on the built-in knowledge MCP's connection
	// headers. Both packages hold their own copy — changes must stay in
	// sync. The legacy bare identity header is no longer injected or
	// consumed.
	knowledgeCapabilityHeader = "X-Agent-Capability"

	// knowledgeCapabilityPrefix is the leading segment of the wire format
	// and doubles as the failure-domain version marker.
	knowledgeCapabilityPrefix = "v1"

	// knowledgeCapabilityInfo is the HKDF info string: domain separation
	// so the capability key is a distinct secret from every other HKDF
	// use of the capability secret.
	knowledgeCapabilityInfo = "agent-hub/knowledge-capability/v1"

	// knowledgeCapabilityKeyLen is the HKDF output size (SHA-256).
	knowledgeCapabilityKeyLen = 32
)

// knowledgeCapabilityPayload is the signed statement inside a capability.
type knowledgeCapabilityPayload struct {
	Version int    `json:"v"` // 1; explicit failure domain, verified even under a valid signature
	Tenant  string `json:"t"` // tenant ID the capability is valid in
	Dep     string `json:"d"` // root deployment key (scoped, #117 contract)
	Agent   string `json:"a"` // requesting agent's DB bare name
	TokenFp string `json:"f"` // sha256hex(runtime token)[:16] — rotation invalidates
}

// knowledgeCapabilityKey derives the capability signing key:
// HKDF-SHA256(capabilitySecret, salt=nil, info="agent-hub/knowledge-capability/v1"),
// 32 bytes. The secret comes from
// systemsetting.EnsureKnowledgeCapabilitySecret (explicit config or a
// persisted random value) and is never empty in production. With no
// server-held secret (empty) it returns nil — callers must fail closed,
// because HKDF(empty) is publicly computable and would make capabilities
// forgeable. Reading 32 bytes from hkdf.New cannot fail (SHA-256 expands
// to 255 blocks); the error branch is defensive.
func knowledgeCapabilityKey(secret []byte) []byte {
	if len(secret) == 0 {
		return nil
	}
	key := make([]byte, knowledgeCapabilityKeyLen)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, nil, []byte(knowledgeCapabilityInfo)), key); err != nil {
		return nil
	}
	return key
}

// tokenFingerprint is the capability's token binding: the first 16 hex
// chars of sha256(token), computed identically on the issue and verify
// sides. Token rotation therefore invalidates every capability of the
// deployment without hub-side state.
func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

// issueKnowledgeCapability signs a payload and returns the wire string
// "v1.<payload>.<hmac>". Callers must guard against an empty secret first
// (a nil derived key yields a value that verification always rejects, so
// the failure stays closed on the verify side as well). The returned value
// is a credential: never persist it, never log it.
func issueKnowledgeCapability(secret []byte, p knowledgeCapabilityPayload) string {
	key := knowledgeCapabilityKey(secret)
	if key == nil {
		return ""
	}
	payloadJSON, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadJSON)
	return knowledgeCapabilityPrefix + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyKnowledgeCapability runs the full verification chain for one
// presented capability and returns the bound agent name. Checks: format
// (three dot-separated segments, v1 prefix), HMAC over the payload bytes,
// payload version, and the four binding invariants — tenant, root
// deployment key, token fingerprint, and payload.Agent ∈ allowedAgents
// (the token agent itself plus its direct subagents). Any failure denies;
// there is deliberately no fallback for a presented-but-invalid capability.
// An empty capabilitySecret always denies (defense in depth — the secret
// is provisioned at startup and never empty in production). Errors
// describe the rejection reason for server-side logs and never contain
// the capability value.
func verifyKnowledgeCapability(secret []byte, cap string, expectTenant, expectDep, expectTokenFp string, allowedAgents map[string]struct{}) (string, error) {
	key := knowledgeCapabilityKey(secret)
	if key == nil {
		// Internal detail: logged server-side, rendered as a neutral deny
		// at the handler boundary (CONTRIBUTING: user-visible text is
		// Chinese, internals are English).
		return "", errors.New("capability verification denied: signing secret is not configured (fail-closed)")
	}
	parts := strings.Split(cap, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid capability format: %d segments", len(parts))
	}
	if parts[0] != knowledgeCapabilityPrefix {
		return "", fmt.Errorf("invalid capability version prefix: %q", parts[0])
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode capability payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode capability signature: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadJSON)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return "", errors.New("capability signature mismatch")
	}
	var p knowledgeCapabilityPayload
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return "", fmt.Errorf("parse capability payload: %w", err)
	}
	if p.Version != 1 {
		return "", fmt.Errorf("unsupported capability version: %d", p.Version)
	}
	if p.Tenant != expectTenant {
		return "", errors.New("capability tenant binding mismatch")
	}
	if p.Dep != expectDep {
		return "", errors.New("capability deployment binding mismatch")
	}
	if p.TokenFp != expectTokenFp {
		return "", errors.New("capability token fingerprint mismatch (token may have rotated)")
	}
	if _, ok := allowedAgents[p.Agent]; !ok {
		return "", fmt.Errorf("capability identity %q is not in the token agent's deployment graph", p.Agent)
	}
	return p.Agent, nil
}
