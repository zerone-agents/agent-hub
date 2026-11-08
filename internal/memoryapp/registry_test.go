package memoryapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/memory"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var registryContext = context.Background()

// This fake models a non-idempotent provider. Only the registry is permitted
// to decide whether a request may dispatch; a second Create really is a second
// external operation, even when its specification is identical.
type testBankProvisioner struct {
	mu          sync.Mutex
	banks       map[string]Bank
	createCalls int
	findCalls   int
	createSpecs []BankSpec
	findSpecs   []BankSpec
	createErr   error
	findErr     error
	doNotStore  bool
	fixedID     string
	mismatch    func(Bank) Bank
	gate        <-chan struct{}
	started     chan struct{}
	findStarted chan struct{}
	startOnce   sync.Once
	findOnce    sync.Once
}

func newTestBanks() *testBankProvisioner { return &testBankProvisioner{banks: map[string]Bank{}} }
func (f *testBankProvisioner) Create(ctx context.Context, in BankSpec) (Bank, error) {
	f.mu.Lock()
	f.createCalls++
	f.createSpecs = append(f.createSpecs, in)
	id := f.fixedID
	if id == "" {
		id = fmt.Sprintf("bank-%d", f.createCalls)
	}
	bank := Bank{ID: id, Name: in.Name, ChatModel: in.ChatModel, EmbeddingModel: in.EmbeddingModel}
	if f.mismatch != nil {
		bank = f.mismatch(bank)
	}
	err, store, gate := f.createErr, !f.doNotStore, f.gate
	f.mu.Unlock()
	if f.started != nil {
		f.startOnce.Do(func() { close(f.started) })
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return Bank{}, ctx.Err()
		}
	}
	if store {
		f.mu.Lock()
		f.banks[in.Name] = bank
		f.mu.Unlock()
	}
	return bank, err
}
func (f *testBankProvisioner) Find(_ context.Context, in BankSpec) (Bank, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findCalls++
	f.findSpecs = append(f.findSpecs, in)
	if f.findStarted != nil {
		f.findOnce.Do(func() { close(f.findStarted) })
	}
	if f.findErr != nil {
		return Bank{}, f.findErr
	}
	bank, ok := f.banks[in.Name]
	if !ok {
		return Bank{}, ErrNotFound
	}
	return bank, nil
}
func (f *testBankProvisioner) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCalls, f.findCalls
}
func (f *testBankProvisioner) setFindError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findErr = err
}
func (f *testBankProvisioner) specifications() ([]BankSpec, []BankSpec) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]BankSpec(nil), f.createSpecs...), append([]BankSpec(nil), f.findSpecs...)
}

type registryFixture struct {
	registry *Registry
	db       *gorm.DB
	wrapper  *memory.SyntheticKeyWrapper
	banks    *testBankProvisioner
	owner    Owner
}

func newRegistryFixture(t *testing.T) *registryFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/registry.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	require.NoError(t, Migrate(db))
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("ab", 32))
	require.NoError(t, err)
	banks := newTestBanks()
	registry, err := NewRegistry(db, wrapper, banks)
	require.NoError(t, err)
	return &registryFixture{registry: registry, db: db, wrapper: wrapper, banks: banks, owner: Owner{TenantID: "tenant-A", HubTenantID: "hub-A", ChatModel: "356", EmbeddingModel: "337", MaxApplications: 10}}
}
func (f *registryFixture) restart(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(f.db, f.wrapper, f.banks)
	require.NoError(t, err)
	return r
}
func createRequest(id string) CreateInput {
	return CreateInput{OperationID: "create-" + id, ApplicationID: id, Name: "合成记忆应用 " + id, Description: "隔离测试", PolicyVersion: "synthetic-v1"}
}
func boolValue(value bool) *bool       { return &value }
func stringValue(value string) *string { return &value }
func requireNoPlainSecret(t *testing.T, secret string, values ...any) {
	t.Helper()
	for _, value := range values {
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		require.NotContains(t, string(raw), secret)
		require.NotContains(t, string(raw), "applicationToken")
		require.NotContains(t, string(raw), "tokenCipher")
		require.NotContains(t, string(raw), "tokenDigest")
	}
}
func registryRows(t *testing.T, f *registryFixture) []map[string]any {
	t.Helper()
	var rows []map[string]any
	require.NoError(t, f.db.Table("memory_applications").Order("id").Find(&rows).Error)
	return rows
}

