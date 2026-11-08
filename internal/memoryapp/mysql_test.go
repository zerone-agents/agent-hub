package memoryapp

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/memory"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The caller owns the exact scratch schema and its cleanup. This test never
// creates/drops a database or deletes/truncates existing data. It deliberately
// leaves its application rows for independent readback by the gate owner.
func TestMySQLMemoryApplications(t *testing.T) {
	raw := os.Getenv("MEMORY_APPLICATION_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("MEMORY_APPLICATION_TEST_MYSQL_DSN is not configured")
	}
	parsed, err := driver.ParseDSN(raw)
	if err != nil {
		t.Fatal("invalid memory application MySQL scratch DSN")
	}
	host, port, addressErr := net.SplitHostPort(parsed.Addr)
	if parsed.Net != "tcp" || addressErr != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") || port == "" || !regexp.MustCompile(`^hub_memory_app_test_[a-f0-9]{8,32}$`).MatchString(parsed.DBName) {
		t.Fatal("memory application MySQL test requires loopback TCP and a dedicated hub_memory_app_test_<8-32 hex digits> scratch database")
	}
	// Time parsing is needed for application readback; these options do not
	// broaden the destination or print the caller's credential-bearing DSN.
	parsed.ParseTime = true
	if parsed.Timeout == 0 {
		parsed.Timeout = 5 * time.Second
	}
	if parsed.ReadTimeout == 0 {
		parsed.ReadTimeout = 15 * time.Second
	}
	if parsed.WriteTimeout == 0 {
		parsed.WriteTimeout = 15 * time.Second
	}
	openPool := func() *gorm.DB {
		db, openErr := gorm.Open(gormmysql.Open(parsed.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if openErr != nil {
			t.Fatal("unable to open memory application MySQL scratch database")
		}
		sql, poolErr := db.DB()
		if poolErr != nil {
			t.Fatal("unable to access memory application MySQL scratch pool")
		}
		sql.SetMaxOpenConns(4)
		sql.SetMaxIdleConns(4)
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr := sql.PingContext(pingCtx)
		cancel()
		if pingErr != nil {
			_ = sql.Close()
			t.Fatal("unable to connect to memory application MySQL scratch database")
		}
		t.Cleanup(func() {
			if sql.Close() != nil {
				t.Error("unable to close memory application MySQL scratch pool")
			}
		})
		return db
	}
	firstDB := openPool()
	var tables int64
	if firstDB.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", parsed.DBName).Scan(&tables).Error != nil {
		t.Fatal("unable to inspect memory application MySQL scratch database")
	}
	require.Zero(t, tables, "memory application scratch database must be empty; test never clears an existing schema")
	secondDB := openPool()
	firstPool, err := firstDB.DB()
	if err != nil {
		t.Fatal("unable to access first scratch pool")
	}
	secondPool, err := secondDB.DB()
	if err != nil {
		t.Fatal("unable to access second scratch pool")
	}
	require.NotSame(t, firstPool, secondPool, "the gate must exercise independent database pools")
	if Migrate(firstDB) != nil {
		t.Fatal("unable to migrate memory application MySQL scratch database")
	}
	if Migrate(secondDB) != nil {
		t.Fatal("second memory application MySQL migration failed")
	}
	wrapper, err := memory.NewSyntheticKeyWrapper(strings.Repeat("ac", 32))
	require.NoError(t, err)
	banks := newTestBanks()
	first, err := NewRegistry(firstDB, wrapper, banks)
	require.NoError(t, err)
	second, err := NewRegistry(secondDB, wrapper, banks)
	require.NoError(t, err)
	owner := Owner{TenantID: "Tenant-A", HubTenantID: "Hub-A", ChatModel: "356", EmbeddingModel: "337", MaxApplications: 20}
	ctx := context.Background()

	// Hold the one provider dispatch while callers through both independent
	// pools observe the durable reservation. Recovery is read-only, even when a
	// concurrent caller initially receives an unknown/pending result.
	gate := make(chan struct{})
	banks.gate = gate
	banks.started = make(chan struct{})
	banks.findStarted = make(chan struct{})
	in := createRequest("same-app")
	type outcome struct {
		result CreateResult
		err    error
	}
	outcomes := make(chan outcome, 8)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(registry *Registry) {
			defer workers.Done()
			<-start
			out, err := registry.Create(ctx, owner, in)
			outcomes <- outcome{out, err}
		}([]*Registry{first, second}[i%2])
	}
	close(start)
	select {
	case <-banks.started:
	case <-time.After(15 * time.Second):
		close(gate)
		t.Fatal("authorized MySQL provisioning dispatch did not start")
	}
	select {
	case <-banks.findStarted:
	case <-time.After(15 * time.Second):
		close(gate)
		t.Fatal("MySQL duplicate creates did not reach read-only recovery")
	}
	close(gate)
	finished := make(chan struct{})
	go func() { workers.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent MySQL creates did not finish")
	}
	close(outcomes)
	secrets := map[string]bool{}
	for out := range outcomes {
		require.True(t, out.err == nil || errors.Is(out.err, ErrUnknown), "concurrent create must be successful or explicitly unknown, got %v", out.err)
		require.Len(t, out.result.ApplicationToken, 64)
		secrets[out.result.ApplicationToken] = true
	}
	require.Len(t, secrets, 1, "all exact-operation receipts must refer to the same issued secret")
	created, err := second.Create(ctx, owner, in)
	require.NoError(t, err)
	require.Equal(t, "active", created.Application.Status)
	creates, finds := banks.counts()
	require.Equal(t, 1, creates)
	require.GreaterOrEqual(t, finds, 1)
	require.True(t, secrets[created.ApplicationToken])
	list, err := first.List(ctx, owner.TenantID, 0, 100)
	require.NoError(t, err)
	require.Equal(t, []Application{created.Application}, list)

	// MySQL's default case-insensitive collation must not merge business or Hub
	// tenant identities. Both tenants intentionally reuse the app/operation IDs.
	otherOwner := owner
	otherOwner.TenantID = "tenant-a"
	otherOwner.HubTenantID = "hub-a"
	other, err := second.Create(ctx, otherOwner, in)
	require.NoError(t, err)
	require.NotEqual(t, created.Application.MemoryID, other.Application.MemoryID)
	for _, test := range []struct {
		owner       Owner
		application Application
	}{{owner, created.Application}, {otherOwner, other.Application}} {
		rows, err := first.List(ctx, test.owner.TenantID, 0, 100)
		require.NoError(t, err)
		require.Equal(t, []Application{test.application}, rows)
		admin, err := second.ForHubTenant(ctx, test.owner.HubTenantID)
		require.NoError(t, err)
		require.Len(t, admin, 1)
		require.Equal(t, test.application.MemoryID, admin[0].MemoryID)
	}
	foreign, err := first.ForHubTenant(ctx, "HUB-A")
	require.NoError(t, err)
	require.Empty(t, foreign)
	_, err = second.Get(ctx, "unknown-tenant", in.ApplicationID, true)
	require.ErrorIs(t, err, ErrNotFound)

	// A second process/pool cannot continue authenticating a disabled app. Admin
	// readback remains available and reenable preserves its original bank/token.
	disabled, err := first.Update(ctx, owner.TenantID, in.ApplicationID, UpdateInput{ExpectedVersion: created.Application.Version, Enabled: boolValue(false)})
	require.NoError(t, err)
	_, err = second.Authenticate(ctx, created.ApplicationToken)
	require.ErrorIs(t, err, ErrDisabled)
	adminDetail, err := second.Get(ctx, owner.TenantID, in.ApplicationID, false)
	require.NoError(t, err)
	require.Equal(t, disabled, adminDetail)
	adminBindings, err := second.ForHubTenant(ctx, owner.HubTenantID)
	require.NoError(t, err)
	require.Len(t, adminBindings, 1)
	require.Equal(t, "disabled", adminBindings[0].Status)
	_, err = second.Update(ctx, owner.TenantID, in.ApplicationID, UpdateInput{ExpectedVersion: created.Application.Version, Enabled: boolValue(true)})
	require.ErrorIs(t, err, ErrConflict)
	active, err := second.Update(ctx, owner.TenantID, in.ApplicationID, UpdateInput{ExpectedVersion: disabled.Version, Enabled: boolValue(true)})
	require.NoError(t, err)
	authorized, err := first.Authenticate(ctx, created.ApplicationToken)
	require.NoError(t, err)
	require.Equal(t, active.MemoryID, authorized.MemoryID)
	requireNoPlainSecret(t, created.ApplicationToken, list, adminDetail, adminBindings, authorized)

	// Rotating a configured static binding does not free its retired secret or
	// container for another app or for a management purpose in another process.
	beforeToken, afterToken, managerToken := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)
	before := StaticReservation{TenantID: "static-tenant", ApplicationID: "rotating", HubTenantID: "static-hub", Backend: memory.BackendNative, ContainerID: "mysql-static-old-bank", TokenDigest: TokenDigest(beforeToken)}
	after := before
	after.TokenDigest = TokenDigest(afterToken)
	after.ContainerID = "mysql-static-new-bank"
	require.NoError(t, first.ReserveStatic(ctx, []StaticReservation{before}))
	require.NoError(t, second.ReserveStatic(ctx, []StaticReservation{after}))
	for _, secret := range []string{beforeToken, afterToken} {
		require.ErrorIs(t, second.CheckManagerToken(ctx, secret), ErrConflict)
		require.ErrorIs(t, second.ReserveManager(ctx, secret, "foreign-manager-tenant"), ErrConflict)
	}
	oldTokenAsApp := StaticReservation{TenantID: "foreign-static", ApplicationID: "reused-secret", Backend: memory.BackendNative, ContainerID: "new-static-bank", TokenDigest: TokenDigest(beforeToken)}
	oldContainerAsApp := StaticReservation{TenantID: "foreign-static", ApplicationID: "reused-container", Backend: memory.BackendNative, ContainerID: strings.ToUpper(before.ContainerID), TokenDigest: TokenDigest(strings.Repeat("4", 64))}
	require.ErrorIs(t, second.ReserveStatic(ctx, []StaticReservation{oldTokenAsApp}), ErrConflict)
	require.ErrorIs(t, first.ReserveStatic(ctx, []StaticReservation{oldContainerAsApp}), ErrConflict)
	banks.mu.Lock()
	banks.fixedID = before.ContainerID
	banks.mu.Unlock()
	_, err = second.Create(ctx, owner, createRequest("retired-bank"))
	require.ErrorIs(t, err, ErrConflict)
	unbound, err := first.Get(ctx, owner.TenantID, "retired-bank", false)
	require.NoError(t, err)
	require.Empty(t, unbound.MemoryID)
	require.NoError(t, first.ReserveManager(ctx, managerToken, owner.TenantID))
	require.NoError(t, second.ReserveManager(ctx, managerToken, owner.TenantID))
	require.ErrorIs(t, second.ReserveManager(ctx, managerToken, otherOwner.TenantID), ErrConflict)
	managerAsApp := StaticReservation{TenantID: "foreign-static", ApplicationID: "manager-as-app", Backend: memory.BackendNative, ContainerID: "manager-must-not-own-bank", TokenDigest: TokenDigest(managerToken)}
	require.ErrorIs(t, first.ReserveStatic(ctx, []StaticReservation{managerAsApp}), ErrConflict)
	_, err = second.Authenticate(ctx, managerToken)
	require.ErrorIs(t, err, ErrNotFound)

	t.Log("isolated MySQL: migration x2; two independent pools; eight exact creates/one fake provider POST and one secret; case-sensitive tenant isolation; cross-registry disable/CAS; retired static purposes and containers retained; no schema creation or cleanup performed")
}
