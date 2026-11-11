package nativememory

import (
	"context"
	"testing"

	"control-panel/internal/memory"
	"github.com/stretchr/testify/require"
)

func TestNativeForgetConfirmsMarkerAndPreventsRecall(t *testing.T) {
	for _, lostResponse := range []bool{false, true} {
		f, parent, child := setupManagedMessages(t)
		f.lostDeleteResponse = lostResponse
		id := "9007199254740994"
		row, err := f.adapter.ForgetMessage(context.Background(), testToken, id)
		require.NoError(t, err)
		require.NotEmpty(t, row.ForgottenAt)
		require.False(t, row.Status)
		require.True(t, row.NativeStatus, "forget does not change the enable switch")
		require.False(t, row.StatusChangeAllowed)
		require.Nil(t, parent["forget_at"], "forgetting a child leaves the source unchanged")
		require.Equal(t, 1, f.deletes)
		_, err = f.adapter.ForgetMessage(context.Background(), testToken, id)
		require.NoError(t, err)
		require.Equal(t, 1, f.deletes, "confirmed markers make repeated requests read-only")
		f.searchRows = []map[string]any{child}
		result, err := f.adapter.SearchDiagnostic(context.Background(), testToken, "test", memory.NativeSearchOptions{Limit: 5, SimilarityThreshold: 0.2, KeywordWeight: 0.7})
		require.NoError(t, err)
		require.Empty(t, result, "even stale provider hits must not restore forgotten records")
		_, err = f.adapter.SetMessageStatus(context.Background(), testToken, id, true, true)
		require.ErrorIs(t, err, memory.ErrUnsupported)
		require.Zero(t, f.puts)
	}
}

func TestNativeForgetSourceSuppressesChildrenWithoutMarkingThemForgotten(t *testing.T) {
	f, _, child := setupManagedMessages(t)
	_, err := f.adapter.ForgetMessage(context.Background(), testToken, "9007199254740993")
	require.NoError(t, err)
	rows, err := f.adapter.ListMemories(context.Background(), testToken)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEmpty(t, rows[0].ForgottenAt)
	require.Empty(t, rows[1].ForgottenAt)
	require.Equal(t, rows[0].ForgottenAt, rows[1].ParentForgottenAt)
	require.False(t, rows[1].Status)
	require.Nil(t, child["forget_at"], "native forget is not a cascading operation")
}

func TestNativeForgetScopeAndReadbackFailures(t *testing.T) {
	for _, scenario := range []string{"foreign-user", "foreign-parent", "foreign-session", "invalid-id", "no-write", "changed-content"} {
		t.Run(scenario, func(t *testing.T) {
			f, parent, child := setupManagedMessages(t)
			id := "9007199254740994"
			writes := 0
			switch scenario {
			case "foreign-user":
				child["user_id"] = "other"
			case "foreign-parent":
				parent["user_id"] = "other"
			case "foreign-session":
				child["session_id"] = "other"
			case "invalid-id":
				id = "01"
			case "no-write":
				f.skipForgetWrite = true
				writes = 1
			case "changed-content":
				f.mutateAfterDelete = func(row map[string]any) { row["content"] = "changed" }
				writes = 1
			}
			_, err := f.adapter.ForgetMessage(context.Background(), testToken, id)
			require.Error(t, err)
			require.Equal(t, writes, f.deletes)
		})
	}
}