func TestRegistryCreateReadUpdateAndExactReplay(t *testing.T) {
	f := newRegistryFixture(t)
	in := createRequest("tea")
	created, err := f.registry.Create(registryContext, f.owner, in)
	require.NoError(t, err)
	require.Equal(t, "active", created.Application.Status)
	require.NotEmpty(t, created.Application.MemoryID)
	require.Len(t, created.ApplicationToken, 64)
	replay, err := f.restart(t).Create(registryContext, f.owner, in)
	require.NoError(t, err)
	require.Equal(t, created, replay)
	creates, finds := f.banks.counts()
	require.Equal(t, 1, creates)
	require.Zero(t, finds)
	detail, err := f.registry.Get(registryContext, f.owner.TenantID, in.ApplicationID, false)
	require.NoError(t, err)
	require.Equal(t, created.Application, detail)
	listed, err := f.registry.List(registryContext, f.owner.TenantID, 0, 10)
	require.NoError(t, err)
	require.Equal(t, []Application{detail}, listed)
	updated, err := f.registry.Update(registryContext, f.owner.TenantID, in.ApplicationID, UpdateInput{ExpectedVersion: detail.Version, Name: stringValue("新的显示名称"), Description: stringValue("")})
	require.NoError(t, err)
	require.Equal(t, "新的显示名称", updated.Name)
	require.Empty(t, updated.Description)
	require.Greater(t, updated.Version, detail.Version)
	require.Equal(t, detail.MemoryID, updated.MemoryID)
	require.Equal(t, detail.MemoryName, updated.MemoryName)
	_, err = f.registry.Update(registryContext, f.owner.TenantID, in.ApplicationID, UpdateInput{ExpectedVersion: detail.Version, Name: stringValue("过期修改")})
	require.ErrorIs(t, err, ErrConflict)
	after, err := f.registry.Get(registryContext, f.owner.TenantID, in.ApplicationID, false)
	require.NoError(t, err)
	require.Equal(t, updated, after)
	requireNoPlainSecret(t, created.ApplicationToken, detail, listed, updated)
}

func TestRegistryRejectsOperationAndPayloadReuseBeforeExternalEffects(t *testing.T) {
	f := newRegistryFixture(t)
	in := createRequest("tea")
	original, err := f.registry.Create(registryContext, f.owner, in)
	require.NoError(t, err)
	cases := []struct {
		name  string
		owner Owner
		input CreateInput
	}{
		{"same app new operation", f.owner, func() CreateInput { x := in; x.OperationID = "another-operation"; return x }()},
		{"same operation changed body", f.owner, func() CreateInput { x := in; x.Name = "Changed payload"; return x }()},
		{"operation reused for other app", f.owner, func() CreateInput { x := in; x.ApplicationID = "other-app"; return x }()},
		{"same app new delegated owner", func() Owner { x := f.owner; x.HubTenantID = "another-hub"; return x }(), in},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.registry.Create(registryContext, test.owner, test.input)
			require.ErrorIs(t, err, ErrConflict)
		})
	}
	creates, finds := f.banks.counts()
	require.Equal(t, 1, creates)
	require.Zero(t, finds)
	list, err := f.registry.List(registryContext, f.owner.TenantID, 0, 10)
	require.NoError(t, err)
	require.Equal(t, []Application{original.Application}, list)
}

