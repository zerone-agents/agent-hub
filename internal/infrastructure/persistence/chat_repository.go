package repository

import (
	"fmt"
	"log"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChatRepository struct {
	db *gorm.DB
}

func NewChatRepository() *ChatRepository {
	return &ChatRepository{
		db: database.GetDB(),
	}
}

type ConflictInfo struct {
	SessionID       string `json:"session_id"`
	ClientUpdatedAt string `json:"client_updated_at"`
	ServerUpdatedAt string `json:"server_updated_at"`
	Resolution      string `json:"resolution"`
}

type PushResult struct {
	SyncedSessions  int
	SkippedSessions int
	SyncedMessages  int
	Conflicts       []ConflictInfo
}

func (r *ChatRepository) PushSessions(userID string, sessions []*chat.Session, messagesPerSession [][]*chat.Message) (*PushResult, error) {
	result := &PushResult{}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		for i, sess := range sessions {
			sess.UserID = userID

			var existing chat.Session
			err := tx.Where("user_id = ? AND id = ?", userID, sess.ID).First(&existing).Error

			if err == nil {
				if !existing.UpdatedAt.Before(sess.UpdatedAt) {
					result.SkippedSessions++
					result.Conflicts = append(result.Conflicts, ConflictInfo{
						SessionID:       sess.ID,
						ClientUpdatedAt: sess.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
						ServerUpdatedAt: existing.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
						Resolution:      "skipped: server_data_is_newer_or_equal",
					})
					continue
				}
			} else if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("failed to query session %s: %w", sess.ID, err)
			}

			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"title", "created_at", "updated_at", "model", "system_prompt",
					"status", "mode", "provider_id", "agent_id", "permission_profile",
					"hidden", "extra_directories", "is_user_bound",
					"user_name", "display_name",
				}),
			}).Create(sess).Error; err != nil {
				return fmt.Errorf("failed to upsert session %s: %w", sess.ID, err)
			}

			if err := tx.Where("user_id = ? AND session_id = ?", userID, sess.ID).Delete(&chat.Message{}).Error; err != nil {
				return fmt.Errorf("failed to delete messages for session %s: %w", sess.ID, err)
			}

			sessionMessages := messagesPerSession[i]
			for _, msg := range sessionMessages {
				msg.UserID = userID
				msg.SessionID = sess.ID
			}
			if len(sessionMessages) > 0 {
				if err := tx.CreateInBatches(sessionMessages, 100).Error; err != nil {
					return fmt.Errorf("failed to insert messages for session %s: %w", sess.ID, err)
				}
			}

			result.SyncedSessions++
			result.SyncedMessages += len(sessionMessages)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	log.Printf("[Chat] user=%s pushed=%d skipped=%d messages=%d conflicts=%d",
		userID, result.SyncedSessions, result.SkippedSessions, result.SyncedMessages, len(result.Conflicts))

	return result, nil
}

func (r *ChatRepository) ListSessions(page, pageSize int) ([]*chat.Session, int64, error) {
	var total int64
	if err := r.db.Model(&chat.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var sessions []*chat.Session
	offset := (page - 1) * pageSize
	err := r.db.Order("updated_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&sessions).Error
	return sessions, total, err
}

func (r *ChatRepository) GetSession(sessionID string) (*chat.Session, error) {
	var sess chat.Session
	err := r.db.Where("id = ?", sessionID).First(&sess).Error
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (r *ChatRepository) ListMessages(sessionID string, page, pageSize int) ([]*chat.Message, int64, error) {
	var total int64
	if err := r.db.Model(&chat.Message{}).Where("session_id = ?", sessionID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var messages []*chat.Message
	offset := (page - 1) * pageSize
	err := r.db.Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Offset(offset).Limit(pageSize).
		Find(&messages).Error
	return messages, total, err
}

func (r *ChatRepository) DeleteSession(sessionID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&chat.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", sessionID).Delete(&chat.Session{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// ListSessionsByAgentAndUser returns sessions for a specific (agent, user) pair.
// When source is non-empty, only sessions with that source are returned.
func (r *ChatRepository) ListSessionsByAgentAndUser(agentID, userID, source string, page, pageSize int) ([]*chat.Session, int64, error) {
	var total int64
	q := r.db.Model(&chat.Session{}).Where("agent_id = ? AND user_id = ?", agentID, userID)
	if source != "" {
		q = q.Where("source = ?", source)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var sessions []*chat.Session
	offset := (page - 1) * pageSize
	err := q.Order("updated_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&sessions).Error
	return sessions, total, err
}

// GetSessionForUser returns a session only if it belongs to the given user.
// Returns gorm.ErrRecordNotFound if the session does not exist or is owned by another user.
func (r *ChatRepository) GetSessionForUser(sessionID, userID string) (*chat.Session, error) {
	var sess chat.Session
	err := r.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&sess).Error
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// CreateSession inserts a new session.
func (r *ChatRepository) CreateSession(sess *chat.Session) error {
	return r.db.Create(sess).Error
}

// CreateMessage inserts a new message.
func (r *ChatRepository) CreateMessage(msg *chat.Message) error {
	return r.db.Create(msg).Error
}

// UpdateSessionRuntimeSessionID updates the runtime SDK session id bound to a
// control-panel chat session. Empty values are ignored to avoid overwriting an
// already-bound id with a blank string.
func (r *ChatRepository) UpdateSessionRuntimeSessionID(sessionID, runtimeSessionID string) error {
	if runtimeSessionID == "" {
		return nil
	}
	return r.db.Model(&chat.Session{}).
		Where("id = ?", sessionID).
		Update("runtime_session_id", runtimeSessionID).Error
}

// UpdateSessionTitle updates the title of a chat session. The title is only
// overwritten when the current title is empty, preserving any user-edited title.
func (r *ChatRepository) UpdateSessionTitle(sessionID, title string) error {
	return r.db.Model(&chat.Session{}).
		Where("id = ? AND (title IS NULL OR title = '')", sessionID).
		Update("title", title).Error
}
