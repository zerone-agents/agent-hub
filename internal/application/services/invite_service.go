package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	authdom "control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// ErrInviteInvalid covers unknown / expired / used / revoked invites uniformly
// to avoid leaking invite state through distinguishable errors.
var ErrInviteInvalid = errors.New("邀请链接无效或已失效")

const (
	defaultInviteTTLDays = 7
	maxInviteTTLDays     = 30
)

// InviteResult carries the plaintext token, shown exactly once at creation.
// The token is never retrievable afterwards (only its SHA-256 hash is stored).
type InviteResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// InviteService issues, validates, consumes, lists, and revokes one-time
// registration invites. Token hashing reuses HashToken from cli_token_service
// (SHA-256 hex) so the whole auth subsystem shares one digest format.
type InviteService struct {
	db *gorm.DB
}

// NewInviteService constructs an InviteService backed by db.
func NewInviteService(db *gorm.DB) *InviteService {
	return &InviteService{db: db}
}

// Create makes a one-time invite for the given role. ttlDays <= 0 defaults to
// 7 days; ttlDays > 30 is rejected. Only the SHA-256 hash of the plaintext
// token is stored.
func (s *InviteService) Create(role, note string, createdBy uint64, ttlDays int) (*InviteResult, error) {
	if !authdom.IsValidRole(role) {
		return nil, errors.New("非法角色")
	}
	if ttlDays <= 0 {
		ttlDays = defaultInviteTTLDays
	}
	if ttlDays > maxInviteTTLDays {
		return nil, errors.New("邀请有效期不能超过 30 天")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	plaintext := "inv_" + hex.EncodeToString(b)
	expiresAt := time.Now().AddDate(0, 0, ttlDays)
	inv := &authdom.Invite{
		TokenHash: HashToken(plaintext),
		Role:      role,
		Note:      note,
		ExpiresAt: expiresAt,
		CreatedBy: createdBy,
	}
	if err := s.db.Create(inv).Error; err != nil {
		return nil, err
	}
	return &InviteResult{Token: plaintext, ExpiresAt: expiresAt}, nil
}

// Validate returns the invite if usable, else ErrInviteInvalid. Used/revoked
// (deleted) and expired invites all map to the same error.
func (s *InviteService) Validate(token string) (*authdom.Invite, error) {
	var inv authdom.Invite
	if err := s.db.Where("token_hash = ?", HashToken(token)).First(&inv).Error; err != nil {
		return nil, ErrInviteInvalid
	}
	if inv.UsedAt != nil || time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteInvalid
	}
	return &inv, nil
}

// Consume atomically marks the invite used and returns it. Fails (ErrInviteInvalid)
// if already used or concurrently consumed by another request.
func (s *InviteService) Consume(token string) (*authdom.Invite, error) {
	inv, err := s.Validate(token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	res := s.db.Model(&authdom.Invite{}).
		Where("id = ? AND used_at IS NULL", inv.ID).
		Update("used_at", now)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrInviteInvalid
	}
	inv.UsedAt = &now
	return inv, nil
}

// List returns all invites, newest first. Plaintext tokens are never present
// (only hashes are stored); the DTO is the model itself minus the hash column
// (json:"-").
func (s *InviteService) List() ([]*authdom.Invite, error) {
	var invites []*authdom.Invite
	err := s.db.Order("created_at DESC").Find(&invites).Error
	return invites, err
}

// Revoke deletes an unused invite. Used invites cannot be revoked (they are
// already consumed and thus invalid for registration).
func (s *InviteService) Revoke(id uint64) error {
	res := s.db.Where("id = ? AND used_at IS NULL", id).Delete(&authdom.Invite{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("邀请不存在或已被使用")
	}
	return nil
}
