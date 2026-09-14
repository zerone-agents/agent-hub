// H7.5 UsageService 测试：埋点零阻断、实时聚合正确性、软预算 denied、
// CSV 导出格式、健康状态合成、跨租户隔离。
package services

import (
	"bytes"
	"testing"
	"time"

	"control-panel/internal/domain/systemsetting"
	"control-panel/internal/domain/usage"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func usageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&usage.UsageRecord{}, &usage.UsageBudget{}, &usage.UsageAlert{}, &usage.UsageAlertEvent{}, &systemsetting.SystemSetting{}))
	return db
}

// newUsageSvc 构造服务并注册清理。
func newUsageSvc(t *testing.T, db *gorm.DB) *UsageService {
	t.Helper()
	svc := NewUsageService(db)
	t.Cleanup(svc.Close)
	return svc
}

// flushUsage 等待后台落库把 n 条记录写完。
func flushUsage(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int64
		db.Model(&usage.UsageRecord{}).Count(&count)
		return count >= int64(n)
	}, 3*time.Second, 10*time.Millisecond)
}

func i64(v int64) *int64 { return &v }

func TestUsageRecordNilServiceAndPanicSafe(t *testing.T) {
	// nil 接收者：绝不 panic（Recover 在方法入口 defer）。
	var nilSvc *UsageService
	require.NotPanics(t, func() {
		nilSvc.Record(UsageRecordInput{Kind: usage.KindModelCall, TenantID: "t"})
	})
	// 空 kind / 空 tenant：静默忽略。
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	require.NotPanics(t, func() {
		svc.Record(UsageRecordInput{Kind: "", TenantID: "t"})
		svc.Record(UsageRecordInput{Kind: usage.KindModelCall, TenantID: ""})
	})
	// 主流程模拟：Record 之后再执行的业务步骤照常跑完（零阻断）。
	order := []string{}
	svc.Record(UsageRecordInput{Kind: usage.KindModelCall, TenantID: "t"})
	order = append(order, "after-record")
	require.Equal(t, []string{"after-record"}, order)
}

func TestUsageRecordQueueFullDropsAndCounts(t *testing.T) {
	db := usageTestDB(t)
	// 手工构造：队列容量 2 且无后台 worker，模拟"满了丢弃计数"。
	svc := &UsageService{db: db, queue: make(chan *usage.UsageRecord, 2)}
	for i := 0; i < 5; i++ {
		svc.Record(UsageRecordInput{Kind: usage.KindMessage, TenantID: "t"})
	}
	require.Equal(t, int64(3), svc.DroppedCount())
	require.Equal(t, 2, len(svc.queue))
}

func TestUsageSummaryAggregatesCorrectly(t *testing.T) {
	db := usageTestDB(t)
	day1 := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	rows := []*usage.UsageRecord{
		{TenantID: "ta", Kind: usage.KindModelCall, Model: "m1", TokensIn: i64(100), TokensOut: i64(50), LatencyMs: i64(100), Error: "", CreatedAt: day1},
		{TenantID: "ta", Kind: usage.KindModelCall, Model: "m1", TokensIn: i64(200), TokensOut: i64(100), LatencyMs: i64(300), Error: "boom", CreatedAt: day1.Add(2 * time.Hour)},
		{TenantID: "ta", Kind: usage.KindMessage, TokensOut: i64(10), CreatedAt: day1.Add(3 * time.Hour)},
		{TenantID: "ta", Kind: usage.KindModelCall, Model: "m2", CreatedAt: day1.Add(25 * time.Hour)}, // 次日
		{TenantID: "tb", Kind: usage.KindModelCall, TokensIn: i64(999), CreatedAt: day1},             // 他租户
	}
	require.NoError(t, db.Create(&rows).Error)
	svc := newUsageSvc(t, db)

	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	out, err := svc.Summary("ta", start, end)
	require.NoError(t, err)

	byKind := map[string]KindSummary{}
	for _, k := range out.Kinds {
		byKind[k.Kind] = k
	}
	mc := byKind[usage.KindModelCall]
	require.Equal(t, int64(3), mc.TotalCalls)                 // tb 不计
	require.Equal(t, int64(450), mc.TotalTokens)              // (100+50)+(200+100)，in+out
	require.Equal(t, int64(1), mc.TotalErrors)
	require.InDelta(t, 200.0, mc.AvgLatencyMs, 0.01)          // (100+300+NULL)/2
	require.Equal(t, int64(10), byKind[usage.KindMessage].TotalTokens)

	// 趋势：9-10 两天 model_call = 2 与 1。
	trend := map[string]int64{}
	for _, tr := range out.Trends {
		if tr.Kind == usage.KindModelCall {
			trend[tr.Date] = tr.Calls
		}
	}
	require.Equal(t, int64(2), trend["2026-09-10"])
	require.Equal(t, int64(1), trend["2026-09-11"])
}

