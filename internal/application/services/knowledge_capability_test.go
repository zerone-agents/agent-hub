package services

// knowledge_capability_test.go pins the issue #111 (reopened) per-agent
// knowledge capability at the crypto layer: HKDF-SHA256 key derivation with
// RFC 5869 vectors, token fingerprint, issue/verify round trip, and the
// attack matrix — tamper (payload/sig/case), cross-tenant, cross-root,
// token rotation, closure escape, malformed input, and the empty-key
// fail-closed policy. Handler-level plumbing and deploy-time injection are
// covered by knowledge_mcp_test.go and agent_deployer_graph_test.go.

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"control-panel/internal/infrastructure/deployer"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/hkdf"
)

// testKnowledgeEncKey mirrors a production provider.encryption_key (hex
// string, 32 bytes = 64 hex chars). Both the deployer-side issuer and the
// service-side verifier derive the signing key from the same value. Must
// stay valid hex: the deploy path also AES-encrypts the runtime token with
// it (providerdomain.Encrypt hex-decodes).
const testKnowledgeEncKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestKnowledgeCapabilityKey_HKDFRFC5869Vectors pins our hkdf usage against
// the RFC 5869 SHA-256 test vectors (TC1 basic + TC2 longer inputs). The
// vectors exercise hkdf.New exactly the way knowledgeCapabilityKey calls it
// (extract-then-expand, explicit salt/info, io.ReadFull), so an accidental
// argument swap or a broken hkdf version fails here instead of silently
// changing every issued capability.
func TestKnowledgeCapabilityKey_HKDFRFC5869Vectors(t *testing.T) {
	seq := func(start byte, n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = start + byte(i)
		}
		return b
	}
	cases := []struct {
		name    string
		ikm     []byte
		salt    []byte
		info    []byte
		length  int
		wantOKM string
	}{
		{
			name:   "TC1 basic",
			ikm:    bytes.Repeat([]byte{0x0b}, 22),
			salt:   seq(0x00, 13),
			info:   seq(0xf0, 10),
			length: 42,
			wantOKM: "3cb25f25faacd57a90434f64d0362f2a" +
				"2d2d0a90cf1a5a4c5db02d56ecc4c5bf" +
				"34007208d5b887185865",
		},
		{
			name:   "TC2 longer inputs",
			ikm:    seq(0x00, 80),
			salt:   seq(0x60, 80),
			info:   seq(0xb0, 80),
			length: 82,
			wantOKM: "b11e398dc80327a1c8e7f78c596a4934" +
				"4f012eda2d4efad8a050cc4c19afa97c" +
				"59045a99cac7827271cb41c65e590e09" +
				"da3275600c2f09b8367793a9aca3db71" +
				"cc30c58179ec3e87c14c01d5c1f3434f" +
				"1d87",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			okm := make([]byte, tc.length)
			_, err := io.ReadFull(hkdf.New(sha256.New, tc.ikm, tc.salt, tc.info), okm)
			require.NoError(t, err)
			require.Equal(t, tc.wantOKM, hex.EncodeToString(okm))
		})
	}
}

// TestKnowledgeCapabilityKey_Properties locks the derivation contract:
// 32 bytes, deterministic for a given encKey, and different for different
// encKeys (or info strings) — never the raw encKey itself.
func TestKnowledgeCapabilityKey_Properties(t *testing.T) {
	enc := []byte(testKnowledgeEncKey)
	k1 := knowledgeCapabilityKey(enc)
	k2 := knowledgeCapabilityKey(enc)
	require.Len(t, k1, 32)
	require.Equal(t, k1, k2, "derivation must be deterministic")
	require.NotEqual(t, enc, k1, "derived key must not be the raw encKey")
	other := knowledgeCapabilityKey([]byte("0f1e2d3c4b5a69788796a5b4c3d2e1f0" + "0f1e2d3c4b5a69788796a5b4c3d2e1f0"))
	require.NotEqual(t, k1, other, "different encKeys must derive different keys")
	// The info string is domain separation: changing it changes the key.
	r := hkdf.New(sha256.New, enc, nil, []byte("some-other-info/v2"))
	alt := make([]byte, 32)
	_, err := io.ReadFull(r, alt)
	require.NoError(t, err)
	require.NotEqual(t, k1, alt)
}

