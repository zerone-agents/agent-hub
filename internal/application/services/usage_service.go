// UsageService 是 H7.5 用量与运维的应用服务：埋点采集、实时聚合统计、
// 软预算、单价换算、系统健康合成、CSV 导出与基础告警。
//
// 采集路径的硬约束（D-H7.5-1）：埋点零阻断、零 panic 传播。Record 是
// 纯内存操作（价格换算 + 预算预检），随后非阻塞投递进 buffered channel
// （容量 100，满了丢弃并计数）；后台单 goroutine 批量落库，库错误仅
// 记日志。任何一步 panic 都被 recover，绝不影响调用方。
//
// 聚合决策（D-H7.5-2）：不设 usage_daily 预聚合表，统计查询对
// usage_records 实时 SUM / GROUP BY。预期万级行/天，实时聚合足够快；
// 量级上去后再引入按天物化（接口形状不变）。
package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/systemsetting"
	"control-panel/internal/domain/usage"

	"gorm.io/gorm"
)

// ModelPricingKey 是 system_settings 中模型单价配置的键（D-H7.5-3）。
const ModelPricingKey = "model_pricing"

// usageQueueCapacity 是埋点 channel 容量：满了丢弃并计数（不反压主流程）。
const usageQueueCapacity = 100

// usageBatchSize / usageBatchIdle 控制后台落库的批大小与最大等待。
const (
	usageBatchSize = 50
	usageBatchIdle = 200 * time.Millisecond
)

// pricingCacheTTL 是单价配置的缓存时长（管理端改价后一分钟内生效）。
const pricingCacheTTL = time.Minute

// UsageRecordInput 是埋点入参：埋点处给什么算什么，全部可选。
type UsageRecordInput struct {
	Kind          string
	TenantID      string
	AgentID       *uint64
	RunID         string
	ExtensionName string
	Model         string
	TokensIn      *int64
	TokensOut     *int64
	Bytes         *int64
	LatencyMs     *int64
	Error         string
	CreatedAt     time.Time
}

// KindSummary 是一个 kind 的汇总。
type KindSummary struct {
	Kind         string  `json:"kind"`
	TotalCalls   int64   `json:"totalCalls"`
	TotalTokens  int64   `json:"totalTokens"`
	TotalCost    int64   `json:"totalCostMicros"`
	TotalErrors  int64   `json:"totalErrors"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	DeniedCount  int64   `json:"deniedCount"`
}

// DailyTrend 是一天的 kind 汇总（趋势数组元素）。
type DailyTrend struct {
	Date   string `json:"date"`
	Kind   string `json:"kind"`
	Calls  int64  `json:"calls"`
	Tokens int64  `json:"tokens"`
	Cost   int64  `json:"costMicros"`
	Errors int64  `json:"errors"`
}

// UsageSummary 是 /summary 的响应。
type UsageSummary struct {
	From   string        `json:"from"`
	To     string        `json:"to"`
	Kinds  []KindSummary `json:"kinds"`
	Trends []DailyTrend  `json:"trends"`
}

// DimensionRow 是扩展 / Agent / Run / 模型维度的排行行。
type DimensionRow struct {
	Key         string  `json:"key"` // extension_name / agent_id / run_id / model
	TotalCalls  int64   `json:"totalCalls"`
	TotalTokens int64   `json:"totalTokens"`
	TotalCost   int64   `json:"totalCostMicros"`
	TotalErrors int64   `json:"totalErrors"`
	AvgLatency  float64 `json:"avgLatencyMs"`
}

// DimensionPage 是分页响应。
type DimensionPage struct {
	Items      []DimensionRow `json:"items"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	TotalPages int            `json:"totalPages"`
}

// ErrorRow 是错误聚合行。
type ErrorRow struct {
	Kind   string    `json:"kind"`
	Error  string    `json:"error"`
	Count  int64     `json:"count"`
	LastAt time.Time `json:"lastAt"`
}

// BudgetView 是预算 + 当前周期用量进度。
type BudgetView struct {
	usage.UsageBudget
	UsedValue int64   `json:"usedValue"`
	UsedPct   float64 `json:"usedPct"` // 0-100+，超 100 为超限
}

// StorageRow 是一张表的行数与字节估算（information_schema）。
type StorageRow struct {
	TableName      string `json:"tableName"`
	RowEstimate    int64  `json:"rowEstimate"`
	DataLengthByte int64  `json:"dataLengthBytes"`
}