func TestRegistryTenantsAndDelegatedHubTenantsRemainCaseSensitiveAndIsolated(t *testing.T) {
	f := newRegistryFixture(t)
	in := createRequest("same-app")
	first, err := f.registry.Create(registryContext, f.owner, in)
	require.NoError(t, err)
	other := f.owner
	other.TenantID = "tenant-a"
	other.HubTenantID = "hub-a"
	second, err := f.registry.Create(registryContext, other, in)
	require.NoError(t, err)
	require.NotEqual(t, first.Application.MemoryID, second.Application.MemoryID)
	require.NotEqual(t, first.ApplicationToken, second.ApplicationToken)
	for _, test := range []struct {
		owner  Owner
		result CreateResult
	}{{f.owner, first}, {other, second}} {
		rows, err := f.registry.List(registryContext, test.owner.TenantID, 0, 10)
		require.NoError(t, err)
		require.Equal(t, []Application{test.result.Application}, rows)
		admin, err := f.registry.ForHubTenant(registryContext, test.owner.HubTenantID)
		require.NoError(t, err)
		require.Len(t, admin, 1)
		require.Equal(t, test.result.Application.MemoryID, admin[0].MemoryID)
		authorized, err := f.registry.Authenticate(registryContext, test.result.ApplicationToken)
		require.NoError(t, err)
		require.Equal(t, test.owner.TenantID, authorized.TenantID)
		require.Equal(t, test.owner.HubTenantID, authorized.HubTenantID)
	}
	_, err = f.registry.Get(registryContext, "foreign-tenant", in.ApplicationID, true)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = f.registry.Update(registryContext, "foreign-tenant", in.ApplicationID, UpdateInput{ExpectedVersion: first.Application.Version, Enabled: boolValue(false)})
	require.ErrorIs(t, err, ErrNotFound)
	rows, err := f.registry.ForHubTenant(registryContext, "HUB-A")
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = f.registry.Authenticate(registryContext, strings.Repeat("f", 64))
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRegistryBindingsPreserveSameUsersApplicationBoundaryInMemoryConsumer(t *testing.T) {
	f := newRegistryFixture(t)
	first, err := f.registry.Create(registryContext, f.owner, createRequest("app-a"))
	require.NoError(t, err)
	second, err := f.registry.Create(registryContext, f.owner, createRequest("app-b"))
	require.NoError(t, err)
	firstBinding, err := f.registry.Authenticate(registryContext, first.ApplicationToken)
	require.NoError(t, err)
	secondBinding, err := f.registry.Authenticate(registryContext, second.ApplicationToken)
	require.NoError(t, err)
	require.NoError(t, memory.Migrate(f.db))
	consumer := func(binding Binding) *memory.Service {
		svc, err := memory.NewService(f.db, memory.Config{TenantID: binding.TenantID, ApplicationID: binding.ApplicationID, PolicyVersion: binding.PolicyVersion, DatasetID: binding.MemoryID, Backend: memory.BackendNative, SyntheticOnly: true, Wrapper: f.wrapper})
		require.NoError(t, err)
		return svc
	}
	firstService, secondService := consumer(firstBinding), consumer(secondBinding)
	firstScope := memory.Scope{TenantID: firstBinding.TenantID, ApplicationID: firstBinding.ApplicationID, UserID: "same-user", LifecycleID: "same-account"}
	secondScope := firstScope
	secondScope.ApplicationID = secondBinding.ApplicationID
	_, err = firstService.Activate(registryContext, firstScope, "activate-a")
	require.NoError(t, err)
	_, err = secondService.List(registryContext, secondScope)
	require.ErrorIs(t, err, memory.ErrNotFound)
	_, err = firstService.List(registryContext, secondScope)
	require.ErrorIs(t, err, memory.ErrForbidden)
	_, err = secondService.Activate(registryContext, secondScope, "activate-b")
	require.NoError(t, err)
	_, err = secondService.List(registryContext, firstScope)
	require.ErrorIs(t, err, memory.ErrForbidden)
	facts, err := firstService.List(registryContext, firstScope)
	require.NoError(t, err)
	require.Empty(t, facts)
	facts, err = secondService.List(registryContext, secondScope)
	require.NoError(t, err)
	require.Empty(t, facts)
}

func TestRegistryUnknownWriteRecoversAfterRestartUsingOnlyExactFind(t *testing.T) {
	f := newRegistryFixture(t)
	f.banks.createErr = errors.New("provider response lost after write")
	in := createRequest("lost-response")
	initial, err := f.registry.Create(registryContext, f.owner, in)
	require.ErrorIs(t, err, ErrUnknown)
	require.Equal(t, "unknown", initial.Application.Status)
	require.Empty(t, initial.Application.MemoryID)
	require.Len(t, initial.ApplicationToken, 64)
	_, err = f.registry.Authenticate(registryContext, initial.ApplicationToken)
	require.ErrorIs(t, err, ErrDisabled)
	// Trusted defaults may have changed after restart; recovery must use the
	// immutable persisted specification rather than search a different model.
	ownerAfterRestart := f.owner
	ownerAfterRestart.ChatModel = "other-chat-model"
	recovered, err := f.restart(t).Create(registryContext, ownerAfterRestart, in)
	require.NoError(t, err)
	require.Equal(t, "active", recovered.Application.Status)
	require.Equal(t, initial.ApplicationToken, recovered.ApplicationToken)
	creates, finds := f.banks.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 1, finds)
	written, searched := f.banks.specifications()
	require.Equal(t, written, searched)
	require.Equal(t, f.owner.ChatModel, recovered.Application.ChatModel)
	binding, err := f.registry.Authenticate(registryContext, recovered.ApplicationToken)
	require.NoError(t, err)
	require.Equal(t, recovered.Application.MemoryID, binding.MemoryID)
}