// TestKnowledgeCapabilityKey_EmptyKeyFailsClosed: with no server-held secret
// the derivation must fail closed (nil key) — an HKDF(empty) key would be
// publicly computable and capabilities forgeable.
func TestKnowledgeCapabilityKey_EmptyKeyFailsClosed(t *testing.T) {
	require.Nil(t, knowledgeCapabilityKey(nil))
	require.Nil(t, knowledgeCapabilityKey([]byte{}))
}

// TestTokenFingerprint locks the TokenFp shape: the first 16 hex chars of
// sha256(token), identical on the issue and verify sides (same function).
func TestTokenFingerprint(t *testing.T) {
	fp := tokenFingerprint("runtime-token-a")
	require.Len(t, fp, 16)
	sum := sha256.Sum256([]byte("runtime-token-a"))
	require.Equal(t, hex.EncodeToString(sum[:])[:16], fp)
	require.NotEqual(t, fp, tokenFingerprint("runtime-token-b"))
	// Both tokens below collapse to the same fingerprint shape even for
	// near-identical tokens (rotation produces a wholly different fp).
	require.Len(t, tokenFingerprint("x"), 16)
}

// capabilityFixture is the shared universe for the crypto attack matrix:
// tenant-a/root with child-a + child-b mounted, deployment key
// "tenant-a-root", and one runtime token.
type capabilityFixture struct {
	encKey  []byte
	tenant  string
	dep     string
	token   string
	allowed map[string]struct{}
}

func newCapabilityFixture() *capabilityFixture {
	return &capabilityFixture{
		encKey: []byte(testKnowledgeEncKey),
		tenant: "tenant-a",
		dep:    "tenant-a-root",
		token:  "0123456789abcdef0123456789abcdef",
		allowed: map[string]struct{}{
			"root":    {},
			"child-a": {},
			"child-b": {},
		},
	}
}

func (f *capabilityFixture) payloadFor(agent string) knowledgeCapabilityPayload {
	return knowledgeCapabilityPayload{
		Version: 1,
		Tenant:  f.tenant,
		Dep:     f.dep,
		Agent:   agent,
		TokenFp: tokenFingerprint(f.token),
	}
}

// TestKnowledgeCapabilityRoundTrip: issued capabilities for root and child
// nodes verify against the matching expectations and return the payload's
// agent name (matrix items 1/2 at the crypto layer).
func TestKnowledgeCapabilityRoundTrip(t *testing.T) {
	f := newCapabilityFixture()
	for _, agent := range []string{"root", "child-a", "child-b"} {
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor(agent))
		require.True(t, strings.HasPrefix(cap, "v1."), "capability must carry the v1. prefix: %q", cap)
		name, err := verifyKnowledgeCapability(f.encKey, cap, f.tenant, f.dep, tokenFingerprint(f.token), f.allowed)
		require.NoError(t, err, "agent %s round trip", agent)
		require.Equal(t, agent, name)
	}
}

// TestKnowledgeCapabilityFormat: the wire format is
// "v1." + base64url(payloadJSON) + "." + base64url(hmacSHA256) — three dot
// separated segments, unpadded base64url, and the payload decodes to the
// pinned JSON field names (v/t/d/a/f).
func TestKnowledgeCapabilityFormat(t *testing.T) {
	f := newCapabilityFixture()
	cap := issueKnowledgeCapability(f.encKey, f.payloadFor("child-a"))
	parts := strings.Split(cap, ".")
	require.Len(t, parts, 3)
	require.Equal(t, "v1", parts[0])
	require.NotContains(t, cap, "=", "base64url must be unpadded")
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &raw))
	require.Equal(t, float64(1), raw["v"])
	require.Equal(t, "tenant-a", raw["t"])
	require.Equal(t, "tenant-a-root", raw["d"])
	require.Equal(t, "child-a", raw["a"])
	require.Equal(t, tokenFingerprint(f.token), raw["f"])
	_, err = base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
}

