package services

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/domain/emotion"
	rundomain "control-panel/internal/domain/run"

	"gorm.io/gorm"
)

// EmotionService 是 io.zerone.emotion 心情能力包的服务层：
// 领域纯函数（internal/domain/emotion）+ RunService 的 RunState 存放通道。
// 状态命名空间 io.zerone.emotion，schema emotion-state v1，subject=agent。
type EmotionService struct {
	runService *RunService
	// now 是可替换时钟：Status 的惰性衰减结算以它为"当前时间"。
	// 生产恒为 time.Now().UTC()；测试可锁定时间以避免用例随时间失效。
	now func() time.Time
}

func NewEmotionService(runService *RunService) *EmotionService {
	return &EmotionService{runService: runService, now: func() time.Time { return time.Now().UTC() }}
}

// EnsureSchemas 幂等注册 emotion-state v1 状态模式：先查后注册，
// 已存在（tenant+namespace+name+version 唯一）则视为成功。
func (s *EmotionService) EnsureSchemas() error {
	for _, tenantID := range s.emotionTenants() {
		if err := s.ensureSchemaFor(tenantID); err != nil {
			return err
		}
	}
	return nil
}

// Status 读取 Agent 自己的心情状态：读取时按事件时间惰性结算衰减。
// 状态不存在时返回基线默认状态（不报错）；结果包含确定性叙述句。
func (s *EmotionService) Status(tenantID, runID string, agentID uint64) (*emotion.State, error) {
	stateRow, err := s.findState(tenantID, runID, agentID)
	if err != nil {
		return nil, err
	}
	if stateRow == nil {
		return emotion.DefaultState(""), nil
	}
	current, err := emotion.StateFromData(stateRow.Data)
	if err != nil {
		return nil, err
	}
	settled := emotion.SettleDecay(current, current.UpdatedAt, s.now())
	if !sameEmotionState(current, settled) {
		// 惰性结算写回（带新 revision）；已冻结的局只允许读，跳过持久化。
		if _, err := s.runService.CommitState(tenantID, runID, stateRow.ID, CommitStateInput{
			ExpectedRevision: stateRow.Revision,
			Data:             settled.ToMap(),
			IdempotencyKey:   settleIdempotencyKey(runID, agentID, stateRow.Revision),
			Reason:           "emotion lazy decay settlement",
			Source:           emotion.RuleSource,
		}); err != nil && !errors.Is(err, rundomain.ErrFrozen) {
			return nil, fmt.Errorf("心情衰减结算写回失败: %w", err)
		}
	}
	return settled, nil
}

// OnEvent 应用一次词表心情事件：先结算衰减到事件时间，再叠加 delta，
// 强度钳制 0–100，重建叙述句后持久化。
// idempotencyKey 经 run_state_changes 唯一索引兜底，重复投递为无操作而非报错。
// 未知事件类型返回领域错误。
func (s *EmotionService) OnEvent(tenantID, runID string, agentID uint64, eventType string, severity int, at time.Time, idempotencyKey string) error {
	if _, ok := emotion.EventDelta(eventType, severity); !ok {
		return fmt.Errorf("%w: %s", emotion.ErrUnknownEventType, eventType)
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return fmt.Errorf("幂等键不能为空")
	}
	if at.IsZero() {
		at = s.now()
	}

	stateRow, err := s.findState(tenantID, runID, agentID)
	if err != nil {
		return err
	}
	if stateRow == nil {
		// 首次事件：先确保模式已注册（幂等），再以基线默认状态为起点结算事件。
		if err := s.ensureSchemaFor(tenantID); err != nil {
			return err
		}
		next, err := emotion.ApplyEvent(emotion.DefaultState(""), eventType, severity, at)
		if err != nil {
			return err
		}
		_, err = s.runService.InitializeState(tenantID, runID, InitializeStateInput{
			Namespace:      emotion.Namespace,
			SchemaName:     emotion.SchemaName,
			SchemaVersion:  emotion.SchemaVersion,
			SubjectType:    emotion.SubjectType,
			SubjectID:      agentSubjectID(agentID),
			Data:           next.ToMap(),
			IdempotencyKey: idempotencyKey,
			Reason:         "emotion event: " + eventType,
			Source:         emotion.RuleSource,
		})
		if errors.Is(err, rundomain.ErrDuplicate) {
			return nil // 重复投递：InitializeState 已按幂等键返回既有状态
		}
		return err
	}

	current, err := emotion.StateFromData(stateRow.Data)
	if err != nil {
		return err
	}
	settled := emotion.SettleDecay(current, current.UpdatedAt, at)
	next, err := emotion.ApplyEvent(settled, eventType, severity, at)
	if err != nil {
		return err
	}
	_, err = s.runService.CommitState(tenantID, runID, stateRow.ID, CommitStateInput{
		ExpectedRevision: stateRow.Revision,
		Data:             next.ToMap(),
		IdempotencyKey:   idempotencyKey,
		Reason:           "emotion event: " + eventType,
		Source:           emotion.RuleSource,
	})
	if errors.Is(err, rundomain.ErrDuplicate) {
		return nil // 重复投递：无操作
	}
	return err
}