func TestRegistryUnknownWithoutMatchNeverRepostsOrAllowsNewOperation(t *testing.T) {
	f := newRegistryFixture(t)
	f.banks.createErr = errors.New("uncertain dispatch")
	f.banks.doNotStore = true
	in := createRequest("no-match")
	initial, err := f.registry.Create(registryContext, f.owner, in)
	require.ErrorIs(t, err, ErrUnknown)
	for i := 0; i < 3; i++ {
		reg := f.restart(t)
		replay, err := reg.Create(registryContext, f.owner, in)
		require.ErrorIs(t, err, ErrUnknown)
		require.Equal(t, initial.Application.MemoryName, replay.Application.MemoryName)
		detail, err := reg.Get(registryContext, f.owner.TenantID, in.ApplicationID, true)
		require.NoError(t, err)
		require.Empty(t, detail.MemoryID)
		require.Equal(t, "unknown", detail.Status)
	}
	changed := in
	changed.OperationID = "fresh-key-must-not-authorize-repost"
	_, err = f.restart(t).Create(registryContext, f.owner, changed)
	require.ErrorIs(t, err, ErrConflict)
	creates, finds := f.banks.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 6, finds)
	_, err = f.registry.Update(registryContext, f.owner.TenantID, in.ApplicationID, UpdateInput{ExpectedVersion: initial.Application.Version, Enabled: boolValue(true)})
	require.ErrorIs(t, err, ErrConflict)
}

func TestRegistryRejectsAmbiguousRecoveryAndMismatchedProviderBank(t *testing.T) {
	for _, test := range []struct {
		name     string
		mismatch func(Bank) Bank
	}{
		{"wrong name", func(b Bank) Bank { b.Name = "other-bank"; return b }},
		{"wrong chat model", func(b Bank) Bank { b.ChatModel = "other-model"; return b }},
		{"wrong embedding", func(b Bank) Bank { b.EmbeddingModel = "other-embedding"; return b }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newRegistryFixture(t)
			f.banks.mismatch = test.mismatch
			in := createRequest("mismatch")
			created, err := f.registry.Create(registryContext, f.owner, in)
			require.ErrorIs(t, err, ErrUnknown)
			require.Empty(t, created.Application.MemoryID)
			_, err = f.registry.Authenticate(registryContext, created.ApplicationToken)
			require.ErrorIs(t, err, ErrDisabled)
			f.banks.setFindError(ErrConflict)
			_, err = f.restart(t).Create(registryContext, f.owner, in)
			require.ErrorIs(t, err, ErrUnknown)
			creates, finds := f.banks.counts()
			require.Equal(t, 1, creates)
			require.Equal(t, 1, finds)
		})
	}
}

