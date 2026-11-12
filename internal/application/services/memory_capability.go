package services

// memory_capability.go implements the server-verifiable per-agent capability
// for the built-in memory MCP (spec docs/superpowers/specs/2026-11-11-memory
// -mcp-design.md §4), mirroring knowledge_capability.go: HMAC-signed
// capability issued at deploy time, binding tenant ID, agent name, scope
// mode, allowed tool set, and the runtime token's fingerprint. Only the hub
// holds the signing key (systemsetting.EnsureMemoryCapabilitySecret), so a
// leaked token alone cannot claim another agent's memory scope.
//
// Wire format: "v1." + base64url(payloadJSON) + "." + base64url(hmac).
// The HMAC covers the exact payload JSON bytes. The version lives in the
// prefix and the HKDF info string (no payload field) — a future v2 uses a
// different info derivation and therefore a different key.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// ErrMemoryCapabilityDenied marks a presented capability as invalid or
// binding-mismatched. Handlers map it to a neutral deny message; the
// rejection reason stays in server-side logs only (no probing oracle).
var ErrMemoryCapabilityDenied = errors.New("memory capability denied")

const (
	// memoryCapabilityHeader mirrors the handler-side constant of the same
	// name (internal/handler memory_mcp.go): the signed per-agent capability
	// carried on the built-in memory MCP's connection headers. Both packages
	// hold their own copy — changes must stay in sync (same convention as
	// the knowledge capability header).
	memoryCapabilityHeader = "X-Agent-Capability"

	memoryCapabilityPrefix = "v1"

	// memoryCapabilityInfo is the HKDF info string: domain separation from
	// every other HKDF use of the capability secret (including knowledge's).
	memoryCapabilityInfo = "agent-hub/memory-capability/v1"

	memoryCapabilityKeyLen = 32
)

// MemoryCapabilityPayload is the signed statement inside a memory
// capability. Tools is the allowed tool set; ScopeMode is pinned to "agent"
// in Phase 1 (user mode arrives with runtime#95).
type MemoryCapabilityPayload struct {
	TenantID         string   `json:"t"`
	AgentName        string   `json:"a"`
	ScopeMode        string   `json:"m"`
	Tools            []string `json:"o,omitempty"`
	TokenFingerprint string   `json:"f"`
}

// MemoryCapabilityService signs and verifies memory capabilities against a
// server-held secret. The capability value is a credential: never persist
// it, never log it.
type MemoryCapabilityService struct {
	secret []byte
}

func NewMemoryCapabilityService(secret string) *MemoryCapabilityService {
	return &MemoryCapabilityService{secret: []byte(secret)}
}

// memoryCapabilityKey derives the signing key:
// HKDF-SHA256(secret, salt=nil, info="agent-hub/memory-capability/v1").
// Empty secret yields nil — callers fail closed (HKDF(empty) is publicly
// computable and would make capabilities forgeable).
func memoryCapabilityKey(secret []byte) []byte {
	if len(secret) == 0 {
		return nil
	}
	key := make([]byte, memoryCapabilityKeyLen)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, nil, []byte(memoryCapabilityInfo)), key); err != nil {
		return nil
	}
	return key
}

func (s *MemoryCapabilityService) Sign(p MemoryCapabilityPayload) (string, error) {
	key := memoryCapabilityKey(s.secret)
	if key == nil {
		return "", errors.New("memory capability signing secret is not configured")
	}
	payloadJSON, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal memory capability payload: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadJSON)
	return memoryCapabilityPrefix + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// VerifyWithToken 由 bearer 原文计算 token 指纹后执行 Verify——指纹公式
// （sha256hex[:16]，与 knowledge capability 的 tokenFingerprint 同式）的
// 唯一来源保持在 services 包内，调用方不自行复制。
func (s *MemoryCapabilityService) VerifyWithToken(raw, agentName, bearerToken string) (MemoryCapabilityPayload, error) {
	return s.Verify(raw, agentName, tokenFingerprint(bearerToken))
}

// AgentMemoryToolNames is the single source of the memory MCP tool set —
// the handler's tools/list schema, the switch dispatch, the deployer's
// capability payload, and tests all derive from this list (review 重要项 9):
// drift between a broadcast schema and runtime denial is the failure mode
// this consolidation eliminates. Add-only contract; pinned by
// TestAgentMemoryToolNamesComplete in the handler tests.
var AgentMemoryToolNames = []string{"memory_remember", "memory_recall", "memory_list", "memory_update"}

// Verify runs the full chain for one presented capability: format (three
// dot-separated segments, v1 prefix), HMAC over the payload bytes, and the
// agent-name / token-fingerprint bindings. Tenant / scopeMode / tools are
// returned for the caller to check against its authenticated context (the
// MCP handler needs them for scope derivation anyway). Any failure denies;
// there is no fallback for a presented-but-invalid capability.
func (s *MemoryCapabilityService) Verify(raw, agentName, tokenFP string) (MemoryCapabilityPayload, error) {
	deny := func(reason string) (MemoryCapabilityPayload, error) {
		return MemoryCapabilityPayload{}, fmt.Errorf("%w: %s", ErrMemoryCapabilityDenied, reason)
	}
	key := memoryCapabilityKey(s.secret)
	if key == nil {
		return deny("signing secret is not configured (fail-closed)")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return deny(fmt.Sprintf("invalid capability format: %d segments", len(parts)))
	}
	if parts[0] != memoryCapabilityPrefix {
		return deny(fmt.Sprintf("invalid capability version prefix: %q", parts[0]))
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return deny("decode capability payload")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return deny("decode capability signature")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadJSON)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return deny("capability signature mismatch")
	}
	var p MemoryCapabilityPayload
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return deny("parse capability payload")
	}
	if p.AgentName != agentName {
		return deny("capability agent binding mismatch")
	}
	if p.TokenFingerprint != tokenFP {
		return deny("capability token fingerprint mismatch (token may have rotated)")
	}
	return p, nil
}
