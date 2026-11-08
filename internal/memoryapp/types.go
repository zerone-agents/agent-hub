// Package memoryapp provisions application boundaries around native memory.
// It does not extract memories or implement a second memory store.
package memoryapp

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid memory application request")
	ErrNotFound    = errors.New("memory application not found")
	ErrConflict    = errors.New("memory application conflict")
	ErrUnknown     = errors.New("memory application outcome unknown")
	ErrUnavailable = errors.New("memory application service unavailable")
	ErrDisabled    = errors.New("memory application disabled")
	ErrQuota       = errors.New("memory application quota reached")
)

// BankSpec is persisted before any native POST. Name contains an unpredictable
// provisioning marker; it is immutable even when the Hub display name changes.
type BankSpec struct {
	Name           string `json:"name"`
	ChatModel      string `json:"chatModel"`
	EmbeddingModel string `json:"embeddingModel"`
}

type Bank struct {
	ID             string
	Name           string
	ChatModel      string
	EmbeddingModel string
}

// Create sends at most one POST and independently verifies its result. Find is
// read-only, exact-name matched and fails closed on ambiguity or partial lists.
// No match returns ErrNotFound and never authorizes a second Create.
type BankProvisioner interface {
	Create(context.Context, BankSpec) (Bank, error)
	Find(context.Context, BankSpec) (Bank, error)
}

// Owner is trusted server configuration, never a caller-selected tenant.
type Owner struct {
	TenantID        string
	HubTenantID     string
	ChatModel       string
	EmbeddingModel  string
	MaxApplications int
}

type Application struct {
	TenantID       string    `json:"tenantId"`
	ApplicationID  string    `json:"applicationId"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	PolicyVersion  string    `json:"policyVersion"`
	HubTenantID    string    `json:"hubTenantId"`
	Status         string    `json:"status"`
	MemoryID       string    `json:"memoryId,omitempty"`
	MemoryName     string    `json:"memoryName"`
	ChatModel      string    `json:"chatModel"`
	EmbeddingModel string    `json:"embeddingModel"`
	Origin         string    `json:"origin"`
	Version        uint64    `json:"version"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type CreateInput struct {
	OperationID   string `json:"operationId"`
	ApplicationID string `json:"applicationId"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	PolicyVersion string `json:"policyVersion"`
}

// The secret is returned only by the original create operation (including its
// exact idempotent replay), never by ordinary list/detail/status endpoints.
type CreateResult struct {
	Application      Application `json:"application"`
	ApplicationToken string      `json:"applicationToken"`
}

type UpdateInput struct {
	ExpectedVersion uint64  `json:"expectedVersion"`
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

// Binding contains no plaintext credential. It is used by the existing scoped
// memory service factory, so user/lifecycle rules stay in the same consumer.
type Binding struct {
	TenantID      string
	ApplicationID string
	HubTenantID   string
	PolicyVersion string
	MemoryID      string
	Name          string
	Status        string
}

type StaticReservation struct {
	TenantID      string
	ApplicationID string
	HubTenantID   string
	Backend       string
	ContainerID   string
	TokenDigest   string
}