// HealthReport 是 /health 的合成状态。
type HealthReport struct {
	Status          string    `json:"status"` // green / yellow / red
	DB              string    `json:"db"`
	QueueBacklog    int       `json:"queueBacklog"`
	QueueDropped    int64     `json:"queueDropped"`
	TodayErrorRate  float64   `json:"todayErrorRatePct"`
	ExtensionsTotal int64     `json:"extensionsTotal"`
	ExtensionsOn    int64     `json:"extensionsOn"`
	ExtensionsOff   int64     `json:"extensionsOff"`
	UptimeSec       int64     `json:"uptimeSec"`
	CheckedAt       time.Time `json:"checkedAt"`
}

// modelPrice 是单个模型的单价（每百万 token 的分计价）。
type modelPrice struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
}

// UsageService 采集并查询用量数据。NewUsageService 后即开始后台落库
// goroutine；进程退出前可调 Close 优雅停止（测试必调）。
type UsageService struct {
	db        *gorm.DB
	startedAt time.Time

	queue chan *usage.UsageRecord
	stop  chan struct{}
	done  chan struct{}

	dropped   atomic.Int64
	workerErr atomic.Int64

	pricingMu sync.Mutex
	pricing   map[string]modelPrice
	pricingAt time.Time
}

// NewUsageService 构造服务并启动后台落库 goroutine。
func NewUsageService(db *gorm.DB) *UsageService {
	s := &UsageService{
		db:        db,
		startedAt: time.Now(),
		queue:     make(chan *usage.UsageRecord, usageQueueCapacity),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go s.worker()
	return s
}

// Close 停止后台 goroutine 并等待收尾。幂等。
func (s *UsageService) Close() {
	select {
	case <-s.done:
		return
	default:
	}
	close(s.stop)
	<-s.done
}

// QueueStats 返回采集队列状态（health 用）。
func (s *UsageService) QueueStats() (backlog int, dropped int64) {
	return len(s.queue), s.dropped.Load()
}

// DroppedCount 返回因队列满被丢弃的埋点总数。
func (s *UsageService) DroppedCount() int64 { return s.dropped.Load() }

// Record 是埋点唯一入口。语义：永不 panic、永不阻塞（队列满丢弃计数）、
// 永不返回错误。价格换算与软预算预检在此同步完成；落库异步。
func (s *UsageService) Record(in UsageRecordInput) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[usage] record panic recovered: %v", r)
		}
	}()
	if s == nil || s.db == nil || in.TenantID == "" || in.Kind == "" {
		return
	}
	rec := &usage.UsageRecord{
		TenantID:      in.TenantID,
		Kind:          in.Kind,
		AgentID:       in.AgentID,
		RunID:         in.RunID,
		ExtensionName: in.ExtensionName,
		Model:         in.Model,
		TokensIn:      in.TokensIn,
		TokensOut:     in.TokensOut,
		Bytes:         in.Bytes,
		LatencyMs:     in.LatencyMs,
		Error:         in.Error,
		CreatedAt:     in.CreatedAt,
	}
	if rec.CreatedAt.IsZero() {
		// 必须落 UTC：所有统计窗口（periodRange / NormalizeRange）都是 UTC 边界，
		// 而 SQLite 按字符串比较带偏移的时间戳；写本地时间会让 UTC+8 服务器上
		// 的记录整天落在日/月窗口之外，导致用量与预算少算。
		rec.CreatedAt = time.Now().UTC()
	}
	if in.TokensIn != nil || in.TokensOut != nil {
		rec.CostMicros = s.costMicros(in.Model, in.TokensIn, in.TokensOut)
	}
	// 软预算预检：超限只标记 denied + 触发告警，不阻断调用方（D-H7.5-4）。
	s.checkBudget(rec)
	select {
	case s.queue <- rec:
	default:
		s.dropped.Add(1)
	}
}

// costMicros 按 model_pricing 换算成本（微分）。无配置返回 nil（未知）。
func (s *UsageService) costMicros(model string, tokensIn, tokensOut *int64) *int64 {
	if model == "" {
		return nil
	}
	p, ok := s.pricingMap()[model]
	if !ok {
		return nil
	}
	var in, out float64
	if tokensIn != nil {
		in = float64(*tokensIn)
	}
	if tokensOut != nil {
		out = float64(*tokensOut)
	}
	// 单价为「分/百万 token」，1 分 = 100 微分：先归一到百万 token 再换算。
	micros := int64(in/1e6*p.InputPerMillion*100 + out/1e6*p.OutputPerMillion*100)
	return &micros
}