func TestUsageByExtensionAndByModelAndErrors(t *testing.T) {
	db := usageTestDB(t)
	day := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	rows := []*usage.UsageRecord{
		{TenantID: "ta", Kind: usage.KindExtensionCall, ExtensionName: "ext.a", LatencyMs: i64(10), CreatedAt: day},
		{TenantID: "ta", Kind: usage.KindExtensionCall, ExtensionName: "ext.a", Error: "x", CreatedAt: day.Add(time.Hour)},
		{TenantID: "ta", Kind: usage.KindExtensionCall, ExtensionName: "ext.b", CreatedAt: day.Add(2 * time.Hour)},
		{TenantID: "ta", Kind: usage.KindModelCall, Model: "m1", CreatedAt: day},
		{TenantID: "ta", Kind: usage.KindModelCall, Model: "m1", Error: "x", CreatedAt: day.Add(time.Hour)},
		{TenantID: "tb", Kind: usage.KindModelCall, Model: "m1", Error: "x", CreatedAt: day},
	}
	require.NoError(t, db.Create(&rows).Error)
	svc := newUsageSvc(t, db)
	start := day
	end := day.AddDate(0, 0, 2)

	ext, err := svc.ByExtension("ta", start, end, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), ext.Total)
	require.Equal(t, "ext.a", ext.Items[0].Key)
	require.Equal(t, int64(2), ext.Items[0].TotalCalls)

	mdl, err := svc.ByModel("ta", start, end, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), mdl.Total)
	require.Equal(t, int64(1), mdl.Items[0].TotalErrors)

	run, err := svc.ByRun("ta", start, end, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(0), run.Total) // 无 run_id 记录

	errs, err := svc.Errors("ta", start, end)
	require.NoError(t, err)
	require.Len(t, errs, 2)
	require.Equal(t, int64(1), errs[0].Count)
}

func TestUsageBudgetDeniedMarkingAndProgress(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	now := time.Now().UTC()

	// 预置 2 条 model_call（当天），预算 limit=2 → 下一条 denied。
	base := time.Date(now.Year(), now.Month(), now.Day(), 1, 0, 0, 0, time.UTC)
	seed := []*usage.UsageRecord{
		{TenantID: "ta", Kind: usage.KindModelCall, CreatedAt: base},
		{TenantID: "ta", Kind: usage.KindModelCall, CreatedAt: base.Add(time.Minute)},
	}
	require.NoError(t, db.Create(&seed).Error)

	b, err := svc.UpsertBudget("ta", BudgetInput{
		Kind: usage.BudgetKindModelCall, LimitValue: 2, Period: usage.PeriodDaily, AlertThresholdPct: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), b.LimitValue)

	svc.Record(UsageRecordInput{Kind: usage.KindModelCall, TenantID: "ta"})
	flushUsage(t, db, 3)

	var rec usage.UsageRecord
	require.NoError(t, db.Where("denied = true").First(&rec).Error)
	require.Equal(t, "ta", rec.TenantID)
	require.True(t, rec.Denied)

	// 进度：used=3 / limit=2 = 150%。
	views, err := svc.Budgets("ta", time.Now())
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, int64(3), views[0].UsedValue)
	require.InDelta(t, 150.0, views[0].UsedPct, 0.01)

	// 阈值告警事件已落库。
	var events []usage.UsageAlertEvent
	require.NoError(t, db.Where("tenant_id=? AND rule=?", "ta", usage.RuleBudgetPct).Find(&events).Error)
	require.NotEmpty(t, events)
}

