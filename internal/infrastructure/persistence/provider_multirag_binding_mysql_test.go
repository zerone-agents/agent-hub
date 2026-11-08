package repository

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/provider"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TEST_MYSQL_DSN is only a connection seed. Both repository handles use a new,
// randomly named schema; no migration, delete or table write targets its DBName.
// Two independent sql.DB pools exercise CAS across Hub process boundaries.
func setupProviderBindingMySQL(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("TEST_MYSQL_DSN could not be parsed")
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	quiet := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(mysql.Open(cfg.FormatDSN()), quiet)
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	var version string
	require.NoError(t, admin.Raw("SELECT VERSION()").Scan(&version).Error)
	require.True(t, strings.HasPrefix(version, "8."), "requires real MySQL8, got %s", version)
	var nonce [8]byte
	require.NoError(t, func() error { _, err := rand.Read(nonce[:]); return err }())
	name := fmt.Sprintf("hub_binding_%d_%s", time.Now().UnixNano(), hex.EncodeToString(nonce[:]))
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4").Error)
	t.Logf("isolated_schema=%s mysql_version=%s", name, version)
	var handles []*gorm.DB
	t.Cleanup(func() {
		for _, db := range handles {
			if sqlDB, err := db.DB(); err == nil {
				if err := sqlDB.Close(); err != nil {
					t.Errorf("close isolated test pool: %v", err)
				}
			}
		}
		if err := admin.Exec("DROP DATABASE `" + name + "`").Error; err != nil {
			t.Errorf("drop isolated schema %s: %v", name, err)
		}
		var remaining int64
		require.NoError(t, admin.Raw("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", name).Scan(&remaining).Error)
		require.Zero(t, remaining, "test schema must be removed")
		t.Logf("dropped_schema=%s remaining=%d", name, remaining)
	})
	cfg.DBName = name
	for i := 0; i < 2; i++ {
		db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), quiet)
		require.NoError(t, err)
		handles = append(handles, db)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(4)
		sqlDB.SetMaxIdleConns(4)
	}
	require.NoError(t, handles[0].AutoMigrate(&provider.ProviderSummary{}, &provider.MultiRAGSyncBinding{}))
	return handles[0], handles[1]
}

func newProviderBindingFixture(id uint64, target, name, lease string) provider.MultiRAGSyncBinding {
	return provider.MultiRAGSyncBinding{ProviderID: id, Target: target, InstanceName: name, Lease: lease, ConfigHash: "hash-initial", State: "creating"}
}

func TestProviderMultiRAGBindingOwnershipMySQL(t *testing.T) {
	db, otherDB := setupProviderBindingMySQL(t)
	owner := seedProvider(t, db, "binding-owner", "owned", "")
	shared := seedProvider(t, db, "", "shared", "")
	repo := &ProviderRepository{db: db}
	other := &ProviderRepository{db: otherDB}
	binding := newProviderBindingFixture(owner.ID, "target-a", "reserved-a", "lease-owner")
	created, err := repo.ReserveMultiRAGBinding("binding-owner", &binding)
	require.NoError(t, err)
	require.True(t, created)
	for _, inaccessible := range []struct {
		tenant string
		id     uint64
	}{{"binding-other", owner.ID}, {"binding-owner", shared.ID}, {"binding-owner", owner.ID + shared.ID + 1000}} {
		t.Run(fmt.Sprintf("%s/%d", inaccessible.tenant, inaccessible.id), func(t *testing.T) {
			attempt := newProviderBindingFixture(inaccessible.id, "target-a", "reserved-a", "lease-owner")
			_, err := other.GetMultiRAGBinding(inaccessible.tenant, inaccessible.id, "target-a")
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			created, err := other.ReserveMultiRAGBinding(inaccessible.tenant, &attempt)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			require.False(t, created)
			require.ErrorIs(t, other.ConfirmMultiRAGBinding(inaccessible.tenant, &attempt), gorm.ErrRecordNotFound)
			acquired, err := other.BeginMultiRAGSync(inaccessible.tenant, &attempt)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			require.False(t, acquired)
			require.ErrorIs(t, other.ReleaseMultiRAGBinding(inaccessible.tenant, &attempt), gorm.ErrRecordNotFound)
		})
	}
	saved, err := repo.GetMultiRAGBinding("binding-owner", owner.ID, "target-a")
	require.NoError(t, err)
	require.Equal(t, binding.InstanceName, saved.InstanceName)
	require.Equal(t, binding.Lease, saved.Lease)
	missing, err := repo.GetMultiRAGBinding("binding-owner", owner.ID, "target-b")
	require.NoError(t, err)
	require.Nil(t, missing)
	second := newProviderBindingFixture(owner.ID, "target-b", "reserved-b", "lease-b")
	created, err = other.ReserveMultiRAGBinding("binding-owner", &second)
	require.NoError(t, err)
	require.True(t, created, "different upstream targets must have independent bindings")
}

