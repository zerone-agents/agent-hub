package memory

import "context"

// ListSubjects exposes recoverable business identities for this configured
// application only. Cleared identities are never reconstructed from hashes.
func (s *Service) ListSubjects(ctx context.Context) (SubjectList, error) {
	const limit = 1000
	out := SubjectList{Subjects: []Subject{}, Limit: limit}
	if err := s.lockForRecall(ctx); err != nil {
		return out, err
	}
	defer s.mu.Unlock()
	var rows []lifecycleRow
	if err := s.db.WithContext(ctx).Where("app_key = ? AND state IN ?", s.appKey(), []string{"active", "frozen"}).Order("updated_at DESC, id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.Limited = true
		rows = rows[:limit]
	}
	for i := range rows {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		row := &rows[i]
		if len(row.ScopePayload) == 0 || len(row.WrappedKey) == 0 {
			out.MissingScopeCount++
			continue
		}
		var scope Scope
		if err := s.decode(ctx, row, "scope", row.ScopePayload, &scope); err != nil {
			return out, err
		}
		if err := s.check(scope); err != nil {
			return out, err
		}
		if scopeKey(scope) != row.ID {
			return out, ErrForbidden
		}
		out.Subjects = append(out.Subjects, Subject{Scope: scope, State: row.State, Revision: row.Revision})
	}
	if out.MissingScopeCount > 0 {
		out.Note = "部分历史账户缺少可恢复身份，已跳过；请由可信业务服务携带原始账户范围再次访问或幂等激活以回填。"
	}
	return out, nil
}