func TestRegistryProviderContainerCollisionFailsClosedAcrossApplications(t *testing.T) {
	f := newRegistryFixture(t)
	f.banks.fixedID = "provider-reused-bank"
	first, err := f.registry.Create(registryContext, f.owner, createRequest("first"))
	require.NoError(t, err)
	secondInput := createRequest("second")
	_, err = f.registry.Create(registryContext, f.owner, secondInput)
	require.ErrorIs(t, err, ErrConflict)
	second, err := f.registry.Get(registryContext, f.owner.TenantID, secondInput.ApplicationID, false)
	require.NoError(t, err)
	require.Empty(t, second.MemoryID)
	require.NotEqual(t, "active", second.Status)
	_, err = f.restart(t).Create(registryContext, f.owner, secondInput)
	require.ErrorIs(t, err, ErrConflict)
	creates, _ := f.banks.counts()
	require.Equal(t, 2, creates)
	bound, err := f.registry.Authenticate(registryContext, first.ApplicationToken)
	require.NoError(t, err)
	require.Equal(t, first.Application.MemoryID, bound.MemoryID)
	admin, err := f.registry.ForHubTenant(registryContext, f.owner.HubTenantID)
	require.NoError(t, err)
	require.Len(t, admin, 1)
	require.Equal(t, first.Application.ApplicationID, admin[0].ApplicationID)
}

func TestRegistryStaticReservationsProtectAppTokenAndContainerNamespaces(t *testing.T) {
	f := newRegistryFixture(t)
	staticToken := strings.Repeat("a", 64)
	static := StaticReservation{TenantID: f.owner.TenantID, ApplicationID: "static", HubTenantID: f.owner.HubTenantID, Backend: memory.BackendNative, ContainerID: "reserved-bank", TokenDigest: TokenDigest(staticToken)}
	require.NoError(t, f.registry.ReserveStatic(registryContext, []StaticReservation{static}))
	require.NoError(t, f.registry.ReserveStatic(registryContext, []StaticReservation{static}))
	_, err := f.registry.Create(registryContext, f.owner, createRequest("static"))
	require.ErrorIs(t, err, ErrConflict)
	creates, _ := f.banks.counts()
	require.Zero(t, creates)
	_, err = f.registry.Authenticate(registryContext, staticToken)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = f.registry.Get(registryContext, f.owner.TenantID, static.ApplicationID, false)
	require.ErrorIs(t, err, ErrNotFound)
	list, err := f.registry.List(registryContext, f.owner.TenantID, 0, 100)
	require.NoError(t, err)
	require.Empty(t, list)
	managed, err := f.registry.Create(registryContext, f.owner, createRequest("managed"))
	require.NoError(t, err)
	for _, collision := range []StaticReservation{
		{TenantID: "other-tenant", ApplicationID: "token-collision", Backend: memory.BackendNative, ContainerID: "another-bank", TokenDigest: TokenDigest(managed.ApplicationToken)},
		{TenantID: "other-tenant", ApplicationID: "container-collision", Backend: memory.BackendNative, ContainerID: managed.Application.MemoryID, TokenDigest: TokenDigest(strings.Repeat("b", 64))},
		{TenantID: f.owner.TenantID, ApplicationID: managed.Application.ApplicationID, Backend: memory.BackendNative, ContainerID: "unrelated-bank", TokenDigest: TokenDigest(strings.Repeat("c", 64))},
	} {
		before := registryRows(t, f)
		require.ErrorIs(t, f.registry.ReserveStatic(registryContext, []StaticReservation{collision}), ErrConflict)
		require.Equal(t, before, registryRows(t, f))
	}
	f.banks.mu.Lock()
	f.banks.fixedID = static.ContainerID
	f.banks.mu.Unlock()
	_, err = f.registry.Create(registryContext, f.owner, createRequest("reverse-container-collision"))
	require.ErrorIs(t, err, ErrConflict)
	detail, err := f.registry.Get(registryContext, f.owner.TenantID, "reverse-container-collision", false)
	require.NoError(t, err)
	require.Empty(t, detail.MemoryID)
}