// ensureSchemaFor 幂等地为单个租户注册模式；已存在则直接成功。
func (s *EmotionService) ensureSchemaFor(tenantID string) error {
	var count int64
	if err := s.runService.db.Model(&rundomain.StateSchema{}).
		Where("tenant_id = ? AND namespace = ? AND name = ? AND version = ?", tenantID, emotion.Namespace, emotion.SchemaName, emotion.SchemaVersion).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.runService.RegisterStateSchema(tenantID, RegisterStateSchemaInput{
		Namespace:    emotion.Namespace,
		Name:         emotion.SchemaName,
		Version:      emotion.SchemaVersion,
		Schema:       emotion.SchemaDocument(),
		ScopeTypes:   []string{"run"},
		SubjectTypes: []string{emotion.SubjectType},
	})
	if err != nil && !errors.Is(err, rundomain.ErrDuplicate) {
		return err
	}
	return nil
}

// findState 定位 Agent 自己的心情状态行；不存在时返回 nil。
func (s *EmotionService) findState(tenantID, runID string, agentID uint64) (*rundomain.RunState, error) {
	var row rundomain.RunState
	err := s.runService.db.
		Where("tenant_id = ? AND run_id = ? AND namespace = ? AND subject_type = ? AND subject_id = ?",
			tenantID, runID, emotion.Namespace, emotion.SubjectType, agentSubjectID(agentID)).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// emotionTenants 汇总需要注册模式的目标租户（已有状态模式的租户 +
// 已建局的租户），用于跨租户幂等注册；重复注册由唯一索引兜底。
func (s *EmotionService) emotionTenants() []string {
	seen := map[string]struct{}{}
	tenants := make([]string, 0)
	collect := func(table string) {
		var rows []string
		if err := s.runService.db.Table(table).Distinct().Pluck("tenant_id", &rows).Error; err != nil {
			return
		}
		for _, t := range rows {
			if t == "" {
				continue
			}
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				tenants = append(tenants, t)
			}
		}
	}
	collect(rundomain.StateSchema{}.TableName())
	collect(rundomain.Run{}.TableName())
	return tenants
}

func agentSubjectID(agentID uint64) string {
	return strconv.FormatUint(agentID, 10)
}

func settleIdempotencyKey(runID string, agentID uint64, revision uint64) string {
	return fmt.Sprintf("emotion-settle-%s-%d-r%d", runID, agentID, revision)
}

func sameEmotionState(a, b *emotion.State) bool {
	return a.Mood == b.Mood &&
		a.Intensity == b.Intensity &&
		a.Baseline == b.Baseline &&
		a.DecayPerDay == b.DecayPerDay &&
		a.Narration == b.Narration &&
		a.UpdatedAt.Equal(b.UpdatedAt)
}
