package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

type conversationValue struct {
	Input   ConversationInput   `json:"input"`
	Receipt ConversationReceipt `json:"receipt"`
}

func (s *Service) native() (NativeConversations, error) {
	if s.cfg.Backend != BackendNative || s.cfg.Native == nil {
		return nil, ErrUnsupported
	}
	return s.cfg.Native, nil
}

func validConversation(in ConversationInput, policy string) bool {
	for _, v := range []string{in.OperationID, in.SessionID, in.TurnID, in.ConsentRef} {
		if strings.TrimSpace(v) == "" || len(v) > 128 {
			return false
		}
	}
	return in.DataClass == "synthetic" && in.PolicyVersion == policy && strings.TrimSpace(in.UserInput) != "" && strings.TrimSpace(in.AgentResponse) != "" && utf8.RuneCountInString(in.UserInput) <= 6000 && utf8.RuneCountInString(in.AgentResponse) <= 6000
}

// IngestConversation reserves a turn in durable encrypted storage before any
// provider write. Replays return the stored result; an uncertain write is never
// repeated. Provider extraction itself remains asynchronous.
func (s *Service) IngestConversation(ctx context.Context, sc Scope, in ConversationInput) (ConversationReceipt, error) {
	native, err := s.native()
	if err != nil {
		return ConversationReceipt{}, err
	}
	if !validConversation(in, s.cfg.PolicyVersion) {
		return ConversationReceipt{}, ErrInvalid
	}
	s.mu.Lock()
	var row conversationRow
	var value conversationValue
	var reserved bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		life, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		if err = s.consentAllowed(tx, life, in.ConsentRef); err != nil {
			return err
		}
		key := digest([]string{life.ID, in.OperationID})
		hash, err := s.fingerprint(ctx, life, in)
		if err != nil {
			return err
		}
		err = tx.First(&row, "scope_key = ? AND operation_key = ?", life.ID, key).Error
		if err == nil {
			if row.RequestHash != hash {
				return ErrConflict
			}
			return s.decode(ctx, life, row.ID, row.Payload, &value)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// A caller cannot submit the same runtime turn with a fresh operation key.
		row = conversationRow{ID: digest([]string{life.ID, in.SessionID, in.TurnID}), ScopeKey: life.ID, OperationKey: key, RequestHash: hash}
		var count int64
		if err = tx.Model(&conversationRow{}).Where("id = ?", row.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrConflict
		}
		value = conversationValue{Input: in, Receipt: ConversationReceipt{OperationID: in.OperationID, TurnID: in.TurnID, SessionID: in.SessionID, NativeIngestResult: NativeIngestResult{Status: "submitting", Messages: []NativeMessage{}}}}
		row.Payload, err = s.encode(ctx, life, row.ID, value)
		if err != nil {
			return err
		}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		reserved = true
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return ConversationReceipt{}, err
	}
	if !reserved {
		return value.Receipt, nil
	}
	result, providerErr := native.AddConversation(ctx, "memory-"+scopeKey(sc), in.NativeTurn)
	if providerErr != nil {
		result.Status = "unknown"
	}
	if result.Status == "" {
		result.Status = "unknown"
	}
	if result.Messages == nil {
		result.Messages = []NativeMessage{}
	}
	value.Receipt.NativeIngestResult = result
	// Persist the uncertain state even when the request context has expired.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err = s.saveConversation(saveCtx, sc, &row, &value); err != nil {
		return value.Receipt, ErrIndexUnknown
	}
	// The receipt tells callers to query, never silently replay the POST.
	return value.Receipt, nil
}

func (s *Service) saveConversation(ctx context.Context, sc Scope, row *conversationRow, value *conversationValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		life, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		payload, err := s.encode(ctx, life, row.ID, value)
		if err != nil {
			return err
		}
		res := tx.Model(&conversationRow{}).Where("id = ? AND scope_key = ? AND request_hash = ?", row.ID, life.ID, row.RequestHash).Update("payload", payload)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
}

