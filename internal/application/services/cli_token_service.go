package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// CLITokenService issues, verifies, lists, and revokes opaque CLI tokens.
// Tokens are stored as SHA-256 hashes; plaintext is returned exactly once at issue time.
type CLITokenService struct {
	db *gorm.DB
}

func NewCLITokenService(db *gorm.DB) *CLITokenService {
	return &CLITokenService{db: db}
}

// IssueResult is returned once at token creation. Token is the plaintext form,
// shown only this once.
type IssueResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// TokenDTO is the safe projection returned by List (no token hash).
type TokenDTO struct {
	ID         uint64     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
}

const (
	defaultTTLDays = 90
	maxTTLDays     = 365
)

// Issue creates a new CLI token for userID with the given friendly name.
// ttlDays <= 0 falls back to default; ttlDays > 365 is rejected.
func (s *CLITokenService) Issue(userID string, name string, ttlDays int) (*IssueResult, error) {
	if userID == "" {
		return nil, errors.New("userID is required")
	}
	if name == "" {
		return nil, errors.New("name is required")
	}
	if ttlDays <= 0 {
		ttlDays = defaultTTLDays
	}
	if ttlDays > maxTTLDays {
		return nil, fmt.Errorf("TTL 不能超过 %d 天", maxTTLDays)
	}
	expiresAt := time.Now().AddDate(0, 0, ttlDays)

	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("generate random: %w", err)
	}
	plaintext := "cli_" + hex.EncodeToString(randomBytes)
	hash := HashToken(plaintext)

	record := &auth.CLIToken{
		UserID:    userID,
		Name:      name,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	return &IssueResult{Token: plaintext, ExpiresAt: expiresAt}, nil
}

// Verify looks up a CLI token by its plaintext, updates last_used_at, and returns
// the record. Returns an error if the token is malformed, unknown, or expired.
func (s *CLITokenService) Verify(plaintext string) (*auth.CLIToken, error) {
	if len(plaintext) < 4 || plaintext[:4] != "cli_" {
		return nil, errors.New("invalid token format")
	}
	hash := HashToken(plaintext)
	var record auth.CLIToken
	err := s.db.Where("token_hash = ?", hash).First(&record).Error
	if err != nil {
		return nil, errors.New("token not found")
	}
	if time.Now().After(record.ExpiresAt) {
		return nil, errors.New("token expired")
	}
	now := time.Now()
	s.db.Model(&auth.CLIToken{}).Where("id = ?", record.ID).Update("last_used_at", now)
	record.LastUsedAt = &now
	return &record, nil
}

// List returns all tokens for userID ordered by creation descending.
func (s *CLITokenService) List(userID string) ([]*TokenDTO, error) {
	var records []*auth.CLIToken
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	dtos := make([]*TokenDTO, 0, len(records))
	for _, r := range records {
		dtos = append(dtos, &TokenDTO{
			ID:         r.ID,
			Name:       r.Name,
			CreatedAt:  r.CreatedAt,
			LastUsedAt: r.LastUsedAt,
			ExpiresAt:  r.ExpiresAt,
		})
	}
	return dtos, nil
}

// Revoke deletes a token. The id must belong to userID, otherwise an error is
// returned (callers must not be able to probe other users' token ids).
func (s *CLITokenService) Revoke(id uint64, userID string) error {
	result := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&auth.CLIToken{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("token not found or not owned by user")
	}
	return nil
}

// HashToken returns the SHA-256 hex digest of plaintext. Exported so tests and
// the middleware can mirror the hashing without re-implementing it.
func HashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}
