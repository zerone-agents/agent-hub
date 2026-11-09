package memory

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

func (s *Service) ForgetNativeMessage(ctx context.Context, sc Scope, id string) (NativeMessage, error) {
	if _, err := s.native(); err != nil {
		return NativeMessage{}, err
	}
	forgetter, ok := s.cfg.Native.(NativeForget)
	if !ok {
		return NativeMessage{}, ErrUnsupported
	}
	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil || parsed < 1 || strconv.FormatInt(parsed, 10) != id {
		return NativeMessage{}, ErrInvalid
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, e := s.life(tx, sc, true); return e }); err != nil {
		return NativeMessage{}, err
	}
	return forgetter.ForgetMessage(ctx, "memory-"+scopeKey(sc), id)
}

func (s *Service) nativeManagement() (NativeManagement, error) {
	if s.cfg.Backend != BackendNative {
		return nil, ErrUnsupported
	}
	if manager, ok := s.cfg.Native.(NativeManagement); ok {
		return manager, nil
	}
	if manager, ok := s.cfg.Index.(NativeManagement); ok {
		return manager, nil
	}
	return nil, ErrUnsupported
}

func (s *Service) NativeConfiguration(ctx context.Context) (NativeBankInfo, error) {
	manager, err := s.nativeManagement()
	if err != nil {
		return NativeBankInfo{}, err
	}
	return manager.BankInfo(ctx)
}

// SetNativeMessageStatus changes a record's own native switch. Stopping a RAW
// prevents Hub from supplying its derived records without rewriting them. This
// neither erases data nor recalls context already read by an in-flight run.
func (s *Service) SetNativeMessageStatus(ctx context.Context, sc Scope, id string, enabled, expectedEnabled bool) (NativeMessage, error) {
	if _, err := s.native(); err != nil {
		return NativeMessage{}, err
	}
	manager, err := s.nativeManagement()
	if err != nil {
		return NativeMessage{}, err
	}
	parsed, parseErr := strconv.ParseInt(id, 10, 64)
	if parseErr != nil || parsed < 1 || strconv.FormatInt(parsed, 10) != id {
		return NativeMessage{}, ErrInvalid
	}
	if err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, e := s.life(tx, sc, true); return e }); err != nil {
		return NativeMessage{}, err
	}
	return manager.SetMessageStatus(ctx, "memory-"+scopeKey(sc), id, enabled, expectedEnabled)
}

// DiagnoseNativeSearch exposes a single bounded provider search to managers.
// It does not change the defaults or recent-RAW policy used by chat context.
func (s *Service) DiagnoseNativeSearch(ctx context.Context, sc Scope, query string, options NativeSearchOptions) (NativeSearchResult, error) {
	if _, err := s.native(); err != nil {
		return NativeSearchResult{}, err
	}
	diagnostic, ok := s.cfg.Native.(NativeDiagnostics)
	if !ok {
		return NativeSearchResult{}, ErrUnsupported
	}
	if strings.TrimSpace(query) == "" || utf8.RuneCountInString(query) > 2000 || options.Limit < 1 || options.Limit > 20 || !(options.SimilarityThreshold >= 0 && options.SimilarityThreshold <= 1) || !(options.KeywordWeight >= 0 && options.KeywordWeight <= 1) {
		return NativeSearchResult{}, ErrInvalid
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, e := s.life(tx, sc, true); return e }); err != nil {
		return NativeSearchResult{}, err
	}
	messages, err := diagnostic.SearchDiagnostic(ctx, "memory-"+scopeKey(sc), query, options)
	if err != nil {
		return NativeSearchResult{}, err
	}
	if messages == nil {
		messages = []NativeMessage{}
	}
	status := "ready"
	if len(messages) == 0 {
		status = "empty"
	}
	return NativeSearchResult{Status: status, Messages: messages, Options: options}, nil
}
