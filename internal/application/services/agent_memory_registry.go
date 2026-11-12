package services

// agent_memory_registry.go resolves the per-tenant ledger service behind the
// memory MCP endpoint (spec docs/superpowers/specs/2026-11-11-memory-mcp
// -design.md §3/§6). One shared memory.Index adapter serves every tenant —
// index calls are parameterized by datasetID — while each tenant gets its
// own memory.Service pinned to a per-tenant reserved dataset: MultiRAG
// dataset names are globally unique, so a shared name would mix retrieval
// replicas across tenants.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"control-panel/internal/memory"

	"gorm.io/gorm"
)

const (
	// 保留标识（spec §3/§6），勿改值。
	AgentMemoryApplication   = "hub-agent"
	AgentMemoryDatasetPrefix = "hub-agent-mem"
	AgentMemoryPolicy        = "agent-mcp-v1"
)

// AgentMemoryDatasetID derives the per-tenant reserved dataset name.
// MultiRAG identifiers cap at 32 chars of [A-Za-z0-9_-] (memoryindex adapter
// identifier regexp) and org names may be up to 63 — so the tenant component
// is a 16-hex digest, keeping the total at 29 (review 阻塞③): raw tenant
// names would silently fail validation and degrade recall to always-empty.
func AgentMemoryDatasetID(tenantID string) string {
	sum := sha256.Sum256([]byte(tenantID))
	return AgentMemoryDatasetPrefix + "-" + hex.EncodeToString(sum[:])[:16]
}

// AgentMemoryRegistry lazily builds and caches per-tenant memory services.
type AgentMemoryRegistry struct {
	db      *gorm.DB
	wrapper memory.KeyWrapper
	guard   *DatasetGuard
	index   memory.Index
	mu      sync.Mutex
	cache   map[string]*memory.Service
}

func NewAgentMemoryRegistry(db *gorm.DB, wrapper memory.KeyWrapper, guard *DatasetGuard, index memory.Index) *AgentMemoryRegistry {
	return &AgentMemoryRegistry{db: db, wrapper: wrapper, guard: guard, index: index, cache: map[string]*memory.Service{}}
}

// Service returns the tenant's ledger service, provisioning the reserved
// dataset on first resolution (idempotent via the guard's OnConflict
// DoNothing reservation). Safe for concurrent use.
func (r *AgentMemoryRegistry) Service(tenantID string) (*memory.Service, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("agent memory: empty tenant")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if svc, ok := r.cache[tenantID]; ok {
		return svc, nil
	}
	dataset := AgentMemoryDatasetID(tenantID)
	if err := r.guard.Reserve(context.Background(), dataset, tenantID, AgentMemoryApplication); err != nil {
		return nil, fmt.Errorf("reserve agent-memory dataset %s: %w", dataset, err)
	}
	svc, err := memory.NewService(r.db, memory.Config{
		TenantID:      tenantID,
		ApplicationID: AgentMemoryApplication,
		PolicyVersion: AgentMemoryPolicy,
		DatasetID:     dataset,
		Backend:       memory.BackendKnowledge,
		SyntheticOnly: true,
		Wrapper:       r.wrapper,
		Index:         r.index,
	})
	if err != nil {
		return nil, err
	}
	r.cache[tenantID] = svc
	return svc, nil
}

// AgentMemoryRecaller returns the tenant's narrowed recall surface — the
// injector's dependency seam (memory_injection.go). *memory.Service
// satisfies AgentMemoryRecaller structurally.
func (r *AgentMemoryRegistry) AgentMemoryRecaller(tenantID string) (AgentMemoryRecaller, error) {
	return r.Service(tenantID)
}

// Services returns a snapshot of every per-tenant service created so far.
// The memory outbox worker consumes this: hub-agent services live outside
// the business memoryBindings, and without draining them the agent-memory
// outbox never empties (chunk_id never set → recall always empty — review
// 阻塞①). Lazy creation still works: a service created by the first tool
// call appears on the next worker tick.
func (r *AgentMemoryRegistry) Services() []*memory.Service {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*memory.Service, 0, len(r.cache))
	for _, svc := range r.cache {
		out = append(out, svc)
	}
	return out
}