// pricingMap 读 model_pricing 配置（60s 缓存）。读失败视为无配置。
func (s *UsageService) pricingMap() map[string]modelPrice {
	now := time.Now()
	s.pricingMu.Lock()
	defer s.pricingMu.Unlock()
	if s.pricing != nil && now.Sub(s.pricingAt) < pricingCacheTTL {
		return s.pricing
	}
	pricing := map[string]modelPrice{}
	var row systemsetting.SystemSetting
	err := s.db.Where("setting_key = ?", ModelPricingKey).First(&row).Error
	if err == nil && strings.TrimSpace(row.Value) != "" {
		if err := json.Unmarshal([]byte(row.Value), &pricing); err != nil {
			log.Printf("[usage] model_pricing invalid JSON, ignored: %v", err)
			pricing = map[string]modelPrice{}
		}
	}
	s.pricing, s.pricingAt = pricing, now
	return pricing
}

// InvalidatePricingCache 使单价缓存立即失效（管理端改价后调用）。
func (s *UsageService) InvalidatePricingCache() {
	s.pricingMu.Lock()
	s.pricing, s.pricingAt = nil, time.Time{}
	s.pricingMu.Unlock()
}

// worker 后台落库 goroutine：批量插入 + recover。库错误仅计数记日志。
func (s *UsageService) worker() {
	defer close(s.done)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[usage] worker panic recovered: %v", r)
		}
	}()
	batch := make([]*usage.UsageRecord, 0, usageBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := s.db.CreateInBatches(batch, 50).Error; err != nil {
			s.workerErr.Add(int64(len(batch)))
			log.Printf("[usage] persist batch failed (dropped %d): %v", len(batch), err)
		}
		batch = batch[:0]
	}
	timer := time.NewTimer(usageBatchIdle)
	defer timer.Stop()
	for {
		select {
		case rec := <-s.queue:
			batch = append(batch, rec)
			if len(batch) >= usageBatchSize {
				flush()
			}
		case <-timer.C:
			flush()
			timer.Reset(usageBatchIdle)
		case <-s.stop:
			// 收尾：排空队列后退出（尽力而为，单次限时）。
			drain := time.After(2 * time.Second)
			for len(batch) < usageBatchSize {
				select {
				case rec := <-s.queue:
					batch = append(batch, rec)
				case <-drain:
					flush()
					return
				default:
					flush()
					return
				}
			}
			flush()
			return
		}
	}
}

// checkBudget 软预算预检：同 tenant+kind+period 的当期累计超 limit 时
// 标记 denied（记录仍写入），并按阈值触发 budget_pct 告警。库错误仅
// 记日志——预算检查失败绝不影响埋点。
func (s *UsageService) checkBudget(rec *usage.UsageRecord) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[usage] budget check panic recovered: %v", r)
		}
	}()
	kind := budgetKindFor(rec.Kind)
	if kind == "" {
		return
	}
	var budgets []usage.UsageBudget
	if err := s.db.Where("tenant_id=? AND kind=?", rec.TenantID, kind).Find(&budgets).Error; err != nil {
		log.Printf("[usage] load budgets failed: %v", err)
		return
	}
	if len(budgets) == 0 {
		return
	}
	// budget_pct 告警规则（可选）：未配置时阈值命中仍写事件（log 通道）。
	var alert *usage.UsageAlert
	var alertRow usage.UsageAlert
	if err := s.db.Where("tenant_id=? AND rule=?", rec.TenantID, usage.RuleBudgetPct).First(&alertRow).Error; err == nil {
		alert = &alertRow
	}
	for _, b := range budgets {
		start, end := periodRange(rec.CreatedAt, b.Period)
		used := s.periodUsage(rec.TenantID, kind, start, end)
		rec.Denied = rec.Denied || used >= b.LimitValue
		pct := float64(0)
		if b.LimitValue > 0 {
			pct = float64(used) * 100 / float64(b.LimitValue)
		}
		if b.AlertThresholdPct > 0 && pct >= float64(b.AlertThresholdPct) {
			s.fireAlert(rec.TenantID, alert, usage.RuleBudgetPct, fmt.Sprintf(
				"预算用量 %s/%s 已达 %.1f%%（%d/%d）", b.Kind, b.Period, pct, used, b.LimitValue))
		}
	}
}

// budgetKindFor 把用量 kind 归一到预算 kind；无对应预算返回 ""。
func budgetKindFor(kind string) string {
	switch kind {
	case usage.KindModelCall, usage.KindToken:
		return usage.BudgetKindModelCall
	case usage.KindExtensionCall:
		return usage.BudgetKindExtensionCall
	case usage.KindStorage:
		return usage.BudgetKindStorage
	default:
		return ""
	}
}

// periodRange 返回 t 所在 daily / monthly 周期的 [start, end)。
func periodRange(t time.Time, period string) (time.Time, time.Time) {
	t = t.UTC()
	if period == usage.PeriodMonthly {
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 1)
}