func TestRegistryQuotaIncludesUncertainAndDisabledApplicationsWithoutSideEffects(t *testing.T) {
	f := newRegistryFixture(t)
	f.owner.MaxApplications = 1
	f.banks.createErr = ErrUnknown
	first, err := f.registry.Create(registryContext, f.owner, createRequest("quota-one"))
	require.ErrorIs(t, err, ErrUnknown)
	_, err = f.registry.Create(registryContext, f.owner, createRequest("quota-two"))
	require.ErrorIs(t, err, ErrQuota)
	recovered, err := f.restart(t).Create(registryContext, f.owner, createRequest("quota-one"))
	require.NoError(t, err)
	disabled, err := f.registry.Update(registryContext, f.owner.TenantID, "quota-one", UpdateInput{ExpectedVersion: recovered.Application.Version, Enabled: boolValue(false)})
	require.NoError(t, err)
	require.Equal(t, "disabled", disabled.Status)
	_, err = f.registry.Create(registryContext, f.owner, createRequest("quota-two"))
	require.ErrorIs(t, err, ErrQuota)
	creates, _ := f.banks.counts()
	require.Equal(t, 1, creates)
	list, err := f.registry.List(registryContext, f.owner.TenantID, 0, 100)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, first.ApplicationToken, recovered.ApplicationToken)
}

func TestRegistryDisableImmediatelyRevokesAuthenticationButPreservesAdminAndReenable(t *testing.T) {
	f := newRegistryFixture(t)
	created, err := f.registry.Create(registryContext, f.owner, createRequest("toggle"))
	require.NoError(t, err)
	_, err = f.registry.Authenticate(registryContext, created.ApplicationToken)
	require.NoError(t, err)
	disabled, err := f.registry.Update(registryContext, f.owner.TenantID, "toggle", UpdateInput{ExpectedVersion: created.Application.Version, Enabled: boolValue(false)})
	require.NoError(t, err)
	restarted := f.restart(t)
	_, err = restarted.Authenticate(registryContext, created.ApplicationToken)
	require.ErrorIs(t, err, ErrDisabled)
	detail, err := restarted.Get(registryContext, f.owner.TenantID, "toggle", true)
	require.NoError(t, err)
	require.Equal(t, disabled, detail)
	list, err := restarted.List(registryContext, f.owner.TenantID, 0, 10)
	require.NoError(t, err)
	require.Equal(t, []Application{disabled}, list)
	admin, err := restarted.ForHubTenant(registryContext, f.owner.HubTenantID)
	require.NoError(t, err)
	require.Len(t, admin, 1)
	require.Equal(t, "disabled", admin[0].Status)
	_, err = restarted.Update(registryContext, f.owner.TenantID, "toggle", UpdateInput{ExpectedVersion: created.Application.Version, Enabled: boolValue(true)})
	require.ErrorIs(t, err, ErrConflict)
	_, err = restarted.Authenticate(registryContext, created.ApplicationToken)
	require.ErrorIs(t, err, ErrDisabled)
	enabled, err := restarted.Update(registryContext, f.owner.TenantID, "toggle", UpdateInput{ExpectedVersion: disabled.Version, Enabled: boolValue(true)})
	require.NoError(t, err)
	require.Equal(t, "active", enabled.Status)
	auth, err := restarted.Authenticate(registryContext, created.ApplicationToken)
	require.NoError(t, err)
	require.Equal(t, created.Application.MemoryID, auth.MemoryID)
	creates, finds := f.banks.counts()
	require.Equal(t, 1, creates)
	require.Zero(t, finds)
}

