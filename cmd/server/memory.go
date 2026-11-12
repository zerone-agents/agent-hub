package main

import (
	"context"
	"log"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/config"
	"control-panel/internal/handler"
	knowledgeinfra "control-panel/internal/infrastructure/knowledge"
	"control-panel/internal/infrastructure/memoryindex"
	"control-panel/internal/infrastructure/nativememory"
	"control-panel/internal/memory"
	"control-panel/pkg/database"
)

func configureMemory(cfg config.MemoryConfig, guard *services.DatasetGuard) ([]handler.MemoryBinding, error) {
	clients, err := cfg.Clients()
	if err != nil || len(clients) == 0 {
		return nil, err
	}
	wrapper, err := memory.NewSyntheticKeyWrapper(cfg.WrappingKey)
	if err != nil {
		return nil, err
	}
	engine := knowledgeinfra.NewRemoteMultiragEngine(cfg.MultiragURL, cfg.MultiragKey, 5*time.Second, 5*time.Second)
	out := make([]handler.MemoryBinding, 0, len(clients))
	for _, client := range clients {
		var index memory.Index
		var conversations memory.NativeConversations
		containerID := client.DatasetID
		backend := client.Backend
		if backend == memory.BackendNative {
			containerID = client.MemoryID
			native, err := nativememory.New(nativememory.Config{BaseURL: cfg.MultiragURL, APIKey: cfg.MultiragKey, BankID: containerID})
			if err != nil {
				return nil, err
			}
			index = native
			if client.AutomaticExtraction {
				conversations = native
			}
		} else {
			backend = memory.BackendKnowledge
			if err := guard.Reserve(context.Background(), client.DatasetID, client.TenantID, client.ApplicationID); err != nil {
				return nil, err
			}
			index = memoryindex.New(engine)
		}
		svc, err := memory.NewService(database.GetDB(), memory.Config{TenantID: client.TenantID, ApplicationID: client.ApplicationID, PolicyVersion: client.PolicyVersion, DatasetID: containerID, Backend: backend, SyntheticOnly: true, Wrapper: wrapper, Index: index, Native: conversations})
		if err != nil {
			return nil, err
		}
		out = append(out, handler.MemoryBinding{Client: client, Service: svc})
	}
	return out, nil
}

// runMemoryWorker drains both the business memory bindings and the
// agent-memory registry's per-tenant services（review 阻塞①）：hub-agent
// services live outside memoryBindings, and an undrained agent-memory
// outbox means chunk_id is never set — recall and first-turn injection
// would be permanently empty in production. Registry may be nil (knowledge
// connection unconfigured); lazy per-tenant services appear on the next
// tick after their first tool call.
func runMemoryWorker(ctx context.Context, bindings []handler.MemoryBinding, agentMemory *services.AgentMemoryRegistry) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for _, b := range bindings {
			attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := b.Service.Sync(attempt, 20)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Println("Memory: index reconciliation remains pending; inspect operation status")
			}
		}
		if agentMemory != nil {
			for _, svc := range agentMemory.Services() {
				attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
				err := svc.Sync(attempt, 20)
				cancel()
				if err != nil && ctx.Err() == nil {
					log.Println("Memory: agent-memory index reconciliation remains pending; inspect operation status")
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