// periodUsage 查询某周期内某 kind 的累计值（调用数 / 字节）。
func (s *UsageService) periodUsage(tenantID, kind string, start, end time.Time) int64 {
	column := "COUNT(*)"
	if kind == usage.BudgetKindStorage {
		column = "COALESCE(SUM(bytes), 0)"
	}
	var value int64
	err := s.db.Model(&usage.UsageRecord{}).
		Where("tenant_id=? AND kind=? AND created_at>=? AND created_at<?", tenantID, kind, start, end).
		Select(column).Scan(&value).Error
	if err != nil {
		log.Printf("[usage] period usage failed: %v", err)
		return 0
	}
	return value
}

// fireAlert 触发告警：写事件表（fire-and-forget）+ webhook POST（3s 超时）。
// 同规则 10 分钟内不重复触发（看 alert 行的 LastFiredAt）。预算阈值
// 命中但租户未配置 budget_pct 规则时 alert 为 nil——事件照常落库
// （AlertID=0），只走 log 通道。
func (s *UsageService) fireAlert(tenantID string, alert *usage.UsageAlert, ruleName, message string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[usage] fire alert panic recovered: %v", r)
		}
	}()
	// 同 Record：持久化时间戳统一 UTC，避免与 last_fired_at 的节流窗口比较错位。
	now := time.Now().UTC()
	var alertID uint64
	channel, webhookURL := "log", ""
	if alert != nil {
		if alert.LastFiredAt != nil && now.Sub(*alert.LastFiredAt) < 10*time.Minute {
			return
		}
		if err := s.db.Model(&usage.UsageAlert{}).Where("id=? AND (last_fired_at IS NULL OR last_fired_at<?)",
			alert.ID, now.Add(-10*time.Minute)).Update("last_fired_at", now).Error; err != nil {
			log.Printf("[usage] update last_fired_at failed: %v", err)
			return
		}
		alertID, channel, webhookURL = alert.ID, alert.Channel, alert.WebhookURL
	}
	event := usage.UsageAlertEvent{TenantID: tenantID, AlertID: alertID, Rule: ruleName,
		Message: message, WebhookURL: webhookURL, CreatedAt: now}
	if err := s.db.Create(&event).Error; err != nil {
		log.Printf("[usage] insert alert event failed: %v", err)
	}
	if channel == "webhook" && webhookURL != "" {
		go postWebhook(webhookURL, map[string]any{
			"rule": ruleName, "message": message, "tenantId": tenantID, "firedAt": now.UTC(),
		})
	} else {
		log.Printf("[usage][alert] tenant=%s rule=%s %s", tenantID, ruleName, message)
	}
}

// postWebhook 告警 webhook：3s 超时，失败仅记日志。
func postWebhook(url string, payload map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[usage] webhook panic recovered: %v", r)
		}
	}()
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[usage] webhook post failed: %v", err)
		return
	}
	_ = resp.Body.Close()
}

// ---------- 统计查询（实时 SUM group by，D-H7.5-2） ----------

// NormalizeRange 解析并规范化 from/to（YYYY-MM-DD）。空值默认近 7 天。
func NormalizeRange(from, to string) (start, end time.Time, err error) {
	now := time.Now().UTC()
	end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	start = end.AddDate(0, 0, -7)
	if strings.TrimSpace(from) != "" {
		start, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(from), time.UTC)
		if err != nil {
			return start, end, fmt.Errorf("from 必须是 YYYY-MM-DD")
		}
	}
	if strings.TrimSpace(to) != "" {
		toT, perr := time.ParseInLocation("2006-01-02", strings.TrimSpace(to), time.UTC)
		if perr != nil {
			return start, end, fmt.Errorf("to 必须是 YYYY-MM-DD")
		}
		end = toT.AddDate(0, 0, 1)
	}
	if !end.After(start) {
		return start, end, fmt.Errorf("from 必须早于 to")
	}
	return start, end, nil
}

func dayString(t time.Time) string { return t.UTC().Format("2006-01-02") }

