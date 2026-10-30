package repository

// H2 并发反例（issue #232 计划）：真实 MySQL 行锁/唯一约束语义验证。
// 默认跳过；设置 TEST_MYSQL_DSN 后生效（须指向专用测试 MySQL 实例）——
// SQLite 单写者序列化掩盖不了这里要验证的锁竞争，缺失 DSN 记为门槛未完成。

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupSyncMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN 未设置，跳过 MySQL 同步并发测试")
	}
	// 管理连接：重建专用库（不用 USE——并发用例需要连接池多连接）。库名按
	// 运行时间唯一化（评审修订 S8：共享 DSN 下并行测试互不干扰）
	dbName := fmt.Sprintf("hub_sync_v2_test_%d", time.Now().UnixNano())
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, admin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).Error)
	require.NoError(t, admin.Exec(fmt.Sprintf("CREATE DATABASE %s CHARACTER SET utf8mb4", dbName)).Error)
	sqlAdmin, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, sqlAdmin.Close())

	// 改写 DSN 定位专用库（ParseDSN 兼容各 DSN 形态）
	cfg, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	cfg.DBName = dbName
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.SessionSyncState{}))
	// ChatRepository 经全局 database.GetDB() 取库
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

// TestChatSyncConcurrentFirstClaim_MySQL：两 source 同时首次 claim 同一新键
// ——唯一约束保证恰一个赢家（计划 Review Focus：首次键不存在时双写）。
func TestChatSyncConcurrentFirstClaim_MySQL(t *testing.T) {
	db := setupSyncMySQLDB(t)
	repo := NewChatSyncRepository(db)
	ctx := context.Background()
	owner := syncOwner("ct", "cu1")

	start := make(chan struct{})
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i, src := range []string{syncSrcA, syncSrcB} {
		wg.Add(1)
		go func(i int, src string) {
			defer wg.Done()
			<-start
			_, results[i] = repo.Claim(ctx, owner, "s-race", src, "imported")
		}(i, src)
	}
	close(start)
	wg.Wait()

	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, chat.ErrSyncClaimConflict, "输家必须是 ownership 冲突而非其他错误")
		}
	}
	require.Equal(t, 1, winners, "恰一个赢家：%v", results)

	st, err := repo.GetState(ctx, owner, "s-race")
	require.NoError(t, err)
	require.Equal(t, chat.StateClaimedEmpty, st.Status)
	require.Contains(t, []string{syncSrcA, syncSrcB}, st.SourceID)
}

// TestChatSyncConcurrentClaimVsV1Push_MySQL：claim 与 v1 push 同时竞争同键
// （计划 Review Focus：不能绕开 ownership 检查）。合法终态二选一：
//   - claim 先提交 → v1 该会话被跳过（隔离生效）；
//   - v1 先提交   → claim 分类为 claimed-legacy。
//
// 非法终态：v1 写入成功 且 状态为 claimed-empty（= claim 漏看 v1 正文）。
func TestChatSyncConcurrentClaimVsV1Push_MySQL(t *testing.T) {
	db := setupSyncMySQLDB(t)
	chatRepo := NewChatRepository()
	repo := NewChatSyncRepository(db)
	ctx := context.Background()
	owner := syncOwner("ct", "cu2")

	for round := 0; round < 5; round++ {
		sid := "s-cv-" + time.Now().Format("150405.000000000")
		start := make(chan struct{})
		var (
			wg        sync.WaitGroup
			claimErr  error
			pushRes   *PushResult
			pushPanic interface{}
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, claimErr = repo.Claim(ctx, owner, sid, syncSrcA, "imported")
		}()
		go func() {
			defer wg.Done()
			<-start
			defer func() { pushPanic = recover() }()
			pushRes, _ = chatRepo.PushSessions(owner.TenantID, owner.UserID,
				[]*chat.Session{{
					TenantID: owner.TenantID, UserID: owner.UserID, ID: sid,
					Title: "v1-race", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
				}},
				[][]*chat.Message{{
					{TenantID: owner.TenantID, UserID: owner.UserID, SessionID: sid,
						ID: sid + "-m1", Role: "user", Content: "v1-race", CreatedAt: time.Now().UTC()},
				}})
		}()
		close(start)
		wg.Wait()
		require.Nil(t, pushPanic)

		require.NoError(t, claimErr)
		require.NotNil(t, pushRes)
		st, err := repo.GetState(ctx, owner, sid)
		require.NoError(t, err)

		v1Wrote := pushRes.SyncedSessions == 1
		if v1Wrote {
			require.Equal(t, chat.StateClaimedLegacy, st.Status,
				"v1 先落地时 claim 必须识别为 legacy，round=%d", round)
		} else {
			require.Equal(t, 1, pushRes.SkippedSessions)
			require.Equal(t, chat.StateClaimedEmpty, st.Status,
				"claim 先落地时 v1 必须被拒，round=%d", round)
		}
	}
}

// TestChatSyncConcurrentSameVersionDiffDigest_MySQL：两 envelope 同 version、
// 异摘要并发 apply——行锁串行化：恰一个 applied，另一个 conflict；展示内容
// 与已确认身份都等于赢家。附带验证版本 10 后版本 9 → superseded。
func TestChatSyncConcurrentSameVersionDiffDigest_MySQL(t *testing.T) {
	db := setupSyncMySQLDB(t)
	repo := NewChatSyncRepository(db)
	ctx := context.Background()
	owner := syncOwner("ct", "cu3")

	_, err := repo.Claim(ctx, owner, "s-sv", syncSrcA, "imported")
	require.NoError(t, err)

	envA := mkEnvelope(t, syncSrcA, "s-sv", 10, chat.KindReplace, 1)
	envB := mkEnvelopeAltDigest(t, syncSrcA, "s-sv", 10, 1)

	start := make(chan struct{})
	replies := make([]chat.Reply, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, env := range []chat.Envelope{envA, envB} {
		wg.Add(1)
		go func(i int, env chat.Envelope) {
			defer wg.Done()
			<-start
			replies[i], errs[i] = repo.Apply(ctx, owner, env)
		}(i, env)
	}
	close(start)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	statuses := []string{replies[0].Status, replies[1].Status}
	require.Contains(t, statuses, chat.StatusApplied)
	require.Contains(t, statuses, chat.StatusConflict)
	require.NotEqual(t, replies[0].Status, replies[1].Status, "必须分出胜负：%v", statuses)

	st, err := repo.GetState(ctx, owner, "s-sv")
	require.NoError(t, err)
	require.Equal(t, "10", st.Version)
	var winnerDigest string
	for _, r := range replies {
		if r.Status == chat.StatusApplied {
			winnerDigest = r.State.Digest
		}
	}
	require.NotEmpty(t, winnerDigest)
	require.Equal(t, winnerDigest, st.Digest)

	// 降版本 → superseded（真实 MySQL 行锁下读当前身份）
	r, err := repo.Apply(ctx, owner, mkEnvelope(t, syncSrcA, "s-sv", 9, chat.KindReplace, 1))
	require.NoError(t, err)
	require.Equal(t, chat.StatusSuperseded, r.Status)
	require.Equal(t, "10", r.State.Version)
	require.Equal(t, st.Digest, r.State.Digest)
}