func TestRegistrySecretsAreEncryptedAndNeverAppearInReadDTOs(t *testing.T) {
	f := newRegistryFixture(t)
	created, err := f.registry.Create(registryContext, f.owner, createRequest("secret"))
	require.NoError(t, err)
	var stored applicationRow
	require.NoError(t, f.db.First(&stored).Error)
	require.NotNil(t, stored.TokenDigest)
	require.NotEqual(t, created.ApplicationToken, *stored.TokenDigest)
	require.False(t, bytes.Contains(stored.TokenCipher, []byte(created.ApplicationToken)))
	raw, err := json.Marshal(registryRows(t, f))
	require.NoError(t, err)
	require.NotContains(t, string(raw), created.ApplicationToken)
	// Ordinary detail, list, admin binding and authentication never include the token.
	detail, err := f.registry.Get(registryContext, f.owner.TenantID, "secret", true)
	require.NoError(t, err)
	list, err := f.registry.List(registryContext, f.owner.TenantID, 0, 100)
	require.NoError(t, err)
	admin, err := f.registry.ForHubTenant(registryContext, f.owner.HubTenantID)
	require.NoError(t, err)
	auth, err := f.registry.Authenticate(registryContext, created.ApplicationToken)
	require.NoError(t, err)
	requireNoPlainSecret(t, created.ApplicationToken, detail, list, admin, auth)
	// A token cannot be silently promoted to a separate management credential.
	require.ErrorIs(t, f.registry.CheckManagerToken(registryContext, created.ApplicationToken), ErrConflict)
	require.NoError(t, f.registry.CheckManagerToken(registryContext, strings.Repeat("d", 64)))
	// Reopening with the same wrapper can recover the original operation secret;
	// ordinary read DTOs still cannot. A different key must fail closed.
	restarted := f.restart(t)
	replay, err := restarted.Create(registryContext, f.owner, createRequest("secret"))
	require.NoError(t, err)
	require.Equal(t, created.ApplicationToken, replay.ApplicationToken)
	wrongKey, err := memory.NewSyntheticKeyWrapper(strings.Repeat("ef", 32))
	require.NoError(t, err)
	wrong, err := NewRegistry(f.db, wrongKey, f.banks)
	require.NoError(t, err)
	leaked, err := wrong.Create(registryContext, f.owner, createRequest("secret"))
	require.ErrorIs(t, err, ErrUnavailable)
	require.Empty(t, leaked.ApplicationToken)
}

func TestRegistryConcurrentExactCreateDispatchesOnlyOneProviderPost(t *testing.T) {
	f := newRegistryFixture(t)
	gate := make(chan struct{})
	f.banks.gate = gate
	f.banks.started = make(chan struct{})
	f.banks.findStarted = make(chan struct{})
	in := createRequest("concurrent")
	type result struct {
		created CreateResult
		err     error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := f.registry.Create(registryContext, f.owner, in)
			results <- result{r, err}
		}()
	}
	close(start)
	select {
	case <-f.banks.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the one authorized dispatch did not start")
	}
	select {
	case <-f.banks.findStarted:
	case <-time.After(10 * time.Second):
		close(gate)
		t.Fatal("duplicate requests did not reach read-only recovery")
	}
	close(gate)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent create did not terminate")
	}
	close(results)
	tokens := map[string]bool{}
	for result := range results {
		require.True(t, result.err == nil || errors.Is(result.err, ErrUnknown), "unexpected error: %v", result.err)
		require.NotEmpty(t, result.created.ApplicationToken)
		tokens[result.created.ApplicationToken] = true
	}
	require.Len(t, tokens, 1)
	replay, err := f.restart(t).Create(registryContext, f.owner, in)
	require.NoError(t, err)
	require.Equal(t, "active", replay.Application.Status)
	creates, finds := f.banks.counts()
	require.Equal(t, 1, creates)
	require.GreaterOrEqual(t, finds, 1)
	list, err := f.registry.List(registryContext, f.owner.TenantID, 0, 100)
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestRegistryInvalidRequestsDoNotReserveOrDispatch(t *testing.T) {
	f := newRegistryFixture(t)
	valid := createRequest("valid")
	invalid := []CreateInput{valid, valid, valid, valid, valid}
	invalid[0].OperationID = "contains space"
	invalid[1].ApplicationID = "../other"
	invalid[2].Name = "\n"
	invalid[3].Description = strings.Repeat("界", 1025)
	invalid[4].PolicyVersion = ""
	for _, in := range invalid {
		_, err := f.registry.Create(registryContext, f.owner, in)
		require.ErrorIs(t, err, ErrInvalid)
	}
	for _, quota := range []int{0, 1001} {
		owner := f.owner
		owner.MaxApplications = quota
		_, err := f.registry.Create(registryContext, owner, valid)
		require.ErrorIs(t, err, ErrInvalid)
	}
	creates, finds := f.banks.counts()
	require.Zero(t, creates)
	require.Zero(t, finds)
	require.Empty(t, registryRows(t, f))
	for _, page := range []struct{ offset, limit int }{{-1, 5}, {0, 0}, {0, 101}, {100001, 5}} {
		_, err := f.registry.List(registryContext, f.owner.TenantID, page.offset, page.limit)
		require.ErrorIs(t, err, ErrInvalid)
	}
}