// Summary 按 kind 汇总 + 按天趋势。
func (s *UsageService) Summary(tenantID string, start, end time.Time) (*UsageSummary, error) {
	var kinds []KindSummary
	err := s.db.Model(&usage.UsageRecord{}).
		Select(`kind,
			COUNT(*) AS total_calls,
			COALESCE(SUM(COALESCE(tokens_in,0)+COALESCE(tokens_out,0)),0) AS total_tokens,
			COALESCE(SUM(COALESCE(cost_micros,0)),0) AS total_cost,
			COALESCE(SUM(CASE WHEN error<>'' THEN 1 ELSE 0 END),0) AS total_errors,
			COALESCE(AVG(latency_ms),0) AS avg_latency_ms,
			COALESCE(SUM(CASE WHEN denied THEN 1 ELSE 0 END),0) AS denied_count`).
		Where("tenant_id=? AND created_at>=? AND created_at<?", tenantID, start, end).
		Group("kind").Order("kind").Scan(&kinds).Error
	if err != nil {
		return nil, err
	}
	var trends []DailyTrend
	err = s.db.Model(&usage.UsageRecord{}).
		Select(`DATE(created_at) AS date, kind,
			COUNT(*) AS calls,
			COALESCE(SUM(COALESCE(tokens_in,0)+COALESCE(tokens_out,0)),0) AS tokens,
			COALESCE(SUM(COALESCE(cost_micros,0)),0) AS cost,
			COALESCE(SUM(CASE WHEN error<>'' THEN 1 ELSE 0 END),0) AS errors`).
		Where("tenant_id=? AND created_at>=? AND created_at<?", tenantID, start, end).
		Group("DATE(created_at), kind").Order("date, kind").Scan(&trends).Error
	if err != nil {
		return nil, err
	}
	for i := range trends {
		if t, perr := time.ParseInLocation("2006-01-02", fmt.Sprint(trends[i].Date), time.UTC); perr == nil {
			trends[i].Date = dayString(t)
		}
	}
	return &UsageSummary{From: dayString(start), To: dayString(end.AddDate(0, 0, -1)), Kinds: kinds, Trends: trends}, nil
}

// dimension 是 by-extension / by-agent / by-run / by-model 的共用实现。
func (s *UsageService) dimension(tenantID, column string, start, end time.Time, page, pageSize int) (*DimensionPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	where := "tenant_id=? AND created_at>=? AND created_at<?"
	args := []any{tenantID, start, end}
	if column == "model" {
		where += " AND model<>''"
	} else if column == "extension_name" {
		where += " AND extension_name<>''"
	} else if column == "run_id" {
		where += " AND run_id<>''"
	} else if column == "agent_id" {
		where += " AND agent_id IS NOT NULL"
	}
	var total int64
	if err := s.db.Model(&usage.UsageRecord{}).Where(where, args...).
		Distinct(column).Count(&total).Error; err != nil {
		return nil, err
	}
	var items []DimensionRow
	err := s.db.Model(&usage.UsageRecord{}).
		Select(column+" AS `key`, COUNT(*) AS total_calls, "+
			"COALESCE(SUM(COALESCE(tokens_in,0)+COALESCE(tokens_out,0)),0) AS total_tokens, "+
			"COALESCE(SUM(COALESCE(cost_micros,0)),0) AS total_cost, "+
			"COALESCE(SUM(CASE WHEN error<>'' THEN 1 ELSE 0 END),0) AS total_errors, "+
			"COALESCE(AVG(latency_ms),0) AS avg_latency").
		Where(where, args...).
		Group("`key`").Order("total_calls DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&items).Error
	if err != nil {
		return nil, err
	}
	return &DimensionPage{Items: items, Total: total, Page: page, PageSize: pageSize,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize))}, nil
}

// ByExtension 扩展维度排行。
func (s *UsageService) ByExtension(tenantID string, start, end time.Time, page, pageSize int) (*DimensionPage, error) {
	return s.dimension(tenantID, "extension_name", start, end, page, pageSize)
}

// ByAgent Agent 维度排行。
func (s *UsageService) ByAgent(tenantID string, start, end time.Time, page, pageSize int) (*DimensionPage, error) {
	return s.dimension(tenantID, "agent_id", start, end, page, pageSize)
}

// ByRun Run 维度排行。
func (s *UsageService) ByRun(tenantID string, start, end time.Time, page, pageSize int) (*DimensionPage, error) {
	return s.dimension(tenantID, "run_id", start, end, page, pageSize)
}

// ByModel 模型维度排行。
func (s *UsageService) ByModel(tenantID string, start, end time.Time, page, pageSize int) (*DimensionPage, error) {
	return s.dimension(tenantID, "model", start, end, page, pageSize)
}

// Errors 错误聚合列表（kind + error 分组）。MAX(created_at) 在部分驱动下
// 以字符串返回，统一解析为 time.Time。
func (s *UsageService) Errors(tenantID string, start, end time.Time) ([]ErrorRow, error) {
	var raw []struct {
		Kind   string
		Error  string
		Count  int64
		LastAt string
	}
	err := s.db.Model(&usage.UsageRecord{}).
		Select("kind, error, COUNT(*) AS count, MAX(created_at) AS last_at").
		Where("tenant_id=? AND error<>'' AND created_at>=? AND created_at<?", tenantID, start, end).
		Group("kind, error").Order("count DESC").Limit(200).Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	rows := make([]ErrorRow, 0, len(raw))
	for _, r := range raw {
		lastAt, _ := time.ParseInLocation("2006-01-02 15:04:05", r.LastAt, time.UTC)
		if lastAt.IsZero() {
			lastAt, _ = time.Parse(time.RFC3339, r.LastAt)
		}
		rows = append(rows, ErrorRow{Kind: r.Kind, Error: r.Error, Count: r.Count, LastAt: lastAt})
	}
	return rows, err
}