func TestProviderMultiRAGBindingUniqueReservationMySQL(t *testing.T) {
	db, otherDB := setupProviderBindingMySQL(t)
	owner := seedProvider(t, db, "binding-owner", "race", "")
	repos := []*ProviderRepository{{db: db}, {db: otherDB}}
	for round := 0; round < 5; round++ {
		target := fmt.Sprintf("race-%d", round)
		candidates := []provider.MultiRAGSyncBinding{
			newProviderBindingFixture(owner.ID, target, "instance-a", "lease-a"),
			newProviderBindingFixture(owner.ID, target, "instance-b", "lease-b"),
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wins, errs := make([]bool, 2), make([]error, 2)
		for i := range repos {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				wins[i], errs[i] = repos[i].ReserveMultiRAGBinding("binding-owner", &candidates[i])
			}(i)
		}
		close(start)
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		require.NotEqual(t, wins[0], wins[1], "exactly one process reserves the absent key")
		winner := 0
		if wins[1] {
			winner = 1
		}
		saved, err := repos[1-winner].GetMultiRAGBinding("binding-owner", owner.ID, target)
		require.NoError(t, err)
		require.Equal(t, candidates[winner].InstanceName, saved.InstanceName)
		require.Equal(t, candidates[winner].Lease, saved.Lease)
		var count int64
		require.NoError(t, db.Model(&provider.MultiRAGSyncBinding{}).Where("provider_id = ? AND target = ?", owner.ID, target).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
}

func TestProviderMultiRAGBindingCrossProcessCASMySQL(t *testing.T) {
	db, otherDB := setupProviderBindingMySQL(t)
	owner := seedProvider(t, db, "binding-owner", "cas", "")
	repos := []*ProviderRepository{{db: db}, {db: otherDB}}
	initial := newProviderBindingFixture(owner.ID, "target", "stable-instance", "create-owner")
	created, err := repos[0].ReserveMultiRAGBinding("binding-owner", &initial)
	require.NoError(t, err)
	require.True(t, created)
	initial.InstanceID = "upstream-instance"
	require.NoError(t, repos[0].ConfirmMultiRAGBinding("binding-owner", &initial))
	ready, err := repos[1].GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.Equal(t, "ready", ready.State)
	require.Empty(t, ready.Lease)
	require.Greater(t, ready.Version, initial.Version)
	candidates := []provider.MultiRAGSyncBinding{*ready, *ready}
	candidates[0].Lease, candidates[0].ConfigHash = "sync-owner-a", "hash-a"
	candidates[1].Lease, candidates[1].ConfigHash = "sync-owner-b", "hash-b"
	start := make(chan struct{})
	var wg sync.WaitGroup
	wins, errs := make([]bool, 2), make([]error, 2)
	for i := range repos {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			wins[i], errs[i] = repos[i].BeginMultiRAGSync("binding-owner", &candidates[i])
		}(i)
	}
	close(start)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.NotEqual(t, wins[0], wins[1])
	winner := 0
	if wins[1] {
		winner = 1
	}
	active, err := repos[1-winner].GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.Equal(t, "syncing", active.State)
	require.Equal(t, ready.Version+1, active.Version)
	require.Equal(t, candidates[winner].Lease, active.Lease)
	require.Equal(t, candidates[winner].ConfigHash, active.ConfigHash)
	// Neither stale versions nor another process's lease can acknowledge success.
	require.ErrorIs(t, repos[1-winner].ConfirmMultiRAGBinding("binding-owner", ready), gorm.ErrRecordNotFound)
	wrongLease := *active
	wrongLease.Lease = candidates[1-winner].Lease
	require.ErrorIs(t, repos[1-winner].ConfirmMultiRAGBinding("binding-owner", &wrongLease), gorm.ErrRecordNotFound)
	wrongVersion := *active
	wrongVersion.Version--
	require.ErrorIs(t, repos[winner].ConfirmMultiRAGBinding("binding-owner", &wrongVersion), gorm.ErrRecordNotFound)
	require.NoError(t, repos[winner].ConfirmMultiRAGBinding("binding-owner", active))
	confirmed, err := repos[1-winner].GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.Equal(t, "ready", confirmed.State)
	require.Equal(t, active.Version+1, confirmed.Version)
	require.Empty(t, confirmed.Lease)
	// Reusing the previously valid lease after confirmation cannot confirm again.
	require.ErrorIs(t, repos[winner].ConfirmMultiRAGBinding("binding-owner", active), gorm.ErrRecordNotFound)
}

func TestProviderMultiRAGBindingPendingCannotBeAcquiredOrReleasedMySQL(t *testing.T) {
	db, otherDB := setupProviderBindingMySQL(t)
	owner := seedProvider(t, db, "binding-owner", "pending", "")
	repo, other := &ProviderRepository{db: db}, &ProviderRepository{db: otherDB}
	binding := newProviderBindingFixture(owner.ID, "target", "stable-instance", "creating-owner")
	created, err := repo.ReserveMultiRAGBinding("binding-owner", &binding)
	require.NoError(t, err)
	require.True(t, created)
	wrongName := binding
	wrongName.InstanceName = "foreign-reservation"
	require.NoError(t, other.ReleaseMultiRAGBinding("binding-owner", &wrongName))
	stillReserved, err := repo.GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.NotNil(t, stillReserved)
	// Releasing an undispatched create permits a genuinely new reservation.
	require.NoError(t, repo.ReleaseMultiRAGBinding("binding-owner", &binding))
	missing, err := other.GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.Nil(t, missing)
	binding = newProviderBindingFixture(owner.ID, "target", "new-reservation", "new-creating-owner")
	created, err = other.ReserveMultiRAGBinding("binding-owner", &binding)
	require.NoError(t, err)
	require.True(t, created)
	binding.InstanceID = "upstream-instance"
	require.NoError(t, other.ConfirmMultiRAGBinding("binding-owner", &binding))
	ready, err := repo.GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	ready.Lease = "active-sync-owner"
	acquired, err := repo.BeginMultiRAGSync("binding-owner", ready)
	require.NoError(t, err)
	require.True(t, acquired)
	pending, err := other.GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.Equal(t, "syncing", pending.State)
	// An ambiguous write leaves the durable in-flight record. A fresh process
	// cannot acquire or release it merely by rereading its current version.
	contender := *pending
	contender.Lease = "replacement-owner"
	acquired, err = other.BeginMultiRAGSync("binding-owner", &contender)
	require.NoError(t, err)
	require.False(t, acquired)
	require.NoError(t, other.ReleaseMultiRAGBinding("binding-owner", &contender))
	require.ErrorIs(t, other.ConfirmMultiRAGBinding("binding-owner", &contender), gorm.ErrRecordNotFound)
	require.ErrorIs(t, repo.ConfirmMultiRAGBinding("binding-owner", ready), gorm.ErrRecordNotFound)
	unchanged, err := repo.GetMultiRAGBinding("binding-owner", owner.ID, "target")
	require.NoError(t, err)
	require.Equal(t, *pending, *unchanged)
}