// ConversationStatus uses the exact persisted turn to reconcile reads only.
func (s *Service) ConversationStatus(ctx context.Context, sc Scope, operationID string) (ConversationReceipt, error) {
	native, err := s.native()
	if err != nil {
		return ConversationReceipt{}, err
	}
	if strings.TrimSpace(operationID) == "" || len(operationID) > 128 {
		return ConversationReceipt{}, ErrInvalid
	}
	var row conversationRow
	var value conversationValue
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		life, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		err = tx.First(&row, "scope_key = ? AND operation_key = ?", life.ID, digest([]string{life.ID, operationID})).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return s.decode(ctx, life, row.ID, row.Payload, &value)
	})
	if err != nil {
		return ConversationReceipt{}, err
	}
	result, err := native.FindConversation(ctx, "memory-"+scopeKey(sc), value.Input.NativeTurn)
	if err != nil {
		return value.Receipt, ErrIndexUnknown
	}
	if result.Status == "" {
		result.Status = "unknown"
	}
	if result.Messages == nil {
		result.Messages = []NativeMessage{}
	}
	value.Receipt.NativeIngestResult = result
	if err = s.saveConversation(ctx, sc, &row, &value); err != nil {
		return ConversationReceipt{}, err
	}
	return value.Receipt, nil
}

func (s *Service) ListNativeMemories(ctx context.Context, sc Scope) ([]NativeMessage, error) {
	native, err := s.native()
	if err != nil {
		return nil, err
	}
	if err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := s.life(tx, sc, true); return err }); err != nil {
		return nil, err
	}
	messages, err := native.ListMemories(ctx, "memory-"+scopeKey(sc))
	if messages == nil && err == nil {
		messages = []NativeMessage{}
	}
	return messages, err
}

func (s *Service) NativeRecall(ctx context.Context, sc Scope, query string, limit int) (NativeContext, error) {
	native, err := s.native()
	if err != nil {
		return NativeContext{}, err
	}
	if strings.TrimSpace(query) == "" || utf8.RuneCountInString(query) > 2000 || limit < 1 || limit > 20 {
		return NativeContext{}, ErrInvalid
	}
	if err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := s.life(tx, sc, true); return err }); err != nil {
		return NativeContext{}, err
	}
	recent, err := native.ListMemories(ctx, "memory-"+scopeKey(sc))
	if err != nil {
		return NativeContext{}, err
	}
	sort.SliceStable(recent, func(i, j int) bool {
		if recent[i].CreatedAt != recent[j].CreatedAt {
			return recent[i].CreatedAt > recent[j].CreatedAt
		}
		if len(recent[i].ID) != len(recent[j].ID) {
			return len(recent[i].ID) > len(recent[j].ID)
		}
		return recent[i].ID > recent[j].ID
	})
	messages, err := native.SearchMemories(ctx, "memory-"+scopeKey(sc), query, limit)
	if err != nil {
		return NativeContext{}, err
	}
	bounded := []NativeMessage{}
	seen := map[string]bool{}
	chars, rawCount := 0, 0
	limited := false
	add := func(message NativeMessage) bool {
		if seen[message.ID] || !message.Status {
			return false
		}
		n := utf8.RuneCountInString(message.Content)
		if chars+n > 4000 || len(bounded) >= limit {
			limited = true
			return false
		}
		chars += n
		seen[message.ID] = true
		bounded = append(bounded, message)
		return true
	}
	for _, message := range recent {
		if message.Type == "raw" && rawCount < 2 && add(message) {
			rawCount++
		}
	}
	for _, message := range messages {
		add(message)
	}
	status := "ready"
	if len(bounded) == 0 {
		status = "empty"
	}
	return NativeContext{Messages: bounded, Status: status, BudgetLimited: limited, RecentSourceIncluded: rawCount > 0, CurrentnessNote: "Recent original conversations are supplied before retrieved model-derived memories. Historical conflicts may remain; use the latest explicit user correction and source time."}, nil
}