func registryStorageSnapshot(t *testing.T, f *registryFixture) map[string][]map[string]any {
	t.Helper()
	snapshot := map[string][]map[string]any{}
	for _, table := range []string{"memory_applications", "memory_application_owners", "memory_application_credentials", "memory_application_containers"} {
		var rows []map[string]any
		require.NoError(t, f.db.Table(table).Order("id").Find(&rows).Error)
		snapshot[table] = rows
	}
	return snapshot
}

func TestRegistryRotatedStaticCredentialsAndContainersNeverBecomeReusable(t *testing.T) {
	f := newRegistryFixture(t)
	firstToken, secondToken, managerToken := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)
	first := StaticReservation{TenantID: f.owner.TenantID, ApplicationID: "rotating-static", HubTenantID: f.owner.HubTenantID, Backend: memory.BackendNative, ContainerID: "bank-before-rotation", TokenDigest: TokenDigest(firstToken)}
	require.NoError(t, f.registry.ReserveStatic(registryContext, []StaticReservation{first}))
	second := first
	second.TokenDigest = TokenDigest(secondToken)
	second.ContainerID = "bank-after-rotation"
	require.NoError(t, f.registry.ReserveStatic(registryContext, []StaticReservation{second}))
	restarted := f.restart(t)
	require.ErrorIs(t, restarted.CheckManagerToken(registryContext, firstToken), ErrConflict)
	require.ErrorIs(t, restarted.CheckManagerToken(registryContext, secondToken), ErrConflict)
	for _, test := range []struct {
		name        string
		reservation StaticReservation
	}{
		{"retired token", StaticReservation{TenantID: "other-tenant", ApplicationID: "new-static", HubTenantID: "other-hub", Backend: memory.BackendNative, ContainerID: "new-bank", TokenDigest: TokenDigest(firstToken)}},
		{"retired container", StaticReservation{TenantID: "other-tenant", ApplicationID: "new-static", HubTenantID: "other-hub", Backend: memory.BackendNative, ContainerID: strings.ToUpper(first.ContainerID), TokenDigest: TokenDigest(strings.Repeat("4", 64))}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := registryStorageSnapshot(t, f)
			require.ErrorIs(t, restarted.ReserveStatic(registryContext, []StaticReservation{test.reservation}), ErrConflict)
			require.Equal(t, before, registryStorageSnapshot(t, f), "rejected rotation collisions must roll back all namespace claims")
		})
	}
	// A managed create also cannot acquire a container retired by a static rotation.
	f.banks.mu.Lock()
	f.banks.fixedID = first.ContainerID
	f.banks.mu.Unlock()
	_, err := restarted.Create(registryContext, f.owner, createRequest("managed-retired-container"))
	require.ErrorIs(t, err, ErrConflict)
	require.NoError(t, restarted.ReserveManager(registryContext, managerToken, f.owner.TenantID))
	before := registryStorageSnapshot(t, f)
	require.NoError(t, f.restart(t).ReserveManager(registryContext, managerToken, f.owner.TenantID))
	require.Equal(t, before, registryStorageSnapshot(t, f))
	require.ErrorIs(t, f.restart(t).ReserveManager(registryContext, managerToken, "other-tenant"), ErrConflict)
	managerAsApplication := StaticReservation{TenantID: "other-tenant", ApplicationID: "manager-as-application", Backend: memory.BackendNative, ContainerID: "fresh-container", TokenDigest: TokenDigest(managerToken)}
	require.ErrorIs(t, restarted.ReserveStatic(registryContext, []StaticReservation{managerAsApplication}), ErrConflict)
	require.Equal(t, before, registryStorageSnapshot(t, f))
	_, err = restarted.Authenticate(registryContext, managerToken)
	require.ErrorIs(t, err, ErrNotFound)
	raw, err := json.Marshal(registryStorageSnapshot(t, f))
	require.NoError(t, err)
	for _, secret := range []string{firstToken, secondToken, managerToken} {
		require.NotContains(t, string(raw), secret)
	}
}
