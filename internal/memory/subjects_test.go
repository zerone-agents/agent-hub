package memory

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubjectListUsesEncryptedBusinessScopeAndOmitsCleanedIdentities(t *testing.T) {
	s, sc, _ := setup(t)
	other := sc
	other.UserID = "business-bob"
	other.LifecycleID = "private-life-bob"
	_, err := s.Activate(testCtx, other, "activate-bob")
	require.NoError(t, err)
	result, err := s.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Len(t, result.Subjects, 2)
	seen := map[string]Subject{}
	for _, subject := range result.Subjects {
		seen[subject.Scope.UserID] = subject
	}
	require.Equal(t, sc, seen[sc.UserID].Scope)
	require.Equal(t, "active", seen[sc.UserID].State)
	var stored lifecycleRow
	require.NoError(t, s.db.First(&stored, "id = ?", scopeKey(other)).Error)
	require.NotEmpty(t, stored.ScopePayload)
	require.NotContains(t, string(stored.ScopePayload), other.UserID)
	require.NotContains(t, string(stored.ScopePayload), other.LifecycleID)
	// Other configured applications cannot enumerate identities from this app.
	cfg := s.cfg
	cfg.ApplicationID = "another-app"
	separate, err := NewService(s.db, cfg)
	require.NoError(t, err)
	isolated, err := separate.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Empty(t, isolated.Subjects)
	_, err = s.Deactivate(testCtx, sc, "freeze")
	require.NoError(t, err)
	result, err = s.ListSubjects(testCtx)
	require.NoError(t, err)
	foundFrozen := false
	for _, subject := range result.Subjects {
		if subject.Scope.UserID == sc.UserID {
			foundFrozen = subject.State == "frozen"
		}
	}
	require.True(t, foundFrozen)
	syncAll(t, s)
	result, err = s.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Len(t, result.Subjects, 1)
	require.Equal(t, other, result.Subjects[0].Scope)
	stored = lifecycleRow{}
	require.NoError(t, s.db.First(&stored, "id = ?", scopeKey(sc)).Error)
	require.Empty(t, stored.ScopePayload)
	require.Empty(t, stored.WrappedKey)
}
func TestLegacySubjectScopeIsSkippedThenBackfilledFromKnownIdentity(t *testing.T) {
	s, sc, _ := setup(t)
	require.NoError(t, s.db.Model(&lifecycleRow{}).Where("id = ?", scopeKey(sc)).Update("scope_payload", nil).Error)
	before, err := s.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Empty(t, before.Subjects)
	require.Equal(t, 1, before.MissingScopeCount)
	require.NotEmpty(t, before.Note)
	_, err = s.Activate(testCtx, sc, "activate")
	require.NoError(t, err)
	after, err := s.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Len(t, after.Subjects, 1)
	require.Equal(t, sc, after.Subjects[0].Scope)
	require.Zero(t, after.MissingScopeCount)
	require.NoError(t, s.db.Model(&lifecycleRow{}).Where("id = ?", scopeKey(sc)).Update("scope_payload", nil).Error)
	_, err = s.List(testCtx, sc)
	require.NoError(t, err)
	after, err = s.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Len(t, after.Subjects, 1)
	// Ciphertext uses the dedicated lifecycle scope AAD, not another record's.
	var row lifecycleRow
	require.NoError(t, s.db.First(&row, "id = ?", scopeKey(sc)).Error)
	var scope Scope
	require.Error(t, s.decode(testCtx, &row, "profile", row.ScopePayload, &scope))
	serialized, _ := json.Marshal(row)
	require.NotContains(t, string(serialized), strings.Repeat("01", 32))
}

func TestLateScopeBackfillCannotRestoreCleanedLifecycleIdentity(t *testing.T) {
	s, sc, _ := setup(t)
	stale, err := s.life(s.db, sc, false)
	require.NoError(t, err)
	stale.ScopePayload = nil
	_, err = s.Deactivate(testCtx, sc, "erase-identity")
	require.NoError(t, err)
	syncAll(t, s)
	// A reader that obtained the old DEK before cleanup cannot write an identity
	// envelope back after the worker has cleared the lifecycle.
	require.NoError(t, s.backfillScope(s.db.WithContext(testCtx), stale, sc))
	var stored lifecycleRow
	require.NoError(t, s.db.First(&stored, "id = ?", scopeKey(sc)).Error)
	require.Equal(t, "cleaned", stored.State)
	require.Empty(t, stored.ScopePayload)
	require.Empty(t, stored.WrappedKey)
	subjects, err := s.ListSubjects(testCtx)
	require.NoError(t, err)
	require.Empty(t, subjects.Subjects)
	require.Zero(t, subjects.MissingScopeCount)
	_, err = s.Activate(testCtx, sc, "activate")
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.List(testCtx, sc)
	require.ErrorIs(t, err, ErrForbidden)
	out, err := s.Recall(testCtx, sc, "旧记忆")
	require.NoError(t, err)
	require.Empty(t, out.Facts)
	valid, err := s.Validate(testCtx, sc, stored.Revision)
	require.NoError(t, err)
	require.False(t, valid)
	stored = lifecycleRow{}
	require.NoError(t, s.db.First(&stored, "id = ?", scopeKey(sc)).Error)
	require.Empty(t, stored.ScopePayload)
}
