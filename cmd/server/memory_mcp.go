package main

import (
	"control-panel/internal/application/services"
	"control-panel/internal/handler"
)

// agentMemoryResolverAdapter adapts the per-tenant registry to the MCP
// handler's resolver interface — *memory.Service structurally implements
// handler.AgentMemoryStore.
type agentMemoryResolverAdapter struct{ registry *services.AgentMemoryRegistry }

func (a agentMemoryResolverAdapter) AgentMemoryStore(tenantID string) (handler.AgentMemoryStore, error) {
	return a.registry.Service(tenantID)
}