// TestKnowledgeCapabilityTamperRejected (matrix item 5): every mutation of
// the capability content — payload agent swap keeping the signature, single
// character flips in payload or signature, case changes anywhere — breaks
// verification. A correctly signed payload with a bumped version is also
// rejected (version is an explicit failure domain, not cosmetic).
func TestKnowledgeCapabilityTamperRejected(t *testing.T) {
	f := newCapabilityFixture()
	good := issueKnowledgeCapability(f.encKey, f.payloadFor("child-a"))
	parts := strings.Split(good, ".")

	tamperCase := func(name, capability string) {
		t.Helper()
		_, err := verifyKnowledgeCapability(f.encKey, capability, f.tenant, f.dep, tokenFingerprint(f.token), f.allowed)
		require.Error(t, err, "%s must be rejected", name)
	}

	// Swap the payload's agent id but keep the original signature.
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	forged := strings.Replace(string(payloadJSON), `"a":"child-a"`, `"a":"root"`, 1)
	require.NotEqual(t, string(payloadJSON), forged, "fixture payload must contain the agent field")
	tamperCase("payload agent swap (sig kept)", parts[0]+"."+base64.RawURLEncoding.EncodeToString([]byte(forged))+"."+parts[2])

	// Single-character case flip inside the payload segment.
	flipped := []byte(parts[1])
	flipped[0] = flipCase(flipped[0])
	tamperCase("payload case change", parts[0]+"."+string(flipped)+"."+parts[2])

	// Single-character case flip inside the signature segment. The first
	// base64 char may be a digit (flip is a no-op), so flip the first
	// letter found — a 43-char base64url string always contains one.
	sigFlipped := []byte(parts[2])
	didFlip := false
	for i, b := range sigFlipped {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
			sigFlipped[i] = flipCase(b)
			didFlip = true
			break
		}
	}
	require.True(t, didFlip, "signature segment must contain a letter to flip")
	tamperCase("signature case change", parts[0]+"."+parts[1]+"."+string(sigFlipped))

	// Prefix case change (V1. instead of v1.).
	tamperCase("prefix case change", "V1."+parts[1]+"."+parts[2])

	// Truncation and malformed shapes.
	tamperCase("missing signature", parts[0]+"."+parts[1])
	tamperCase("empty capability", "")
	tamperCase("garbage", "not-a-capability")
	tamperCase("wrong prefix", "v2."+parts[1]+"."+parts[2])
	tamperCase("non-base64 payload", "v1.!!!."+parts[2])
	tamperCase("non-json payload", "v1."+base64.RawURLEncoding.EncodeToString([]byte("not json"))+"."+parts[2])

	// Correctly signed but wrong version: the failure domain check applies
	// even with a valid signature (a future v2 must not verify as v1).
	p := f.payloadFor("root")
	p.Version = 2
	tamperCase("signed but wrong version", issueKnowledgeCapability(f.encKey, p))
}

func flipCase(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 'a' + 'A'
	}
	if b >= 'A' && b <= 'Z' {
		return b - 'A' + 'a'
	}
	return b
}