func TestUsageBudgetValidationAndDelete(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	_, err := svc.UpsertBudget("ta", BudgetInput{Kind: "bad", LimitValue: 1, Period: usage.PeriodDaily})
	require.Error(t, err)
	_, err = svc.UpsertBudget("ta", BudgetInput{Kind: usage.BudgetKindModelCall, LimitValue: 0, Period: usage.PeriodDaily})
	require.Error(t, err)
	b, err := svc.UpsertBudget("ta", BudgetInput{Kind: usage.BudgetKindModelCall, LimitValue: 10, Period: usage.PeriodDaily})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteBudget("ta", b.ID))
	require.ErrorIs(t, svc.DeleteBudget("ta", b.ID), gorm.ErrRecordNotFound)
}

func TestUsagePricingCostCalculation(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	// 单价：input 1 分/百万 → 100 micros；output 2 分/百万 → 200 micros。
	require.NoError(t, svc.SetPricingRaw(`{"m1":{"input_per_million":1,"output_per_million":2}}`))

	svc.Record(UsageRecordInput{Kind: usage.KindModelCall, TenantID: "ta", Model: "m1",
		TokensIn: i64(1_000_000), TokensOut: i64(1_000_000)})
	svc.Record(UsageRecordInput{Kind: usage.KindModelCall, TenantID: "ta", Model: "unknown",
		TokensIn: i64(1_000_000)})
	flushUsage(t, db, 2)

	var withCost, noCost usage.UsageRecord
	require.NoError(t, db.Where("model='m1'").First(&withCost).Error)
	require.NotNil(t, withCost.CostMicros)
	require.Equal(t, int64(300), *withCost.CostMicros) // 100 + 200
	require.NoError(t, db.Where("model='unknown'").First(&noCost).Error)
	require.Nil(t, noCost.CostMicros) // 无配置 → NULL（未知≠免费）
}

func TestUsageExportCSVFormat(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	day := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&usage.UsageRecord{
		TenantID: "ta", Kind: usage.KindModelCall, Model: "m1", TokensIn: i64(5),
		LatencyMs: i64(7), Error: "oops", CreatedAt: day,
	}).Error)
	require.NoError(t, db.Create(&usage.UsageRecord{
		TenantID: "tb", Kind: usage.KindModelCall, CreatedAt: day,
	}).Error)

	var buf bytes.Buffer
	start := day.AddDate(0, 0, -1)
	require.NoError(t, svc.ExportCSV(&buf, "ta", start, day.AddDate(0, 0, 1), ""))
	out := buf.String()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.Len(t, lines, 2) // 表头 + 1 行（tb 被租户隔离排除）
	require.Contains(t, out, "id,kind,agent_id,run_id,extension_name,model")
	require.Contains(t, out, "model_call")
	require.Contains(t, out, "oops")
	require.Contains(t, out, ",5,") // tokens_in=5
}

func TestUsageHealthComposition(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	rep := svc.Health("ta")
	require.Equal(t, "green", rep.Status)
	require.Equal(t, "ok", rep.DB)
	require.GreaterOrEqual(t, rep.UptimeSec, int64(0))
}

func TestUsageHealthRedOnDBFailure(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	rep := svc.Health("ta")
	require.Equal(t, "red", rep.Status)
	require.Equal(t, "unhealthy", rep.DB)
}

func TestUsageHealthErrorRateThresholds(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	now := time.Now().UTC()
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var rows []*usage.UsageRecord
	for i := 0; i < 10; i++ {
		errText := ""
		if i < 1 { // 10% 错误率 → yellow
			errText = "e"
		}
		rows = append(rows, &usage.UsageRecord{TenantID: "ta", Kind: usage.KindModelCall, Error: errText, CreatedAt: base.Add(time.Duration(i) * time.Minute)})
	}
	require.NoError(t, db.Create(&rows).Error)
	rep := svc.Health("ta")
	require.Equal(t, "yellow", rep.Status)
	require.InDelta(t, 10.0, rep.TodayErrorRate, 0.01)

	// error_rate_spike 告警（阈值 5）。
	a, err := svc.UpsertAlert("ta", AlertInput{Rule: usage.RuleErrorRateSpike, Channel: "log", Threshold: 5})
	require.NoError(t, err)
	_ = a
	rep = svc.Health("ta") // 再次检查触发告警
	require.Equal(t, "yellow", rep.Status)
	var events []usage.UsageAlertEvent
	require.NoError(t, db.Where("rule=?", usage.RuleErrorRateSpike).Find(&events).Error)
	require.NotEmpty(t, events)
}

