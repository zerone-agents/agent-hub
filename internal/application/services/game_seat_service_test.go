package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/gameseat"
	"control-panel/internal/domain/run"
	"github.com/stretchr/testify/require"
)

func TestGameSeatVotePersistsAcrossSessionsAndIsIdempotent(t *testing.T) {
	runs, db := newRunTestService(t)
	service := NewGameSeatService(db, "t1", runs, nil, nil, 120)
	require.NoError(t, service.Migrate())
	agentRow := agent.AgentConfig{TenantID: "t1", Name: "wolfcha-zhou", SystemPrompt: "大家先别急", ContentHash: "test"}
	require.NoError(t, db.Create(&agentRow).Error)
	character := gameseat.Character{TenantID: "t1", GameKey: "wolfcha", Key: "zhou", AgentID: agentRow.ID, Name: "老周"}
	require.NoError(t, db.Create(&character).Error)
	roster, err := service.Roster("wolfcha")
	require.NoError(t, err)
	require.Equal(t, "zhou", roster[0].CharacterID)
	gameRun, err := runs.Create("t1", CreateRunInput{Name: "player-one"})
	require.NoError(t, err)
	_, err = runs.Transition("t1", gameRun.ID, run.StatusRunning)
	require.NoError(t, err)
	require.NoError(t, db.Create(&gameseat.PlayerRun{TenantID: "t1", GameKey: "wolfcha", PlayerID: "player-one", RunID: gameRun.ID, Games: 2}).Error)
	session := gameseat.Session{ID: "11111111-1111-4111-8111-111111111111", TenantID: "t1", GameKey: "wolfcha", PlayerID: "player-one", PlayerName: "玩家", RunID: gameRun.ID, GameNumber: 1}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&gameseat.SessionSeat{SessionID: session.ID, Seat: 2, CharacterID: character.ID}).Error)
	seat := 2
	event := GameSeatEventInput{Seq: 1, Type: "vote", Day: 1, TargetSeat: &seat, Detail: "玩家投票给老周"}
	require.NoError(t, service.Event(session.ID, event))
	first, err := service.Memory("wolfcha", "player-one", "zhou")
	require.NoError(t, err)
	require.Less(t, first.Attitude, 0)
	require.Contains(t, first.Mood, "-20")
	require.NotEmpty(t, first.Recent)
	require.NoError(t, service.Event(session.ID, event))
	second, err := service.Memory("wolfcha", "player-one", "zhou")
	require.NoError(t, err)
	require.Equal(t, first.Attitude, second.Attitude)
	require.Equal(t, first.Recent, second.Recent)

	// A later game for the same player resolves the same long-lived Run.
	later := gameseat.Session{ID: "22222222-2222-4222-8222-222222222222", TenantID: "t1", GameKey: "wolfcha", PlayerID: "player-one", PlayerName: "玩家", RunID: gameRun.ID, GameNumber: 2}
	require.NoError(t, db.Create(&later).Error)
	view, err := service.Memory(later.GameKey, later.PlayerID, "zhou")
	require.NoError(t, err)
	require.Contains(t, view.Recent[0], "玩家投票给老周")
	require.NoError(t, db.Create(&gameseat.SessionSeat{SessionID: later.ID, Seat: 2, CharacterID: character.ID}).Error)
	service.budget = 2
	original := json.RawMessage(`{"role":"user","content":"原始提示"}`)
	service.modelCall = func(_ context.Context, _ agent.AgentConfig, messages []json.RawMessage, input GameSeatGenerateInput) (*http.Response, error) {
		require.Len(t, messages, 2)
		require.JSONEq(t, string(original), string(messages[1]))
		require.Contains(t, string(messages[0]), "玩家投票给老周")
		if input.Stream {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"object\":\"chat.completion.chunk\",\"choices\":[]}\n\ndata: [DONE]\n\n"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"object":"chat.completion","choices":[{"message":{"role":"assistant","content":"大家先别急"}}]}`))}, nil
	}
	for _, stream := range []bool{false, true} {
		response, err := service.Generate(context.Background(), later.ID, GameSeatGenerateInput{Seat: 2, CharacterID: "zhou", Phase: "DAY_SPEECH", Messages: []json.RawMessage{original}, Stream: stream})
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		if stream {
			require.Contains(t, string(body), "chat.completion.chunk")
		} else {
			require.Contains(t, string(body), "chat.completion")
		}
	}
	_, err = service.Generate(context.Background(), later.ID, GameSeatGenerateInput{Seat: 2, CharacterID: "zhou", Phase: "DAY_SPEECH", Messages: []json.RawMessage{original}})
	require.ErrorIs(t, err, ErrGameSeatBudget)
}