// TestKnowledgeCapabilityBindingMatrix (matrix items 3/5/8 at the crypto
// layer): a valid signature is necessary but not sufficient — the payload
// must bind to the expected tenant, the token agent's deployment key, the
// presented token's fingerprint, and an identity inside the token agent's
// closure. Any mismatch denies.
func TestKnowledgeCapabilityBindingMatrix(t *testing.T) {
	f := newCapabilityFixture()

	t.Run("cross tenant", func(t *testing.T) {
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor("root"))
		_, err := verifyKnowledgeCapability(f.encKey, cap, "tenant-b", f.dep, tokenFingerprint(f.token), f.allowed)
		require.Error(t, err)
	})

	t.Run("cross root deployment", func(t *testing.T) {
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor("child-a"))
		_, err := verifyKnowledgeCapability(f.encKey, cap, f.tenant, "tenant-b-root", tokenFingerprint(f.token), f.allowed)
		require.Error(t, err)
	})

	t.Run("token rotation invalidates old capability", func(t *testing.T) {
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor("root"))
		newToken := "ffffffffffffffffffffffffffffffff"
		require.NotEqual(t, f.token, newToken)
		_, err := verifyKnowledgeCapability(f.encKey, cap, f.tenant, f.dep, tokenFingerprint(newToken), f.allowed)
		require.Error(t, err)
	})

	t.Run("agent outside the token agent closure", func(t *testing.T) {
		// Correctly signed for "stranger", but the allowed set is
		// {root, child-a, child-b}.
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor("stranger"))
		_, err := verifyKnowledgeCapability(f.encKey, cap, f.tenant, f.dep, tokenFingerprint(f.token), f.allowed)
		require.Error(t, err)
	})

	t.Run("empty allowed set denies even the token agent itself", func(t *testing.T) {
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor("root"))
		_, err := verifyKnowledgeCapability(f.encKey, cap, f.tenant, f.dep, tokenFingerprint(f.token), map[string]struct{}{})
		require.Error(t, err)
	})

	t.Run("different encKey cannot verify", func(t *testing.T) {
		cap := issueKnowledgeCapability(f.encKey, f.payloadFor("root"))
		_, err := verifyKnowledgeCapability([]byte("0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"), cap, f.tenant, f.dep, tokenFingerprint(f.token), f.allowed)
		require.Error(t, err)
	})
}

// TestKnowledgeCapabilityVerifyEmptyKeyFailsClosed: with no server-held
// secret, verification must deny every presented capability — a publicly
// computable HKDF(empty) key would make capabilities forgeable, so the
// primitive refuses to verify rather than degrade to a public key.
func TestKnowledgeCapabilityVerifyEmptyKeyFailsClosed(t *testing.T) {
	f := newCapabilityFixture()
	cap := issueKnowledgeCapability(f.encKey, f.payloadFor("root"))
	_, err := verifyKnowledgeCapability(nil, cap, f.tenant, f.dep, tokenFingerprint(f.token), f.allowed)
	require.Error(t, err, "empty encKey must fail closed")
	_, err = verifyKnowledgeCapability([]byte{}, cap, f.tenant, f.dep, tokenFingerprint(f.token), f.allowed)
	require.Error(t, err, "empty encKey must fail closed")
}

// capabilityRequestFixture builds a two-node create request (root +
// child-a) where both nodes mount the built-in knowledge MCP; the root's
// copy carries a stale same-named key configured on the MCP server (the
// deployment must override it) and a regular header that must survive.
// The root also mounts an unrelated MCP that must stay untouched.
func capabilityRequestFixture() *deployer.CreateAgentRequest {
	return &deployer.CreateAgentRequest{
		DeploymentKey: "tenant-a-root",
		RuntimeToken:  "0123456789abcdef0123456789abcdef",
		Agents: []deployer.AgentDefinition{
			{
				Name: "root",
				McpServers: map[string]deployer.McpServerConfig{
					"knowledge": {
						Type: "http", URL: "https://hub.example.com/api/v1/knowledge/mcp",
						Headers: map[string]string{
							"Authorization":           "Bearer $agent_runtime_token",
							knowledgeCapabilityHeader: "stale-configured-capability",
						},
					},
					"weather": {Type: "http", URL: "https://mcp.example.com/weather"},
				},
			},
			{
				Name: "child-a",
				McpServers: map[string]deployer.McpServerConfig{
					"knowledge": {
						Type: "http", URL: "https://hub.example.com/api/v1/knowledge/mcp",
						Headers: map[string]string{
							"Authorization": "Bearer $agent_runtime_token",
						},
					},
				},
			},
		},
	}
}

