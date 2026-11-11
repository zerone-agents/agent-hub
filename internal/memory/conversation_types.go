package memory

import "context"

// NativeMessage is a provider record with its original type and provenance.
// Derived content is model output, not a quote-verified Hub ledger Fact.
type NativeMessage struct {
	ID                  string  `json:"id"`
	SourceID            string  `json:"sourceId"`
	SourceSessionID     string  `json:"sourceSessionId"`
	Type                string  `json:"type"`
	Content             string  `json:"content"`
	CreatedAt           string  `json:"createdAt"`
	ValidAt             string  `json:"validAt,omitempty"`
	InvalidAt           string  `json:"invalidAt,omitempty"`
	Status              bool    `json:"status"`
	ForgottenAt         string  `json:"forgottenAt,omitempty"`
	ParentForgottenAt   string  `json:"parentForgottenAt,omitempty"`
	NativeStatus        bool    `json:"nativeStatus"`
	ParentStatus        *bool   `json:"parentStatus,omitempty"`
	StatusChangeAllowed bool    `json:"statusChangeAllowed"`
	ExtractionStatus    string  `json:"extractionStatus,omitempty"`
	ExtractionProgress  float64 `json:"extractionProgress,omitempty"`
	ExtractionError     string  `json:"extractionError,omitempty"`
}

type NativeTurn struct {
	OperationID   string `json:"operationId"`
	SessionID     string `json:"sessionId"`
	TurnID        string `json:"turnId"`
	UserInput     string `json:"userInput"`
	AgentResponse string `json:"agentResponse"`
}

type NativeIngestResult struct {
	Status          string          `json:"status"`
	SourceID        string          `json:"sourceId,omitempty"`
	SourceSessionID string          `json:"sourceSessionId,omitempty"`
	Messages        []NativeMessage `json:"messages"`
}

// NativeConversations consumes the deployed provider's extraction pipeline.
// AddConversation must not retry a POST whose outcome is uncertain.
type NativeConversations interface {
	AddConversation(context.Context, string, NativeTurn) (NativeIngestResult, error)
	FindConversation(context.Context, string, NativeTurn) (NativeIngestResult, error)
	ListMemories(context.Context, string) ([]NativeMessage, error)
	SearchMemories(context.Context, string, string, int) ([]NativeMessage, error)
}

type ConversationInput struct {
	NativeTurn
	ConsentRef    string `json:"consentRef"`
	PolicyVersion string `json:"policyVersion"`
	DataClass     string `json:"dataClass"`
}

type ConversationReceipt struct {
	OperationID string `json:"operationId"`
	TurnID      string `json:"turnId"`
	SessionID   string `json:"sessionId"`
	NativeIngestResult
}

type NativeContext struct {
	RecentSourceIncluded bool            `json:"recentSourceIncluded"`
	CurrentnessNote      string          `json:"currentnessNote"`
	BudgetLimited        bool            `json:"budgetLimited"`
	Messages             []NativeMessage `json:"messages"`
	Status               string          `json:"status"`
}
