package nativememory

import (
	"context"
	"encoding/json"
	"testing"

	"control-panel/internal/memory"
	"github.com/stretchr/testify/require"
)

func bankMetadata() map[string]any {
	return map[string]any{"id": testBank, "tenant_id": "native_owner", "permissions": "me", "memory_type": []string{"raw", "semantic", "episodic", "procedural"}, "name": "Synthetic memory", "description": "Synthetic integration", "storage_type": "table", "llm_id": "356", "embd_id": "337", "memory_size": 5242880, "forgetting_policy": "FIFO", "temperature": 0, "create_time": 1794117215192, "api_key": "secret-provider-key", "system_prompt": "secret-system-prompt", "user_prompt": "secret-user-prompt", "owner_name": "private-owner", "avatar": "private-avatar", "provider": map[string]any{"credential": "private-token"}}
}

func TestBankInfoAllowlistPreservesZeroWithoutLeakingConfiguration(t *testing.T) {
	f := conversationTestFixture(t)
	f.configOverride = bankMetadata()
	info, err := f.adapter.BankInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, "Synthetic memory", info.Name)
	require.Equal(t, []string{"raw", "semantic", "episodic", "procedural"}, info.MemoryTypes)
	require.Equal(t, "356", info.ChatModel)
	require.Equal(t, "337", info.EmbeddingModel)
	require.NotNil(t, info.CapacityBytes)
	require.EqualValues(t, 5242880, *info.CapacityBytes)
	require.NotNil(t, info.Temperature)
	require.Zero(t, *info.Temperature)
	require.Equal(t, "2026-11-08T05:53:35.192Z", info.CreatedAt)
	raw, err := json.Marshal(info)
	require.NoError(t, err)
	for _, forbidden := range []string{"secret", "private-", "owner_name", "tenant_id", "api_key", "system_prompt", "user_prompt", "avatar", "credential", testBank} {
		require.NotContains(t, string(raw), forbidden)
	}
	// Reading an existing raw-only application's metadata remains supported.
	f.configOverride["memory_type"] = []string{"raw"}
	delete(f.configOverride, "memory_size")
	delete(f.configOverride, "temperature")
	delete(f.configOverride, "create_time")
	info, err = f.adapter.BankInfo(context.Background())
	require.NoError(t, err)
	require.Nil(t, info.CapacityBytes)
	require.Nil(t, info.Temperature)
	require.Empty(t, info.CreatedAt)
	require.Zero(t, f.puts)
	require.Zero(t, f.posts)
}

func TestBankInfoRejectsWrongBankAndMalformedMetadata(t *testing.T) {
	for _, field := range []string{"wrong-bank", "shared-permissions", "unsupported-type", "negative-capacity", "temperature", "timestamp", "model-reference"} {
		t.Run(field, func(t *testing.T) {
			f := conversationTestFixture(t)
			f.configOverride = bankMetadata()
			switch field {
			case "wrong-bank":
				f.configOverride["id"] = "other_bank"
			case "shared-permissions":
				f.configOverride["permissions"] = "team"
			case "unsupported-type":
				f.configOverride["memory_type"] = []string{"raw", "other"}
			case "negative-capacity":
				f.configOverride["memory_size"] = -1
			case "temperature":
				f.configOverride["temperature"] = 2
			case "timestamp":
				f.configOverride["create_time"] = "malformed"
			case "model-reference":
				f.configOverride["llm_id"] = map[string]any{"api_key": "secret"}
			}
			_, err := f.adapter.BankInfo(context.Background())
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
		})
	}
}

func setupManagedMessages(t *testing.T) (*conversationFixture, map[string]any, map[string]any) {
	f := conversationTestFixture(t)
	scope, err := f.adapter.scope(testBank, testToken)
	require.NoError(t, err)
	session := conversationSession(scope, "managed")
	parent := f.row("9007199254740993", "0", "raw", session, "User Input: Synthetic input\nAgent Response: Actual response")
	child := f.row("9007199254740994", "9007199254740993", "semantic", session, "Synthetic derived content")
	f.rows = []map[string]any{parent, child}
	return f, parent, child
}