// TestAttachKnowledgeCapabilities (matrix item 9 at the unit layer): the
// deployment injects a per-node X-Agent-Capability on every knowledge MCP,
// overrides a user-configured same-named key, leaves unrelated MCPs and
// headers untouched, mints distinct capabilities per node (same token,
// different agent binding), and the issued values verify against the
// deployment's tenant/key/token triple. X-Agent-Id is never injected.
func TestAttachKnowledgeCapabilities(t *testing.T) {
	s := &AgentDeployerService{encryptionKey: testKnowledgeEncKey}
	req := capabilityRequestFixture()
	token := "0123456789abcdef0123456789abcdef"

	require.NoError(t, s.attachKnowledgeCapabilities(req, "tenant-a", "tenant-a-root", token))

	rootKnowledge := req.Agents[0].McpServers["knowledge"]
	childKnowledge := req.Agents[1].McpServers["knowledge"]

	rootCap := rootKnowledge.Headers[knowledgeCapabilityHeader]
	childCap := childKnowledge.Headers[knowledgeCapabilityHeader]
	require.NotEqual(t, "stale-configured-capability", rootCap, "stale same-named key must be overridden")
	require.True(t, strings.HasPrefix(rootCap, "v1."), "root capability must be v1.-prefixed")
	require.True(t, strings.HasPrefix(childCap, "v1."), "child capability must be v1.-prefixed")
	require.NotEqual(t, rootCap, childCap, "root and child capabilities must differ (agent binding)")
	require.Equal(t, "Bearer $agent_runtime_token", rootKnowledge.Headers["Authorization"],
		"token placeholder substitution stays resolveMcpHeaders' job")

	// X-Agent-Id is gone everywhere.
	for _, node := range req.Agents {
		for _, mcp := range node.McpServers {
			_, hasID := mcp.Headers["X-Agent-Id"]
			require.False(t, hasID, "X-Agent-Id must never be injected")
		}
	}
	// Unrelated MCP stays untouched (no headers materialized).
	weather := req.Agents[0].McpServers["weather"]
	require.Empty(t, weather.Headers, "non-knowledge MCP must not gain headers")

	// The issued values carry the full binding triple.
	allowed := map[string]struct{}{"root": {}, "child-a": {}}
	name, err := verifyKnowledgeCapability([]byte(testKnowledgeEncKey), rootCap, "tenant-a", "tenant-a-root", tokenFingerprint(token), allowed)
	require.NoError(t, err)
	require.Equal(t, "root", name)
	name, err = verifyKnowledgeCapability([]byte(testKnowledgeEncKey), childCap, "tenant-a", "tenant-a-root", tokenFingerprint(token), allowed)
	require.NoError(t, err)
	require.Equal(t, "child-a", name)
}

// TestAttachKnowledgeCapabilities_RequiresEncryptionKey: issuing needs a
// server-held secret — a graph with a knowledge MCP and no configured
// provider.encryption_key fails the deploy instead of shipping publicly
// forgeable capabilities.
func TestAttachKnowledgeCapabilities_RequiresEncryptionKey(t *testing.T) {
	s := &AgentDeployerService{}
	req := capabilityRequestFixture()
	err := s.attachKnowledgeCapabilities(req, "tenant-a", "tenant-a-root", "token")
	require.Error(t, err)
	require.Contains(t, err.Error(), "encryption_key")

	// Without any knowledge MCP the empty key is irrelevant (nothing to sign).
	noKnowledge := &deployer.CreateAgentRequest{
		DeploymentKey: "tenant-a-root",
		Agents: []deployer.AgentDefinition{
			{Name: "root", McpServers: map[string]deployer.McpServerConfig{
				"weather": {Type: "http", URL: "https://mcp.example.com/weather"},
			}},
		},
	}
	require.NoError(t, s.attachKnowledgeCapabilities(noKnowledge, "tenant-a", "tenant-a-root", "token"))
}
