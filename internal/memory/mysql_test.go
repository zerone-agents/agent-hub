package memory

import (
	"context"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The caller owns creation and exact cleanup of this dedicated scratch schema.
// This test never truncates or drops a schema, and rejects non-scratch DSNs.
func TestMySQLMemoryPhase1(t *testing.T) {
	raw := os.Getenv("MEMORY_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("MEMORY_TEST_MYSQL_DSN is not configured")
	}
	parsed, e := driver.ParseDSN(raw)
	if e != nil {
		t.Fatal("invalid memory MySQL scratch DSN")
	}
	host, port, addressErr := net.SplitHostPort(parsed.Addr)
	if parsed.Net != "tcp" || addressErr != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") || port == "" || !regexp.MustCompile(`^hub_memory_test_[a-f0-9]{8,32}$`).MatchString(parsed.DBName) {
		t.Fatal("memory MySQL test requires a loopback TCP address and a dedicated hub_memory_test_<8-32 hex digits> scratch database")
	}
	db, e := gorm.Open(mysql.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("unable to open memory MySQL scratch database")
	}
	sql, e := db.DB()
	require.NoError(t, e)
	defer sql.Close()
	var tableCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", parsed.DBName).Scan(&tableCount).Error)
	require.Zero(t, tableCount, "scratch database must be empty")
	require.NoError(t, Migrate(db))
	require.NoError(t, Migrate(db))
	wrapper, e := NewSyntheticKeyWrapper(strings.Repeat("01", 32))
	require.NoError(t, e)
	index := newTestIndex()
	cfg := Config{TenantID: "tenant", ApplicationID: "app", PolicyVersion: "policy-1", DatasetID: "dataset", SyntheticOnly: true, Wrapper: wrapper, Index: index}
	s, e := NewService(db, cfg)
	require.NoError(t, e)
	second, e := NewService(db, cfg)
	require.NoError(t, e)
	sc := Scope{"tenant", "app", "alice", "life-1"}
	ctx := context.Background()
	_, e = s.Activate(ctx, sc, "activate")
	require.NoError(t, e)
	subjects, e := second.ListSubjects(ctx)
	require.NoError(t, e)
	require.Len(t, subjects.Subjects, 1)
	require.Equal(t, sc, subjects.Subjects[0].Scope)
	require.NoError(t, db.Model(&lifecycleRow{}).Where("id = ?", scopeKey(sc)).Update("scope_payload", nil).Error)
	subjects, e = second.ListSubjects(ctx)
	require.NoError(t, e)
	require.Equal(t, 1, subjects.MissingScopeCount)
	_, e = s.List(ctx, sc)
	require.NoError(t, e)
	subjects, e = second.ListSubjects(ctx)
	require.NoError(t, e)
	require.Len(t, subjects.Subjects, 1)
	require.Equal(t, sc, subjects.Subjects[0].Scope)
	for n := 0; n < 100; n++ {
		in := input("我" + strings.Repeat("合", 199))
		in.SourceMessageID = uuid.NewString() + strings.Repeat("m", 90)
		in.TurnID = uuid.NewString() + strings.Repeat("t", 90)
		_, e = s.SubmitCandidate(ctx, sc, in)
		require.NoError(t, e)
	}
	task, e := second.ClaimTask(ctx, sc, "large-task")
	require.NoError(t, e)
	require.Len(t, task.Candidates, 100)
	var storedTask taskRow
	require.NoError(t, db.First(&storedTask, "id = ?", task.ID).Error)
	require.Greater(t, len(storedTask.Payload), 65535)
	items := []PublishItem{}
	for _, c := range task.Candidates {
		items = append(items, PublishItem{c.ID, c.Version, c.Quote})
	}
	facts, e := s.Publish(ctx, sc, PublishInput{OperationID: "large-publish", TaskID: task.ID, Items: items})
	require.NoError(t, e)
	require.Len(t, facts, 100)
	var storedOp operationRow
	require.NoError(t, db.First(&storedOp, "id = ?", digest([]string{scopeKey(sc), "large-publish"})).Error)
	require.Greater(t, len(storedOp.Result), 65535)
	op, e := second.GetOperation(ctx, sc, "large-publish")
	require.NoError(t, e)
	require.Equal(t, "completed", op.Status)
	transient, e := s.SubmitCandidate(ctx, sc, input("我刚刚喝了一杯水。"))
	require.NoError(t, e)
	transientTask, e := second.ClaimTask(ctx, sc, "transient-task")
	require.NoError(t, e)
	require.Len(t, transientTask.Candidates, 1)
	rejected, e := s.RejectCandidate(ctx, sc, transient.ID, 1, "notDurable", "reject-transient")
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.Status)
	nextTask, e := second.ClaimTask(ctx, sc, "after-transient-rejection")
	require.NoError(t, e)
	require.Empty(t, nextTask.ID)
	var columnTypes []struct {
		ColumnName string
		DataType   string
	}
	require.NoError(t, db.Raw("SELECT COLUMN_NAME AS column_name, DATA_TYPE AS data_type FROM information_schema.columns WHERE table_schema = ? AND ((table_name = 'memory_tasks' AND column_name = 'payload') OR (table_name = 'memory_operations' AND column_name = 'result'))", parsed.DBName).Scan(&columnTypes).Error)
	require.Len(t, columnTypes, 2)
	for _, column := range columnTypes {
		require.Equal(t, "mediumblob", column.DataType)
	}
	// A second service sees the same ciphertext-backed ledger and enforces CAS.
	f := facts[0]
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, service := range []*Service{s, second} {
		wg.Add(1)
		go func(n int, service *Service) {
			defer wg.Done()
			_, err := service.Update(ctx, sc, f.ID, 1, []string{"我喜欢蓝色。", "我喜欢绿色。"}[n], uuid.NewString())
			results <- err
		}(i, service)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrConflict)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	other := sc
	other.UserID = "bob"
	_, e = second.Activate(ctx, other, "activate-bob")
	require.NoError(t, e)
	_, e = second.Get(ctx, other, f.ID)
	require.ErrorIs(t, e, ErrNotFound)
	other = sc
	other.ApplicationID = "another-app"
	_, e = second.List(ctx, other)
	require.ErrorIs(t, e, ErrForbidden)
	updated, e := second.Get(ctx, sc, f.ID)
	require.NoError(t, e)
	require.Equal(t, uint64(2), updated.Version)
	_, e = s.Delete(ctx, sc, f.ID, 2, "delete")
	require.NoError(t, e)
	syncAll(t, s)
	receipt, e := second.GetOperation(ctx, sc, "delete")
	require.NoError(t, e)
	require.True(t, receipt.PhysicalCleanup)
	require.Equal(t, "retrievalReplica", receipt.CleanupScope)
	require.True(t, receipt.LedgerRetained)
	_, e = s.Deactivate(ctx, sc, "close")
	require.NoError(t, e)
	next := sc
	next.LifecycleID = "life-2"
	_, e = second.Activate(ctx, next, "reregister")
	require.NoError(t, e)
	syncAll(t, s)
	receipt, e = second.GetOperation(ctx, sc, "close")
	require.NoError(t, e)
	require.True(t, receipt.PhysicalCleanup)
	require.Equal(t, "hubAndRetrievalReplica", receipt.CleanupScope)
	require.False(t, receipt.LedgerRetained)
	_, e = s.Activate(ctx, sc, "reuse")
	require.ErrorIs(t, e, ErrConflict)
	t.Logf("isolated MySQL migration x2; 100-candidate task=%d bytes, publish receipt=%d bytes; mediumblob readback, cross-service CAS, scope denial and lifecycle cleanup passed", len(storedTask.Payload), len(storedOp.Result))
}
