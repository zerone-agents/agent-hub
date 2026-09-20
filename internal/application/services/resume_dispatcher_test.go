package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type resumeRunnerFake struct {
	tenant, agent, session, message string
	calls                           int
	err                             error
}

func (f *resumeRunnerFake) ResumeAccepted(_ context.Context, tenant, agent, session, message string) error {
	f.calls++
	f.tenant, f.agent, f.session, f.message = tenant, agent, session, message
	return f.err
}

type resumeResolverFake struct {
	name    string
	err     error
	tenant  string
	agentID uint64
}

func (f *resumeResolverFake) ResolveResumeAgentName(_ context.Context, tenant string, agentID uint64) (string, error) {
	f.tenant, f.agentID = tenant, agentID
	return f.name, f.err
}

func validCallbackInput() ResumeAgentMessageInput {
	return ResumeAgentMessageInput{
		TenantID: "tenant-a", RunID: "run-1", WaitID: "wait-1", WaitingAgentID: 7,
		AgentName: "event-controlled-name-must-be-ignored", RuntimeSessionID: "session-b", IdempotencyKey: "wait:1",
		ParentMessageID: "parent-1", ChildMessageID: "child-1", TerminalEventID: "event-1",
		TerminalStatus: "completed", Reply: "child result",
	}
}

func TestRuntimeResumeDispatcherUsesAuthorizedAgentAndExistingSession(t *testing.T) {
	runner := &resumeRunnerFake{}
	resolver := &resumeResolverFake{name: "agent-b"}
	err := NewRuntimeResumeDispatcher(runner, resolver).ResumeAgentMessage(context.Background(), validCallbackInput())
	require.NoError(t, err)
	require.Equal(t, 1, runner.calls)
	require.Equal(t, "tenant-a", resolver.tenant)
	require.Equal(t, uint64(7), resolver.agentID)
	require.Equal(t, "agent-b", runner.agent)
	require.Equal(t, "session-b", runner.session)
	require.NotContains(t, runner.message, "event-controlled-name")
	for _, part := range []string{`"schema":"agenthub.resume.v1"`, `"waitId":"wait-1"`, `"parentMessageId":"parent-1"`, `"completedMessageId":"child-1"`, `"runId":"run-1"`, `"terminalEventId":"event-1"`, `"idempotencyKey":"wait:1"`} {
		require.Contains(t, runner.message, part)
	}
}

func TestRuntimeResumeDispatcherFailsClosedBeforeRuntime(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		runner := &resumeRunnerFake{}
		input := validCallbackInput()
		input.RuntimeSessionID = ""
		err := NewRuntimeResumeDispatcher(runner, &resumeResolverFake{name: "agent-b"}).ResumeAgentMessage(context.Background(), input)
		require.ErrorIs(t, err, ErrInvalidResumeRequest)
		require.Zero(t, runner.calls)
	})
	t.Run("cross tenant resolution rejected", func(t *testing.T) {
		runner := &resumeRunnerFake{}
		err := NewRuntimeResumeDispatcher(runner, &resumeResolverFake{err: errors.New("not found")}).ResumeAgentMessage(context.Background(), validCallbackInput())
		require.ErrorContains(t, err, "resolve waiting agent")
		require.Zero(t, runner.calls)
	})
	t.Run("oversized child result", func(t *testing.T) {
		runner := &resumeRunnerFake{}
		input := validCallbackInput()
		input.Reply = strings.Repeat("结", maxResumeResultRunes+1)
		err := NewRuntimeResumeDispatcher(runner, &resumeResolverFake{name: "agent-b"}).ResumeAgentMessage(context.Background(), input)
		require.ErrorIs(t, err, ErrInvalidResumeRequest)
		require.Zero(t, runner.calls)
	})
}

func TestRuntimeResumeDispatcherPropagatesRuntimeFailureForWaitRetry(t *testing.T) {
	runner := &resumeRunnerFake{err: errors.New("runtime unavailable")}
	err := NewRuntimeResumeDispatcher(runner, &resumeResolverFake{name: "agent-b"}).ResumeAgentMessage(context.Background(), validCallbackInput())
	require.ErrorContains(t, err, "runtime unavailable")
	require.Equal(t, 1, runner.calls)
}