func TestStatusControlKeepsNativeAndEffectiveStatesSeparate(t *testing.T) {
	f, parent, child := setupManagedMessages(t)
	row, err := f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740993", false, true)
	require.NoError(t, err)
	require.False(t, row.NativeStatus)
	require.False(t, row.Status)
	require.True(t, row.StatusChangeAllowed)
	require.Nil(t, row.ParentStatus)
	require.Equal(t, false, parent["status"])
	require.Equal(t, true, child["status"], "native PUT must not cascade to derived rows")
	rows, err := f.adapter.ListMemories(context.Background(), testToken)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.True(t, rows[1].NativeStatus)
	require.False(t, rows[1].Status)
	require.NotNil(t, rows[1].ParentStatus)
	require.False(t, *rows[1].ParentStatus)
	require.True(t, rows[1].StatusChangeAllowed)
	// A child can be independently disabled while its parent suppresses supply.
	row, err = f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", false, true)
	require.NoError(t, err)
	require.False(t, row.NativeStatus)
	_, err = f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", true, false)
	require.ErrorIs(t, err, memory.ErrConflict)
	require.Equal(t, 2, f.puts)
	_, err = f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740993", true, false)
	require.NoError(t, err)
	row, err = f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", true, false)
	require.NoError(t, err)
	require.True(t, row.NativeStatus)
	require.True(t, row.Status)
	require.NotNil(t, row.ParentStatus)
	require.True(t, *row.ParentStatus)
	require.Equal(t, []string{"9007199254740993", "9007199254740994", "9007199254740993", "9007199254740994"}, f.putIDs)
}

func TestStatusControlPreconditionsAndScopePreventAnyMutation(t *testing.T) {
	for _, scenario := range []string{"stale-expected", "foreign-user", "foreign-session", "foreign-parent", "forgotten-target", "forgotten-parent", "invalid-id"} {
		t.Run(scenario, func(t *testing.T) {
			f, parent, child := setupManagedMessages(t)
			id, expected := "9007199254740994", true
			switch scenario {
			case "stale-expected":
				expected = false
			case "foreign-user":
				child["user_id"] = "other-user"
			case "foreign-session":
				child["session_id"] = "other-session"
			case "foreign-parent":
				scope, _ := f.adapter.scope(testBank, testToken)
				parent["session_id"] = conversationSession(scope, "other-operation")
			case "forgotten-target":
				child["forget_at"] = "2026-11-08 12:00:00"
			case "forgotten-parent":
				parent["forget_at"] = "2026-11-08 12:00:00"
			case "invalid-id":
				id = "1/other"
			}
			_, err := f.adapter.SetMessageStatus(context.Background(), testToken, id, false, expected)
			require.Error(t, err)
			require.Zero(t, f.puts)
		})
	}
}

func TestStatusLostResponseAndReadbackDoNotReplayPUT(t *testing.T) {
	f, _, _ := setupManagedMessages(t)
	f.lostPutResponse = true
	row, err := f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", false, true)
	require.NoError(t, err)
	require.False(t, row.NativeStatus)
	require.False(t, row.Status)
	require.Equal(t, 1, f.puts)
	// The original expected value is now stale. No blind repeat is performed.
	_, err = f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", false, true)
	require.ErrorIs(t, err, memory.ErrConflict)
	require.Equal(t, 1, f.puts)
	row, err = f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", false, false)
	require.NoError(t, err)
	require.False(t, row.NativeStatus)
	require.Equal(t, 1, f.puts)
}

func TestStatusSuccessfulEnvelopeCannotReplaceIndependentReadback(t *testing.T) {
	for _, failure := range []string{"no-write", "content-changed", "source-changed"} {
		t.Run(failure, func(t *testing.T) {
			f, _, _ := setupManagedMessages(t)
			switch failure {
			case "no-write":
				f.skipStatusWrite = true
			case "content-changed":
				f.mutateAfterPut = func(row map[string]any) { row["content"] = "Unexpected changed text" }
			case "source-changed":
				f.mutateAfterPut = func(row map[string]any) { row["source_id"] = json.Number("9007199254740995") }
			}
			_, err := f.adapter.SetMessageStatus(context.Background(), testToken, "9007199254740994", false, true)
			require.ErrorIs(t, err, memory.ErrIndexUnknown)
			require.Equal(t, 1, f.puts)
		})
	}
}
