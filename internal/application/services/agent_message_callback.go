package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"control-panel/internal/domain/agentrelation"
	eventdomain "control-panel/internal/domain/event"
)

var ErrInvalidTerminalMessageEvent = errors.New("invalid terminal agent message event")

// AgentMessageWaitCoordinator is deliberately narrower than the concrete wait
// service. ClaimReadyForMessage must be atomic and idempotent for eventID: a
// delivered event may be retried, but a resumed wait must never be returned a
// second time. Expired/cancelled/budget-exhausted waits are closed by the
// coordinator rather than returned to the dispatcher.
type AgentMessageWaitCoordinator interface {
	ClaimReadyForMessage(ctx context.Context, tenantID, runID, waitingMessageID, eventID string, limit int) ([]*agentrelation.AgentMessageWait, error)
	CompleteResume(ctx context.Context, tenantID, waitID, eventID string) error
	FailResume(ctx context.Context, tenantID, waitID, eventID, cause string, retryable bool) error
}

var _ AgentMessageWaitCoordinator = (*AgentMessageWaitService)(nil)

type ResumeAgentMessageInput struct {
	TenantID         string
	RunID            string
	WaitID           string
	WaitingAgentID   uint64
	AgentName        string
	RuntimeSessionID string
	IdempotencyKey   string
	ParentMessageID  string
	ChildMessageID   string
	TerminalEventID  string
	TerminalStatus   string
	Reply            string
	Error            string
	GuardReason      string
}

// AgentMessageResumeDispatcher performs the runtime-specific wake-up. Runtime
// HTTP details stay outside the event consumer so the latter remains testable.
type AgentMessageResumeDispatcher interface {
	ResumeAgentMessage(context.Context, ResumeAgentMessageInput) error
}

// AgentMessageTerminalHandler converts durable terminal message facts into
// durable callback resumptions. It is suitable for direct registration as an
// EventHandler in EventService.RunWorkers.
type AgentMessageTerminalHandler struct {
	waits      AgentMessageWaitCoordinator
	dispatcher AgentMessageResumeDispatcher
}

func NewAgentMessageTerminalHandler(waits AgentMessageWaitCoordinator, dispatcher AgentMessageResumeDispatcher) *AgentMessageTerminalHandler {
	return &AgentMessageTerminalHandler{waits: waits, dispatcher: dispatcher}
}

func (h *AgentMessageTerminalHandler) Handle(ctx context.Context, envelope eventdomain.Envelope) error {
	status, terminal := terminalAgentMessageStatus(envelope.Type)
	if !terminal {
		return nil
	}
	if h == nil || h.waits == nil || h.dispatcher == nil {
		return errors.New("agent message callback handler is not configured")
	}
	messageID := eventString(envelope.Data, "messageId")
	if strings.TrimSpace(envelope.TenantID) == "" || strings.TrimSpace(envelope.RunID) == "" || messageID == "" || strings.TrimSpace(envelope.ID) == "" {
		return fmt.Errorf("%w: tenant, run, event and message ids are required", ErrInvalidTerminalMessageEvent)
	}

	waits, err := h.waits.ClaimReadyForMessage(ctx, envelope.TenantID, envelope.RunID, messageID, envelope.ID, 100)
	if err != nil {
		return fmt.Errorf("claim message callback waits: %w", err)
	}
	for _, wait := range waits {
		// ClaimTerminal is the authority for these invariants, but checking again
		// here prevents an implementation bug from crossing a tenant/run boundary.
		if wait.TenantID != envelope.TenantID || wait.RunID != envelope.RunID || wait.WaitingMessageID != messageID || wait.ID == "" || wait.WaitingAgentID == 0 || strings.TrimSpace(wait.WaitingAgent) == "" || strings.TrimSpace(wait.RuntimeSessionID) == "" {
			err := fmt.Errorf("claimed wait %q does not belong to terminal message", wait.ID)
			_ = h.waits.FailResume(ctx, envelope.TenantID, wait.ID, envelope.ID, err.Error(), false)
			return err
		}
		input := ResumeAgentMessageInput{
			TenantID: envelope.TenantID, RunID: envelope.RunID, WaitID: wait.ID,
			WaitingAgentID: wait.WaitingAgentID, AgentName: wait.WaitingAgent, RuntimeSessionID: wait.RuntimeSessionID, IdempotencyKey: wait.DedupeKey, ParentMessageID: wait.ParentMessageID,
			ChildMessageID: messageID, TerminalEventID: envelope.ID, TerminalStatus: status,
			Reply: eventString(envelope.Data, "reply"), Error: eventString(envelope.Data, "error"),
			GuardReason: eventString(envelope.Data, "guardReason"),
		}
		ensureTerminalResult(&input)
		if err := h.dispatcher.ResumeAgentMessage(ctx, input); err != nil {
			if persistErr := h.waits.FailResume(ctx, envelope.TenantID, wait.ID, envelope.ID, err.Error(), true); persistErr != nil {
				return errors.Join(fmt.Errorf("resume wait %s: %w", wait.ID, err), fmt.Errorf("record resume failure: %w", persistErr))
			}
			return fmt.Errorf("resume wait %s: %w", wait.ID, err)
		}
		if err := h.waits.CompleteResume(ctx, envelope.TenantID, wait.ID, envelope.ID); err != nil {
			return fmt.Errorf("mark wait %s resumed: %w", wait.ID, err)
		}
	}
	return nil
}

func ensureTerminalResult(input *ResumeAgentMessageInput) {
	if input == nil {
		return
	}
	switch input.TerminalStatus {
	case "completed":
		if input.Reply == "" {
			input.Reply = "子任务已完成，但没有返回正文。"
		}
	case "failed":
		if input.Error == "" {
			input.Error = "子任务执行失败。"
		}
	case "guarded":
		if input.GuardReason == "" {
			input.GuardReason = "子任务被平台安全策略拦截。"
		}
	}
}

func terminalAgentMessageStatus(eventType string) (string, bool) {
	switch strings.TrimSpace(eventType) {
	case "agenthub.message.completed.v1":
		return "completed", true
	case "agenthub.message.failed.v1":
		return "failed", true
	case "agenthub.message.guarded.v1":
		return "guarded", true
	default:
		return "", false
	}
}

func eventString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}
