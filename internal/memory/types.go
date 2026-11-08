package memory

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("memory not found")
	ErrConflict     = errors.New("memory version or operation conflict")
	ErrForbidden    = errors.New("memory scope is not active")
	ErrInvalid      = errors.New("invalid memory request")
	ErrIndexUnknown = errors.New("memory index outcome unknown")
	ErrUnsupported  = errors.New("memory operation unsupported by this backend")
)

type Scope struct {
	TenantID      string `json:"tenantId"`
	ApplicationID string `json:"applicationId"`
	UserID        string `json:"userId"`
	LifecycleID   string `json:"accountLifecycleId"`
}

type Subject struct {
	Scope    Scope  `json:"scope"`
	State    string `json:"state"`
	Revision uint64 `json:"revision"`
}

type SubjectList struct {
	Subjects          []Subject `json:"subjects"`
	MissingScopeCount int       `json:"missingScopeCount"`
	Limited           bool      `json:"limited"`
	Limit             int       `json:"limit"`
	Note              string    `json:"note,omitempty"`
}

type Hit struct {
	ChunkID    string
	DocumentID string
	Content    string
	Score      float64
}

type Index interface {
	EnsureDocument(context.Context, string, string, bool) (string, error)
	Put(context.Context, string, string, string) (string, error)
	Remove(context.Context, string, string, string) error
	DropDocument(context.Context, string, string) error
	Search(context.Context, string, string, string, int) ([]Hit, error)
	Find(context.Context, string, string, string) (string, error)
}

type KeyWrapper interface {
	Wrap(context.Context, []byte, []byte) ([]byte, error)
	Unwrap(context.Context, []byte, []byte) ([]byte, error)
}

type CandidateInput struct {
	ReplaceFactID   string `json:"replaceFactId,omitempty"`
	ExpectedVersion uint64 `json:"expectedVersion,omitempty"`
	DataClass       string `json:"dataClass"`
	Source          string `json:"source"`
	OperationID     string `json:"operationId"`
	TurnID          string `json:"turnId"`
	SourceMessageID string `json:"sourceMessageId"`
	UserText        string `json:"userText"`
	Quote           string `json:"quote"`
	Subject         string `json:"subject"`
	Temporal        string `json:"temporal"`
	Category        string `json:"category"`
	Risk            bool   `json:"risk"`
	Sensitive       bool   `json:"sensitive"`
	ConsentRef      string `json:"consentRef"`
	PolicyVersion   string `json:"policyVersion"`
}

type Fact struct {
	ID         string    `json:"factId"`
	Version    uint64    `json:"version"`
	Content    string    `json:"content"`
	Subject    string    `json:"subject"`
	Temporal   string    `json:"temporal"`
	Category   string    `json:"category"`
	Status     string    `json:"status"`
	SyncStatus string    `json:"syncStatus"`
	ConsentRef string    `json:"consentRef,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Candidate struct {
	RejectionReason string `json:"rejectionReason,omitempty"`
	ReplaceFactID   string `json:"replaceFactId,omitempty"`
	ExpectedVersion uint64 `json:"expectedVersion,omitempty"`
	ID              string `json:"candidateId"`
	Version         uint64 `json:"version"`
	Quote           string `json:"quote"`
	Subject         string `json:"subject"`
	Temporal        string `json:"temporal"`
	Category        string `json:"category"`
	SourceMessageID string `json:"sourceMessageId"`
	TurnID          string `json:"turnId"`
	Status          string `json:"status"`
}

type PublishItem struct {
	CandidateID     string `json:"candidateId"`
	ExpectedVersion uint64 `json:"expectedVersion"`
	Content         string `json:"content"`
}

type PublishInput struct {
	OperationID    string        `json:"operationId"`
	TaskID         string        `json:"taskId"`
	Items          []PublishItem `json:"items"`
	Profile        string        `json:"profile,omitempty"`
	ProfileFactIDs []string      `json:"profileFactIds,omitempty"`
}

type Task struct {
	ID         string      `json:"taskId"`
	Candidates []Candidate `json:"candidates"`
}

type Context struct {
	Version uint64 `json:"contextVersion"`
	Facts   []Fact `json:"facts"`
	Profile string `json:"profile,omitempty"`
	Status  string `json:"status"`
}

type Operation struct {
	ID              string `json:"operationId"`
	Status          string `json:"status"`
	StoppedSupply   bool   `json:"stoppedSupply"`
	PhysicalCleanup bool   `json:"physicalCleanup"`
	CleanupScope    string `json:"cleanupScope"`
	LedgerRetained  bool   `json:"ledgerRetained"`
}

type Config struct {
	Backend       string
	TenantID      string
	ApplicationID string
	PolicyVersion string
	DatasetID     string
	SyntheticOnly bool
	Wrapper       KeyWrapper
	Index         Index
	Native        NativeConversations
}
