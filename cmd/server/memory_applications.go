package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"

	"control-panel/internal/config"
	"control-panel/internal/handler"
	"control-panel/internal/infrastructure/nativememory"
	"control-panel/internal/memory"
	"control-panel/internal/memoryapp"
	"control-panel/pkg/database"
	"gorm.io/gorm"
)

type memoryApplicationSource struct {
	db       *gorm.DB
	registry *memoryapp.Registry
	static   []handler.MemoryBinding
	cfg      config.MemoryConfig
	wrapper  memory.KeyWrapper
}

func configureMemoryApplications(cfg config.MemoryConfig, static []handler.MemoryBinding) (*memoryApplicationSource, []config.MemoryManager, error) {
	if !cfg.RuntimeEnabled() {
		return nil, nil, nil
	}
	managers, err := cfg.Managers()
	if err != nil {
		return nil, nil, err
	}
	wrapper, err := memory.NewSyntheticKeyWrapper(cfg.WrappingKey)
	if err != nil {
		return nil, nil, err
	}
	banks, err := nativememory.NewBankProvisioner(nativememory.BankProvisionerConfig{BaseURL: cfg.MultiragURL, APIKey: cfg.MultiragKey})
	if err != nil {
		return nil, nil, err
	}
	registry, err := memoryapp.NewRegistry(database.GetDB(), wrapper, banks)
	if err != nil {
		return nil, nil, err
	}
	reservations := make([]memoryapp.StaticReservation, 0, len(static))
	for _, b := range static {
		c := b.Client
		backend, container := c.Backend, c.DatasetID
		if backend == "" {
			backend = memory.BackendKnowledge
		}
		if backend == memory.BackendNative {
			container = c.MemoryID
		}
		hub := c.HubTenantID
		if hub == "" {
			hub = c.TenantID
		}
		reservations = append(reservations, memoryapp.StaticReservation{TenantID: c.TenantID, ApplicationID: c.ApplicationID, HubTenantID: hub, Backend: backend, ContainerID: container, TokenDigest: memoryapp.TokenDigest(c.Token)})
	}
	if err = registry.ReserveStatic(context.Background(), reservations); err != nil {
		return nil, nil, err
	}
	for _, m := range managers {
		if err = registry.ReserveManager(context.Background(), m.Token, m.TenantID); err != nil {
			return nil, nil, err
		}
	}
	return &memoryApplicationSource{db: database.GetDB(), registry: registry, static: static, cfg: cfg, wrapper: wrapper}, managers, nil
}
func (s *memoryApplicationSource) build(b memoryapp.Binding) (handler.MemoryBinding, error) {
	native, err := nativememory.New(nativememory.Config{BaseURL: s.cfg.MultiragURL, APIKey: s.cfg.MultiragKey, BankID: b.MemoryID})
	if err != nil {
		return handler.MemoryBinding{}, memoryapp.ErrUnavailable
	}
	svc, err := memory.NewService(s.db, memory.Config{TenantID: b.TenantID, ApplicationID: b.ApplicationID, PolicyVersion: b.PolicyVersion, DatasetID: b.MemoryID, Backend: memory.BackendNative, SyntheticOnly: true, Wrapper: s.wrapper, Index: native, Native: native})
	if err != nil {
		return handler.MemoryBinding{}, memoryapp.ErrUnavailable
	}
	return handler.MemoryBinding{Client: config.MemoryClient{TenantID: b.TenantID, ApplicationID: b.ApplicationID, HubTenantID: b.HubTenantID, PolicyVersion: b.PolicyVersion, MemoryID: b.MemoryID, Backend: memory.BackendNative, AutomaticExtraction: true}, Service: svc, Name: b.Name, Status: b.Status, Origin: "managed"}, nil
}
func (s *memoryApplicationSource) Authenticate(ctx context.Context, token string) (handler.MemoryBinding, error) {
	actual := sha256.Sum256([]byte(token))
	selected := -1
	for i, b := range s.static {
		expected := sha256.Sum256([]byte(b.Client.Token))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) == 1 {
			selected = i
		}
	}
	if selected >= 0 {
		return s.static[selected], nil
	}
	b, err := s.registry.Authenticate(ctx, token)
	if err != nil {
		return handler.MemoryBinding{}, err
	}
	return s.build(b)
}
func (s *memoryApplicationSource) BindingsForHubTenant(ctx context.Context, tenant string) ([]handler.MemoryBinding, error) {
	out := []handler.MemoryBinding{}
	for _, b := range s.static {
		hub := b.Client.HubTenantID
		if hub == "" {
			hub = b.Client.TenantID
		}
		if hub == tenant {
			out = append(out, b)
		}
	}
	bindings, err := s.registry.ForHubTenant(ctx, tenant)
	if err != nil {
		return nil, err
	}
	for _, b := range bindings {
		built, err := s.build(b)
		if err != nil {
			return nil, err
		}
		out = append(out, built)
	}
	return out, nil
}

var _ handler.MemoryBindingSource = (*memoryApplicationSource)(nil)