func TestUsageTenantIsolation(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	day := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&usage.UsageRecord{TenantID: "ta", Kind: usage.KindMessage, CreatedAt: day}).Error)
	require.NoError(t, db.Create(&usage.UsageRecord{TenantID: "tb", Kind: usage.KindMessage, CreatedAt: day}).Error)

	start, end := day, day.AddDate(0, 0, 1)
	sa, err := svc.Summary("ta", start, end)
	require.NoError(t, err)
	sb, err := svc.Summary("tb", start, end)
	require.NoError(t, err)
	require.Len(t, sa.Kinds, 1)
	require.Len(t, sb.Kinds, 1)
	// 各自只见自己：两者 calls 都是 1，但数据行互不渗透。
	var taRows, tbRows int64
	db.Model(&usage.UsageRecord{}).Where("tenant_id='ta'").Count(&taRows)
	db.Model(&usage.UsageRecord{}).Where("tenant_id='tb'").Count(&tbRows)
	require.Equal(t, int64(1), taRows)
	require.Equal(t, int64(1), tbRows)

	// 预算也按租户隔离。
	ba, err := svc.UpsertBudget("ta", BudgetInput{Kind: usage.BudgetKindModelCall, LimitValue: 5, Period: usage.PeriodDaily})
	require.NoError(t, err)
	viewsA, err := svc.Budgets("ta", time.Now())
	require.NoError(t, err)
	viewsB, err := svc.Budgets("tb", time.Now())
	require.NoError(t, err)
	require.Len(t, viewsA, 1)
	require.Len(t, viewsB, 0)
	require.Equal(t, ba.TenantID, "ta")
}

func TestUsageStorageFallbackOnSQLite(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	// usage_records 存在 → 有行；不存在的表被跳过。
	require.NoError(t, db.Create(&usage.UsageRecord{TenantID: "ta", Kind: usage.KindMessage}).Error)
	rows, err := svc.Storage("ta")
	require.NoError(t, err)
	names := map[string]bool{}
	for _, r := range rows {
		names[r.TableName] = true
	}
	require.True(t, names["usage_records"])
	require.False(t, names["nope_missing_table"])
}

func TestUsageNormalizeRange(t *testing.T) {
	s, e, err := NormalizeRange("2026-09-01", "2026-09-10")
	require.NoError(t, err)
	require.Equal(t, "2026-09-01", s.Format("2006-01-02"))
	require.Equal(t, "2026-09-11", e.Format("2006-01-02"))
	_, _, err = NormalizeRange("bad", "")
	require.Error(t, err)
	_, _, err = NormalizeRange("2026-09-10", "2026-09-01")
	require.Error(t, err)
}

func TestUsageAlertValidationAndEvents(t *testing.T) {
	db := usageTestDB(t)
	svc := newUsageSvc(t, db)
	_, err := svc.UpsertAlert("ta", AlertInput{Rule: "nope", Channel: "log"})
	require.Error(t, err)
	_, err = svc.UpsertAlert("ta", AlertInput{Rule: usage.RuleBudgetPct, Channel: "webhook"})
	require.Error(t, err) // webhook 缺 URL
	a, err := svc.UpsertAlert("ta", AlertInput{Rule: usage.RuleBudgetPct, Channel: "webhook", WebhookURL: "http://127.0.0.1:1/hook", Threshold: 80})
	require.NoError(t, err)
	events, err := svc.AlertEvents("ta", 10)
	require.NoError(t, err)
	require.Empty(t, events)
	require.NoError(t, svc.DeleteAlert("ta", a.ID))
}
