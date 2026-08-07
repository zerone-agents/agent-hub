package services_test

import (
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/auth"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCLITokenTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&auth.CLIToken{})
	require.NoError(t, err)
	return db
}

func TestCLITokenService_Issue(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	result, err := svc.Issue("user-1", "my-macbook", 90)
	require.NoError(t, err)
	assert.Equal(t, 36, len(result.Token), "token should be cli_ prefix + 32 hex chars")
	assert.True(t, len(result.Token) >= 4 && result.Token[:4] == "cli_", "token must start with cli_")
	assert.WithinDuration(t, time.Now().Add(90*24*time.Hour), result.ExpiresAt, 5*time.Second)

	// Verify: hash 落库 + Verify works
	record, err := svc.Verify(result.Token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", record.UserID)
	assert.Equal(t, "my-macbook", record.Name)
	assert.NotNil(t, record.LastUsedAt, "last_used_at should be updated")
}

func TestCLITokenService_Issue_DefaultTTL(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	result, err := svc.Issue("user-1", "x", 0)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(90*24*time.Hour), result.ExpiresAt, 5*time.Second)
}

func TestCLITokenService_Issue_RejectsExcessiveTTL(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	_, err := svc.Issue("user-1", "x", 400)
	assert.Error(t, err)
}

func TestCLITokenService_Verify_RejectsExpired(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	// Issue + manually expire
	result, err := svc.Issue("user-1", "test", 30)
	require.NoError(t, err)
	// Direct DB update to expire it
	db.Model(&auth.CLIToken{}).
		Where("token_hash = ?", services.HashToken(result.Token)).
		Update("expires_at", time.Now().Add(-1*time.Hour))

	_, err = svc.Verify(result.Token)
	assert.Error(t, err, "expired token must be rejected")
}

func TestCLITokenService_Verify_RejectsUnknown(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	_, err := svc.Verify("cli_deadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestCLITokenService_Verify_RejectsBadFormat(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	_, err := svc.Verify("not-a-cli-token")
	assert.Error(t, err)
}

func TestCLITokenService_Revoke(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	result, _ := svc.Issue("user-1", "x", 30)
	record, err := svc.Verify(result.Token)
	require.NoError(t, err)
	require.NotNil(t, record)

	// Can't revoke someone else's
	err = svc.Revoke(record.ID, "user-2")
	assert.Error(t, err)

	// Can revoke own
	err = svc.Revoke(record.ID, "user-1")
	assert.NoError(t, err)

	// Verify fails after revoke
	_, err = svc.Verify(result.Token)
	assert.Error(t, err)
}

func TestCLITokenService_List(t *testing.T) {
	db := setupCLITokenTestDB(t)
	svc := services.NewCLITokenService(db)

	_, _ = svc.Issue("user-1", "first", 30)
	_, _ = svc.Issue("user-1", "second", 30)
	_, _ = svc.Issue("user-2", "other", 30)

	tokens, err := svc.List("user-1")
	require.NoError(t, err)
	assert.Len(t, tokens, 2)

	// Verify TokenDTO is populated (no token hash leak)
	for _, tk := range tokens {
		assert.NotEmpty(t, tk.Name)
		assert.False(t, tk.ExpiresAt.IsZero())
	}
}
