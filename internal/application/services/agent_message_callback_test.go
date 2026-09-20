package services

import (
	"context"
	"errors"
	"testing"

	"control-panel/internal/domain/agentrelation"
	eventdomain "control-panel/internal/domain/event"

	"github.com/stretchr/testify/require"
)

type callbackWaitFake struct {
	claimed    []*agentrelation.AgentMessageWait
	claimCalls int
	resumed    []string
	failed     []string
	claimErr   error
	markError  error
}

func (f *callbackWaitFake) ClaimReadyForMessage(_ context.Context, tenantID, runID, messageID, eventID string, limit int) ([]*agentrelation.AgentMessageWait, error) {
	f.claimCalls++
	return f.claimed, f.claimErr
}
func (f *callbackWaitFake) CompleteResume(_ context.Context, _, waitID, _ string) error {
	f.resumed = append(f.resumed, waitID)
	return f.markError
}
func (f *callbackWaitFake) FailResume(_ context.Context, _, waitID, _, cause string, retryable bool) error {
	f.failed = append(f.failed, waitID)
	return f.markError
}

type callbackDispatcherFake struct {
	calls []ResumeAgentMessageInput
	err   error
}

func (f *callbackDispatcherFake) ResumeAgentMessage(_ context.Context, input ResumeAgentMessageInput) error {
	f.calls = append(f.calls, input)
	return f.err
}

func terminalEvent(status string) eventdomain.Envelope {
	return eventdomain.Envelope{ID: "event-1", TenantID: "tenant-a", RunID: "run-1", Type: "agenthub.message." + status + ".v1", Data: map[string]any{"messageId": "child-1", "sourceAgent": "agent-b", "reply": "result", "error": "failed", "guardReason": "budget"}}
}

func TestAgentMessageTerminalHandlerIgnoresUnrelatedEvents(t *testing.T) {
	waits, dispatcher := &callbackWaitFake{}, &callbackDispatcherFake{}
	err := NewAgentMessageTerminalHandler(waits, dispatcher).Handle(context.Background(), eventdomain.Envelope{Type: "agenthub.run.started.v1"})
	require.NoError(t, err)
	require.Zero(t, waits.claimCalls)
}

func TestAgentMessageTerminalHandlerResumesClaimedWaitExactlyOnce(t *testing.T) {
	waits := &callbackWaitFake{claimed: []*agentrelation.AgentMessageWait{{ID: "wait-1", TenantID: "tenant-a", RunID: "run-1", WaitingAgentID: 7, WaitingAgent: "agent-b", RuntimeSessionID: "session-1", ParentMessageID: "parent-1", WaitingMessageID: "child-1", DedupeKey: "wait-dedupe-1"}}}
	dispatcher := &callbackDispatcherFake{}
	err := NewAgentMessageTerminalHandler(waits, dispatcher).Handle(context.Background(), terminalEvent("completed"))
	require.NoError(t, err)
	require.Equal(t, []string{"wait-1"}, waits.resumed)
	require.Len(t, dispatcher.calls, 1)
	require.Equal(t, "result", dispatcher.calls[0].Reply)
	require.Equal(t, "completed", dispatcher.calls[0].TerminalStatus)

	// A real coordinator returns no row after event-1 has claimed/resumed it.
	waits.claimed = nil
	require.NoError(t, NewAgentMessageTerminalHandler(waits, dispatcher).Handle(context.Background(), terminalEvent("completed")))
	require.Len(t, dispatcher.calls, 1)
}

func TestAgentMessageTerminalHandlerFailureIsRetryable(t *testing.T) {
	waits := &callbackWaitFake{claimed: []*agentrelation.AgentMessageWait{{ID: "wait-1", TenantID: "tenant-a", RunID: "run-1", WaitingAgentID: 7, WaitingAgent: "agent-b", RuntimeSessionID: "session-1", WaitingMessageID: "child-1", DedupeKey: "wait-dedupe-1"}}}
	dispatcher := &callbackDispatcherFake{err: errors.New("runtime unavailable")}
	err := NewAgentMessageTerminalHandler(waits, dispatcher).Handle(context.Background(), terminalEvent("failed"))
	require.ErrorContains(t, err, "runtime unavailable")
	require.Equal(t, []string{"wait-1"}, waits.failed)
	require.Empty(t, waits.resumed)
}

func TestAgentMessageTerminalHandlerRejectsCrossRunClaim(t *testing.T) {
	waits := &callbackWaitFake{claimed: []*agentrelation.AgentMessageWait{{ID: "wait-1", TenantID: "tenant-a", RunID: "other-run", WaitingAgentID: 7, WaitingAgent: "agent-b", RuntimeSessionID: "session-1", WaitingMessageID: "child-1"}}}
	err := NewAgentMessageTerminalHandler(waits, &callbackDispatcherFake{}).Handle(context.Background(), terminalEvent("guarded"))
	require.ErrorContains(t, err, "does not belong")
	require.Equal(t, []string{"wait-1"}, waits.failed)
}

func TestAgentMessageTerminalHandlerRequiresMessageIdentity(t *testing.T) {
	e := terminalEvent("completed")
	delete(e.Data, "messageId")
	err := NewAgentMessageTerminalHandler(&callbackWaitFake{}, &callbackDispatcherFake{}).Handle(context.Background(), e)
	require.ErrorIs(t, err, ErrInvalidTerminalMessageEvent)
}