// ExportRows 流式导出明细（CSV 用）：按主键升序，回调返回 false 中止。
func (s *UsageService) ExportRows(tenantID string, start, end time.Time, kind string, fn func(*usage.UsageRecord) bool) error {
	q := s.db.Model(&usage.UsageRecord{}).Where("tenant_id=? AND created_at>=? AND created_at<?", tenantID, start, end)
	if strings.TrimSpace(kind) != "" {
		q = q.Where("kind = ?", kind)
	}
	rows, err := q.Order("id").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rec usage.UsageRecord
		if err := s.db.ScanRows(rows, &rec); err != nil {
			return err
		}
		if !fn(&rec) {
			return nil
		}
	}
	return rows.Err()
}

// ExportCSV 把明细写成 CSV（含表头）。
func (s *UsageService) ExportCSV(w io.Writer, tenantID string, start, end time.Time, kind string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"id", "kind", "agent_id", "run_id", "extension_name", "model",
		"tokens_in", "tokens_out", "cost_micros", "bytes", "latency_ms", "error", "denied", "created_at"}); err != nil {
		return err
	}
	err := s.ExportRows(tenantID, start, end, kind, func(rec *usage.UsageRecord) bool {
		row := []string{strconv.FormatUint(rec.ID, 10), rec.Kind, "", rec.RunID, rec.ExtensionName, rec.Model,
			nullableInt(rec.TokensIn), nullableInt(rec.TokensOut), nullableInt(rec.CostMicros),
			nullableInt(rec.Bytes), nullableInt(rec.LatencyMs), rec.Error,
			strconv.FormatBool(rec.Denied), rec.CreatedAt.UTC().Format(time.RFC3339)}
		if rec.AgentID != nil {
			row[2] = strconv.FormatUint(*rec.AgentID, 10)
		}
		return cw.Write(row) == nil
	})
	cw.Flush()
	return err
}

