package nativememory

import (
	"context"
	"math"
	"strings"
	"testing"

	"control-panel/internal/memory"
	"github.com/stretchr/testify/require"
)

func TestNativeDiagnosticPassesInclusiveZeroAndOneAndPreservesOrder(t *testing.T) {
	for _, options := range []memory.NativeSearchOptions{{Limit: 1, SimilarityThreshold: 0, KeywordWeight: 0}, {Limit: 20, SimilarityThreshold: 1, KeywordWeight: 1}} {
		f, _, child := setupManagedMessages(t)
		// The native result contains only this child. Diagnostics must not add
		// its parent to the response merely because source validation reads it.
		f.searchRows = []map[string]any{child}
		hits, err := f.adapter.SearchDiagnostic(context.Background(), testToken, "original diagnostic query", options)
		require.NoError(t, err)
		require.Len(t, hits, 1)
		require.Equal(t, "9007199254740994", hits[0].ID)
		require.True(t, hits[0].NativeStatus)
		require.True(t, hits[0].Status)
		require.NotNil(t, hits[0].ParentStatus)
		require.True(t, *hits[0].ParentStatus)
		if options.Limit == 1 {
			require.Equal(t, "1", f.searchQuery.Get("top_n"))
			require.Equal(t, "0", f.searchQuery.Get("similarity_threshold"))
			require.Equal(t, "0", f.searchQuery.Get("keywords_similarity_weight"))
		} else {
			require.Equal(t, "20", f.searchQuery.Get("top_n"))
			require.Equal(t, "1", f.searchQuery.Get("similarity_threshold"))
			require.Equal(t, "1", f.searchQuery.Get("keywords_similarity_weight"))
		}
		require.Equal(t, "original diagnostic query", f.searchQuery.Get("query"))
		require.Empty(t, f.searchQuery.Get("session_id"))
		require.Zero(t, f.posts)
		require.Zero(t, f.puts)
	}
	f, parent, child := setupManagedMessages(t)
	f.searchRows = []map[string]any{child, parent}
	hits, err := f.adapter.SearchDiagnostic(context.Background(), testToken, "query", memory.NativeSearchOptions{Limit: 5, SimilarityThreshold: 0.35, KeywordWeight: 0.6})
	require.NoError(t, err)
	require.Equal(t, []string{"9007199254740994", "9007199254740993"}, []string{hits[0].ID, hits[1].ID})
	require.Equal(t, "0.35", f.searchQuery.Get("similarity_threshold"))
	require.Equal(t, "0.6", f.searchQuery.Get("keywords_similarity_weight"))
	_, err = f.adapter.SearchMemories(context.Background(), testToken, "normal chat", 5)
	require.NoError(t, err)
	require.Equal(t, "0.2", f.searchQuery.Get("similarity_threshold"))
	require.Equal(t, "0.7", f.searchQuery.Get("keywords_similarity_weight"))
}

func TestNativeDiagnosticRejectsInvalidOptionsBeforeAnyIO(t *testing.T) {
	for _, options := range []memory.NativeSearchOptions{
		{Limit: 0}, {Limit: -1}, {Limit: 21},
		{Limit: 5, SimilarityThreshold: -0.01},
		{Limit: 5, SimilarityThreshold: 1.01},
		{Limit: 5, SimilarityThreshold: math.NaN()},
		{Limit: 5, SimilarityThreshold: math.Inf(1)},
		{Limit: 5, KeywordWeight: -0.01},
		{Limit: 5, KeywordWeight: 1.01},
		{Limit: 5, KeywordWeight: math.NaN()},
		{Limit: 5, KeywordWeight: math.Inf(-1)},
	} {
		f := conversationTestFixture(t)
		_, err := f.adapter.SearchDiagnostic(context.Background(), testToken, "query", options)
		require.ErrorIs(t, err, memory.ErrInvalid)
		require.Zero(t, f.requests)
	}
	for _, query := range []string{"", "  ", strings.Repeat("字", 2001), string([]byte{0xff})} {
		f := conversationTestFixture(t)
		_, err := f.adapter.SearchDiagnostic(context.Background(), testToken, query, memory.NativeSearchOptions{Limit: 5})
		require.ErrorIs(t, err, memory.ErrInvalid)
		require.Zero(t, f.requests)
	}
}

func TestNativeDiagnosticSharesScopeSourceAndEffectiveStatusValidation(t *testing.T) {
	for _, scenario := range []string{"foreign-user", "foreign-agent", "foreign-session", "foreign-source", "parent-disabled", "child-disabled", "forgotten-parent"} {
		t.Run(scenario, func(t *testing.T) {
			f, parent, child := setupManagedMessages(t)
			f.searchRows = []map[string]any{child}
			switch scenario {
			case "foreign-user":
				child["user_id"] = "another-user"
			case "foreign-agent":
				child["agent_id"] = "another-agent"
			case "foreign-session":
				child["session_id"] = "another-session"
			case "foreign-source":
				scope, _ := f.adapter.scope(testBank, testToken)
				parent["session_id"] = conversationSession(scope, "different-operation")
			case "parent-disabled":
				parent["status"] = false
			case "child-disabled":
				child["status"] = false
			case "forgotten-parent":
				parent["forget_at"] = "2026-11-08 12:00:00"
			}
			hits, err := f.adapter.SearchDiagnostic(context.Background(), testToken, "query", memory.NativeSearchOptions{Limit: 5, SimilarityThreshold: 0.2, KeywordWeight: 0.7})
			if scenario == "parent-disabled" || scenario == "child-disabled" || scenario == "forgotten-parent" {
				require.NoError(t, err)
				require.Empty(t, hits)
			} else {
				require.ErrorIs(t, err, memory.ErrIndexUnknown)
			}
			require.Zero(t, f.puts)
			require.Zero(t, f.posts)
		})
	}
}
