package repository

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

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

// PushSessions 同步客户端会话到服务端。安全性前提：一个 user_id 只映射一个
// 租户（casdoor 单组织成员模型），因此 PK (user_id, id) 不含 tenant_id 也不会
// 出现同 PK 跨租户冲突——该前提若被多组织成员模型打破，需同步调整 PK 设计。
func (r *ChatRepository) PushSessions(tenantID, userID string, sessions []*chat.Session, messagesPerSession [][]*chat.Message) (*PushResult, error) {
	result := &PushResult{}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		for i, sess := range sessions {
			sess.UserID = userID
			sess.TenantID = tenantID

			// v2 写入隔离（issue #232 H2）：已 claim / 已同步 / 已删除的同步键
			// 拒绝 v1 写入（旧 App 回写保护）。锁序与 claim/apply 一致（状态行
			// → 会话行）：MySQL 锁定读会等待未提交的占位插入，消除检查后写入
			// 竞态；SQLite 靠单写者序列化。
			var syncRow chat.SessionSyncState
			syncQ := tx.Where("tenant_id = ? AND user_id = ? AND session_id = ?", tenantID, userID, sess.ID)
			if tx.Dialector.Name() == "mysql" {
				syncQ = syncQ.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			syncErr := syncQ.First(&syncRow).Error
			if syncErr == nil {
				result.SkippedSessions++
				result.Conflicts = append(result.Conflicts, ConflictInfo{
					SessionID:  sess.ID,
					Resolution: "skipped: session_managed_by_sync_v2",
				})
				continue
			}
			if !errors.Is(syncErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to query sync state for session %s: %w", sess.ID, syncErr)
			}

			var existing chat.Session
			err := TenantOwned(tx, tenantID).Where("user_id = ? AND id = ?", userID, sess.ID).First(&existing).Error

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

			if err := TenantOwned(tx, tenantID).Where("user_id = ? AND session_id = ?", userID, sess.ID).Delete(&chat.Message{}).Error; err != nil {
				return fmt.Errorf("failed to delete messages for session %s: %w", sess.ID, err)
			}

			sessionMessages := messagesPerSession[i]
			for _, msg := range sessionMessages {
				msg.UserID = userID
				msg.TenantID = tenantID
				msg.SessionID = sess.ID
			}
			normalizeMessageOrder(sessionMessages)
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

	log.Printf("[Chat] tenant=%s user=%s pushed=%d skipped=%d messages=%d conflicts=%d",
		tenantID, userID, result.SyncedSessions, result.SkippedSessions, result.SyncedMessages, len(result.Conflicts))

	return result, nil
}

// normalizeMessageOrder 保证消息 created_at 严格递增。push 调用方（如
// agent-runtime 的快照推送）可能给同一会话的所有消息相同的 created_at
// （transcript 无逐条时间戳），而读路径按 created_at ASC 排序——相同
// 时间戳下数据库返回顺序未定义（生产曾出现 assistant 排在 user 之前）。
// 这里以数组顺序为时序真源，必要时把时间戳顶到前一条 +1ms；重放同一
// 载荷（幂等重推）产生相同结果。
func normalizeMessageOrder(msgs []*chat.Message) {
	for i := 1; i < len(msgs); i++ {
		if !msgs[i].CreatedAt.After(msgs[i-1].CreatedAt) {
			msgs[i].CreatedAt = msgs[i-1].CreatedAt.Add(time.Millisecond)
		}
	}
}

// ChatSessionFilter 会话列表的服务端筛选条件（spec 2026-10-14-cli-chat-fetch §5）。
// 字符串字段一律存**调用方原始值**：LIKE 的转义与 % 包裹只在 applyChatSessionFilter
// 内做一次，两层各转义一次会让 "%" 变成 "\\%" 这种字面量错配（spec §5 契约约束）。
type ChatSessionFilter struct {
	TenantID        string     // 强制租户范围，永不接受客户端输入
	ScopeUserID     string     // member 的强制自我范围（admin 传空串）
	AgentID         string     // 精确
	User            string     // user_name / user_id 两列模糊，列集与审计端点严格一致
	Keyword         string     // 仅命中标题
	MessageContains string     // 跨表正文扫描，显式才走（成本见下方 EXISTS 注释）
	From, To        *time.Time // 闭区间，作用于 updated_at（最后活动时间）
	Page, PageSize  int
}

// escapeLike 把用户输入里的 LIKE 元字符变成字面量。转义符必须用 `!` 而非反斜杠：
// MySQL 的 SQL 文本里 `ESCAPE '\'` 会让字符串字面量不闭合（ERROR 1064，曾为 PR #223
// 评审阻塞项）；`ESCAPE '\\'` 又被 sqlite 解析成两字符（"must be a single character"）。
// 两引擎不存在通用的反斜杠写法；`!` 在两侧都是单字符、且不受 NO_BACKSLASH_ESCAPES
// 影响。转义只在此处做一次（spec §5 契约约束）。
func escapeLike(s string) string {
	return strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`).Replace(s)
}

func likePattern(s string) string { return "%" + escapeLike(s) + "%" }

// applyChatSessionFilter 组装筛选 WHERE。COUNT 与 Find 必须共用它（同
// audit_repository.go 的既有不变量）：两条语句 WHERE 不一致会让 total 与
// 实际分页结果对不上。
func (r *ChatRepository) applyChatSessionFilter(q *gorm.DB, f ChatSessionFilter) *gorm.DB {
	if f.ScopeUserID != "" {
		q = q.Where("user_id = ?", f.ScopeUserID)
	}
	if f.AgentID != "" {
		q = q.Where("agent_id = ?", f.AgentID)
	}
	if f.User != "" {
		p := likePattern(f.User)
		q = q.Where("(user_name LIKE ? ESCAPE '!' OR user_id LIKE ? ESCAPE '!')", p, p)
	}
	if f.Keyword != "" {
		q = q.Where("title LIKE ? ESCAPE '!'", likePattern(f.Keyword))
	}
	if f.From != nil {
		q = q.Where("updated_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("updated_at <= ?", *f.To)
	}
	if f.MessageContains != "" {
		// 成本注记（spec §4.4，勿改写）：cloud_messages 主键是 (user_id, id, session_id)，
		// 可作 ref 访问的前缀只有 user_id；session_id 排第三，不构成可用前缀。
		// 所以这里是「扫该用户的全部消息再按会话过滤」，不是「按会话索引定位」。
		// m.tenant_id 条件不是性能手段（一次表引用只用一个索引），其理由是纵深防御
		// 并与既有 ListMessages 的 TenantOwned 口径一致。
		q = q.Where(
			"EXISTS (SELECT 1 FROM cloud_messages m WHERE m.user_id = cloud_sessions.user_id"+
				" AND m.session_id = cloud_sessions.id AND m.tenant_id = ?"+
				" AND m.content LIKE ? ESCAPE '!')",
			f.TenantID, likePattern(f.MessageContains))
	}
	return q
}

// ListSessionsFiltered 是会话列表筛选的唯一实现。
func (r *ChatRepository) ListSessionsFiltered(f ChatSessionFilter) ([]*chat.Session, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	var total int64
	if err := r.applyChatSessionFilter(TenantOwned(r.db.Model(&chat.Session{}), f.TenantID), f).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var sessions []*chat.Session
	err := r.applyChatSessionFilter(TenantOwned(r.db.Model(&chat.Session{}), f.TenantID), f).
		Order("updated_at DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Find(&sessions).Error
	return sessions, total, err
}

// ListSessions 保留为薄包装：既有调用方与既有测试不破。
func (r *ChatRepository) ListSessions(tenantID string, page, pageSize int) ([]*chat.Session, int64, error) {
	return r.ListSessionsFiltered(ChatSessionFilter{TenantID: tenantID, Page: page, PageSize: pageSize})
}

// ListSessionsByUser 同上，userID 作为强制范围。
func (r *ChatRepository) ListSessionsByUser(tenantID, userID string, page, pageSize int) ([]*chat.Session, int64, error) {
	return r.ListSessionsFiltered(ChatSessionFilter{TenantID: tenantID, ScopeUserID: userID, Page: page, PageSize: pageSize})
}

func (r *ChatRepository) GetSession(tenantID, sessionID string) (*chat.Session, error) {
	var sess chat.Session
	err := TenantOwned(r.db, tenantID).Where("id = ?", sessionID).First(&sess).Error
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (r *ChatRepository) ListMessages(tenantID, sessionID string, page, pageSize int) ([]*chat.Message, int64, error) {
	var total int64
	if err := TenantOwned(r.db.Model(&chat.Message{}), tenantID).Where("session_id = ?", sessionID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var messages []*chat.Message
	offset := (page - 1) * pageSize
	// v2 同步会话按 sync_ordinal 稳定排序（同 timestamp / created_at 与序
	// 相逆时仍按快照序）；NULL（v1 legacy 会话全 NULL）在 ASC 下排前且互
	// 相回退 created_at 排序——两类会话不会混排（claim 后 v1 写入被拒）。
	err := TenantOwned(r.db, tenantID).Where("session_id = ?", sessionID).
		Order("sync_ordinal ASC").
		Order("created_at ASC").
		Offset(offset).Limit(pageSize).
		Find(&messages).Error
	return messages, total, err
}

// DeleteSession removes a session with its messages and upload records in a
// single transaction (issue #94 review R2 F3): upload records are the
// attachment authorization anchors — they must not outlive their session,
// and a partial failure must never leave records behind for a deleted
// session.
func (r *ChatRepository) DeleteSession(tenantID, sessionID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := TenantOwned(tx, tenantID).Where("session_id = ?", sessionID).Delete(&chat.UploadRecord{}).Error; err != nil {
			return err
		}
		if err := TenantOwned(tx, tenantID).Where("session_id = ?", sessionID).Delete(&chat.Message{}).Error; err != nil {
			return err
		}
		if err := TenantOwned(tx, tenantID).Where("id = ?", sessionID).Delete(&chat.Session{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// ListSessionsByAgentAndUser returns sessions for a specific (agent, user) pair.
// When source is non-empty, only sessions with that source are returned.
func (r *ChatRepository) ListSessionsByAgentAndUser(tenantID, agentID, userID, source string, page, pageSize int) ([]*chat.Session, int64, error) {
	var total int64
	q := TenantOwned(r.db.Model(&chat.Session{}), tenantID).Where("agent_id = ? AND user_id = ?", agentID, userID)
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
func (r *ChatRepository) GetSessionForUser(tenantID, sessionID, userID string) (*chat.Session, error) {
	var sess chat.Session
	err := TenantOwned(r.db, tenantID).Where("id = ? AND user_id = ?", sessionID, userID).First(&sess).Error
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// CreateSession inserts a new session. The tenant is always stamped from the
// tenantID argument — the caller-provided model value is not trusted.
// syncManagedErr：会话是否由 v2 同步管理（存在任意 sync-state 行即受管，
// 返回 chat.ErrSyncManagedSession）。UpdateSessionTitle / DeleteMessageByID
// 无 user 参数——v1 模型下 user 与 tenant 1:1（见 PushSessions 注释），按
// tenant+session 判定即可；CreateMessage/CreateSession 携带 user 时精确到
// user（跨用户同名 session id 不误伤）。
func syncManagedErr(tx *gorm.DB, tenantID, userID, sessionID string) error {
	q := tx.Where("tenant_id = ? AND session_id = ?", tenantID, sessionID)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	var row chat.SessionSyncState
	if err := q.First(&row).Error; err == nil {
		return chat.ErrSyncManagedSession
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func (r *ChatRepository) CreateSession(tenantID string, sess *chat.Session) error {
	if err := syncManagedErr(r.db, tenantID, sess.UserID, sess.ID); err != nil {
		return err
	}
	sess.TenantID = tenantID
	return r.db.Create(sess).Error
}

// CreateMessage inserts a new message. The tenant is always stamped from the
// tenantID argument — the caller-provided model value is not trusted.
func (r *ChatRepository) CreateMessage(tenantID string, msg *chat.Message) error {
	if err := syncManagedErr(r.db, tenantID, msg.UserID, msg.SessionID); err != nil {
		return err
	}
	msg.TenantID = tenantID
	return r.db.Create(msg).Error
}

// UpdateSessionRuntimeSessionID updates the runtime SDK session id bound to a
// control-panel chat session. Empty values are ignored to avoid overwriting an
// already-bound id with a blank string.
func (r *ChatRepository) UpdateSessionRuntimeSessionID(tenantID, sessionID, runtimeSessionID string) error {
	if runtimeSessionID == "" {
		return nil
	}
	return TenantOwned(r.db.Model(&chat.Session{}), tenantID).
		Where("id = ?", sessionID).
		Update("runtime_session_id", runtimeSessionID).Error
}

// UpdateSessionTitle updates the title of a chat session. The title is only
// overwritten when the current title is empty, preserving any user-edited title.
func (r *ChatRepository) UpdateSessionTitle(tenantID, sessionID, title string) error {
	if err := syncManagedErr(r.db, tenantID, "", sessionID); err != nil {
		return err
	}
	return TenantOwned(r.db.Model(&chat.Session{}), tenantID).
		Where("id = ? AND (title IS NULL OR title = '')", sessionID).
		Update("title", title).Error
}

// CreateUploadRecords batch-inserts server-side upload records in a single
// transaction (issue #94 review R2 F4): an upload response persists all of
// its descriptors or none — the runtime has already issued the ids/files, so
// a partial batch would leave files no record authorizes. The tenant is
// always stamped from the tenantID argument — the caller-provided model
// values are not trusted.
func (r *ChatRepository) CreateUploadRecords(tenantID string, records []*chat.UploadRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, rec := range records {
			rec.TenantID = tenantID
		}
		return tx.Create(&records).Error
	})
}

// GetUploadRecord returns the upload record for (session, id), tenant-scoped.
// gorm.ErrRecordNotFound when the id was never issued by an upload in the
// session — i.e. a client-forged descriptor.
func (r *ChatRepository) GetUploadRecord(tenantID, sessionID, id string) (*chat.UploadRecord, error) {
	var rec chat.UploadRecord
	err := TenantOwned(r.db, tenantID).Where("session_id = ? AND id = ?", sessionID, id).First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// HasUploadRecordPath reports whether path was registered by a server-side
// upload record of the session created in the given runtime container
// generation (exact container_id equality, zero tolerance — records from
// previous container generations no longer authorize after a recreate;
// issue #94 review R3).
func (r *ChatRepository) HasUploadRecordPath(tenantID, sessionID, path, containerID string) (bool, error) {
	var count int64
	err := TenantOwned(r.db.Model(&chat.UploadRecord{}), tenantID).
		Where("session_id = ? AND path = ? AND container_id = ?", sessionID, path, containerID).Count(&count).Error
	return count > 0, err
}

// DeleteMessageByID deletes one message of a session (used to roll back the
// optimistic user-message persist when the runtime rejects the run).
func (r *ChatRepository) DeleteMessageByID(tenantID, sessionID, messageID string) error {
	if err := syncManagedErr(r.db, tenantID, "", sessionID); err != nil {
		return err
	}
	return TenantOwned(r.db, tenantID).Where("session_id = ? AND id = ?", sessionID, messageID).Delete(&chat.Message{}).Error
}