func nullableInt(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

// ---------- 预算 CRUD + 进度 ----------

// UpsertBudget 创建或更新预算（同 tenant+kind+period 唯一）。
func (s *UsageService) UpsertBudget(tenantID string, in BudgetInput) (*usage.UsageBudget, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	var b usage.UsageBudget
	err := s.db.Where("tenant_id=? AND kind=? AND period=?", tenantID, in.Kind, in.Period).
		First(&b).Error
	if err == gorm.ErrRecordNotFound {
		b = usage.UsageBudget{TenantID: tenantID, Kind: in.Kind, Period: in.Period}
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	b.LimitValue, b.AlertThresholdPct = in.LimitValue, in.AlertThresholdPct
	if b.ID == 0 {
		err = s.db.Create(&b).Error
	} else {
		err = s.db.Save(&b).Error
	}
	return &b, err
}

// DeleteBudget 删除预算；不存在返回 gorm.ErrRecordNotFound。
func (s *UsageService) DeleteBudget(tenantID string, id uint64) error {
	res := s.db.Where("tenant_id=?", tenantID).Delete(&usage.UsageBudget{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BudgetInput 是预算创建/更新入参。
type BudgetInput struct {
	Kind              string `json:"kind"`
	LimitValue        int64  `json:"limitValue"`
	Period            string `json:"period"`
	AlertThresholdPct int    `json:"alertThresholdPct"`
}

// Validate 校验预算入参（中文错误）。
func (in BudgetInput) Validate() error {
	switch in.Kind {
	case usage.BudgetKindModelCall, usage.BudgetKindExtensionCall, usage.BudgetKindStorage:
	default:
		return fmt.Errorf("kind 必须是 %s/%s/%s", usage.BudgetKindModelCall, usage.BudgetKindExtensionCall, usage.BudgetKindStorage)
	}
	switch in.Period {
	case usage.PeriodDaily, usage.PeriodMonthly:
	default:
		return fmt.Errorf("period 必须是 %s/%s", usage.PeriodDaily, usage.PeriodMonthly)
	}
	if in.LimitValue <= 0 {
		return fmt.Errorf("limitValue 必须为正整数")
	}
	if in.AlertThresholdPct < 0 || in.AlertThresholdPct > 100 {
		return fmt.Errorf("alertThresholdPct 必须在 0-100 之间")
	}
	return nil
}

// Budgets 列出租户全部预算 + 当前周期用量进度。
func (s *UsageService) Budgets(tenantID string, now time.Time) ([]BudgetView, error) {
	var budgets []usage.UsageBudget
	if err := s.db.Where("tenant_id=?", tenantID).Order("kind, period").Find(&budgets).Error; err != nil {
		return nil, err
	}
	views := make([]BudgetView, 0, len(budgets))
	for _, b := range budgets {
		start, end := periodRange(now, b.Period)
		used := s.periodUsage(tenantID, b.Kind, start, end)
		pct := float64(0)
		if b.LimitValue > 0 {
			pct = float64(used) * 100 / float64(b.LimitValue)
		}
		views = append(views, BudgetView{UsageBudget: b, UsedValue: used, UsedPct: pct})
	}
	return views, nil
}

// ---------- 告警规则 CRUD + 触发记录 ----------

// UpsertAlert 创建或更新告警规则（同 tenant+rule 唯一，webhook 覆盖更新）。
func (s *UsageService) UpsertAlert(tenantID string, in AlertInput) (*usage.UsageAlert, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	var a usage.UsageAlert
	err := s.db.Where("tenant_id=? AND rule=?", tenantID, in.Rule).First(&a).Error
	if err == gorm.ErrRecordNotFound {
		a = usage.UsageAlert{TenantID: tenantID, Rule: in.Rule}
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	a.Channel, a.WebhookURL, a.Threshold = in.Channel, in.WebhookURL, in.Threshold
	if a.ID == 0 {
		err = s.db.Create(&a).Error
	} else {
		err = s.db.Save(&a).Error
	}
	return &a, err
}

// DeleteAlert 删除告警规则。
func (s *UsageService) DeleteAlert(tenantID string, id uint64) error {
	res := s.db.Where("tenant_id=?", tenantID).Delete(&usage.UsageAlert{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Alerts 列出租户全部告警规则。
func (s *UsageService) Alerts(tenantID string) ([]usage.UsageAlert, error) {
	var rows []usage.UsageAlert
	return rows, s.db.Where("tenant_id=?", tenantID).Order("rule").Find(&rows).Error
}

// AlertEvents 列出告警触发记录（倒序，limit 封顶 200）。
func (s *UsageService) AlertEvents(tenantID string, limit int) ([]usage.UsageAlertEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var rows []usage.UsageAlertEvent
	return rows, s.db.Where("tenant_id=?", tenantID).Order("id DESC").Limit(limit).Find(&rows).Error
}

// AlertInput 是告警规则入参。
type AlertInput struct {
	Rule       string  `json:"rule"`
	Channel    string  `json:"channel"`
	WebhookURL string  `json:"webhookUrl"`
	Threshold  float64 `json:"threshold"`
}

// Validate 校验告警规则入参（中文错误）。
func (in AlertInput) Validate() error {
	switch in.Rule {
	case usage.RuleBudgetPct, usage.RuleErrorRateSpike, usage.RuleHealthRed:
	default:
		return fmt.Errorf("rule 必须是 %s/%s/%s", usage.RuleBudgetPct, usage.RuleErrorRateSpike, usage.RuleHealthRed)
	}
	switch in.Channel {
	case "log", "webhook":
	default:
		return fmt.Errorf("channel 必须是 log 或 webhook")
	}
	if in.Channel == "webhook" && strings.TrimSpace(in.WebhookURL) == "" {
		return fmt.Errorf("channel=webhook 时 webhookUrl 必填")
	}
	return nil
}

// ---------- 存储与健康 ----------

// Storage 返回 usage_records 等核心表的行数与字节估算。SQLite（测试）
// 没有 information_schema，退化为逐表 COUNT(*) 与 0 字节。
func (s *UsageService) Storage(tenantID string) ([]StorageRow, error) {
	tables := []string{"usage_records", "usage_budgets", "usage_alerts", "usage_alert_events",
		"chat_messages", "agent_messages", "workflow_executions", "workflow_step_runs", "run_activities"}
	if s.db.Dialector.Name() == "sqlite" {
		return s.storageByCount(tables)
	}
	var rows []StorageRow
	err := s.db.Table("information_schema.tables").
		Select("table_name, table_rows AS row_estimate, data_length AS data_length_byte").
		Where("table_schema = DATABASE() AND table_name IN ?", tables).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TableName < rows[j].TableName })
	return rows, nil
}

func (s *UsageService) storageByCount(tables []string) ([]StorageRow, error) {
	rows := make([]StorageRow, 0, len(tables))
	for _, t := range tables {
		var row StorageRow
		var count int64
		err := s.db.Table(t).Count(&count).Error
		if err != nil {
			// 表不存在（该部署未启用对应模块）：跳过而非失败。
			continue
		}
		row.TableName, row.RowEstimate = t, count
		rows = append(rows, row)
	}
	return rows, nil
}

// Health 合成系统健康：DB 连通、队列积压、今日错误率、扩展启停计数、
// uptime。判定（D-H7.5-5）：DB 不通或今日错误率 ≥20% → red；
// 队列积压 ≥80 或错误率 ≥5% 或扩展全部停用 → yellow；其余 green。
// error_rate_spike / health_red 告警在此检查。
func (s *UsageService) Health(tenantID string) *HealthReport {
	rep := &HealthReport{Status: "green", CheckedAt: time.Now()}
	// DB 连通
	sqlDB, err := s.db.DB()
	if err != nil || sqlDB.Ping() != nil {
		rep.Status, rep.DB = "red", "unhealthy"
	} else {
		rep.DB = "ok"
	}
	// 队列
	backlog, dropped := s.QueueStats()
	rep.QueueBacklog, rep.QueueDropped = backlog, dropped
	// 今日错误率
	now := time.Now()
	start, end := periodRange(now, usage.PeriodDaily)
	var calls, errs int64
	s.db.Model(&usage.UsageRecord{}).
		Where("tenant_id=? AND created_at>=? AND created_at<?", tenantID, start, end).
		Count(&calls)
	s.db.Model(&usage.UsageRecord{}).
		Where("tenant_id=? AND error<>'' AND created_at>=? AND created_at<?", tenantID, start, end).
		Count(&errs)
	if calls > 0 {
		rep.TodayErrorRate = float64(errs) * 100 / float64(calls)
	}
	// 扩展启停
	s.db.Model(&extension.Extension{}).Where("tenant_id=?", tenantID).Count(&rep.ExtensionsTotal)
	s.db.Model(&extension.Extension{}).Where("tenant_id=? AND status=?", tenantID, extension.StatusActive).Count(&rep.ExtensionsOn)
	rep.ExtensionsOff = rep.ExtensionsTotal - rep.ExtensionsOn
	// uptime
	rep.UptimeSec = int64(now.Sub(s.startedAt).Seconds())
	// 合成判定
	switch {
	case rep.DB != "ok" || rep.TodayErrorRate >= 20:
		rep.Status = "red"
	case backlog >= 80 || rep.TodayErrorRate >= 5 || (rep.ExtensionsTotal > 0 && rep.ExtensionsOn == 0):
		rep.Status = "yellow"
	}
	// 告警检查（软实现，失败忽略）
	s.checkHealthAlerts(tenantID, rep)
	return rep
}

// checkHealthAlerts 触发 error_rate_spike / health_red 告警（软实现）。
func (s *UsageService) checkHealthAlerts(tenantID string, rep *HealthReport) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[usage] health alert panic recovered: %v", r)
		}
	}()
	var alerts []usage.UsageAlert
	if err := s.db.Where("tenant_id=? AND rule IN ?", tenantID,
		[]string{usage.RuleErrorRateSpike, usage.RuleHealthRed}).Find(&alerts).Error; err != nil {
		return
	}
	for i := range alerts {
		a := &alerts[i]
		switch a.Rule {
		case usage.RuleErrorRateSpike:
			if rep.TodayErrorRate >= a.Threshold {
				s.fireAlert(tenantID, a, a.Rule, fmt.Sprintf("今日错误率 %.1f%% 超过阈值 %.0f%%", rep.TodayErrorRate, a.Threshold))
			}
		case usage.RuleHealthRed:
			if rep.Status == "red" {
				s.fireAlert(tenantID, a, a.Rule, "系统健康状态为 red")
			}
		}
	}
}

// GetPricingRaw 返回 model_pricing 原始 JSON（未配置返回 ""）。
func (s *UsageService) GetPricingRaw() (string, error) {
	var row systemsetting.SystemSetting
	err := s.db.Where("setting_key = ?", ModelPricingKey).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return row.Value, nil
}

// SetPricingRaw 写入 model_pricing 原始 JSON：必须是合法 JSON object，
// 写后立即使缓存失效。空串视为删除配置。
func (s *UsageService) SetPricingRaw(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		var probe map[string]modelPrice
		if err := json.Unmarshal([]byte(raw), &probe); err != nil {
			return fmt.Errorf("value 必须是合法 JSON：%w", err)
		}
	}
	err := s.db.Where("setting_key = ?", ModelPricingKey).
		Assign(systemsetting.SystemSetting{Key: ModelPricingKey, Value: raw}).
		FirstOrCreate(&systemsetting.SystemSetting{}).Error
	if err != nil {
		return err
	}
	s.InvalidatePricingCache()
	return nil
}
