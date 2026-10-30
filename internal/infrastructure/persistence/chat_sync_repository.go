package repository

// v2 同步状态机持久化（issue #232 H2）：claim / apply / getstate。
//
// 锁序恒定：session_sync_states 行 → cloud_sessions/cloud_messages 行。
//   - claim：先 INSERT 唯一占位（唯一索引仲裁首次双写）或 FOR UPDATE 锁
//     既有行，再锁读同键 v1 会话做 empty/legacy 分类；
//   - v1 PushSessions：先锁定读状态行（存在即拒绝），再写会话；
//   - apply：锁状态行做版本比较，胜者在同一事务内替换展示内容。
// MySQL 下 v1 的锁定读会等待未提交的占位插入、claim 的会话读会等待未提交
// 的 v1 会话写——「v1 写入成功且状态为 claimed-empty」的竞态在两个方向都
// 被关闭。SQLite 靠单写者序列化。不使用进程内 mutex（多实例前提）。

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"control-panel/internal/domain/chat"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChatSyncRepository struct {
	db *gorm.DB
}

func NewChatSyncRepository(db *gorm.DB) *ChatSyncRepository {
	return &ChatSyncRepository{db: db}
}

// lockSyncRow 锁定并返回同步键状态行；未找到返回 gorm.ErrRecordNotFound。
func lockSyncRow(tx *gorm.DB, owner chat.Owner, sessionID string) (*chat.SessionSyncState, error) {
	q := tx.Where("tenant_id = ? AND user_id = ? AND session_id = ?", owner.TenantID, owner.UserID, sessionID)
	if tx.Dialector.Name() == "mysql" { // FOR UPDATE 仅 MySQL 方言（SQLite 不支持该子句）
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row chat.SessionSyncState
	if err := q.First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// lockSessionRow 锁读同键 v1 会话行；返回会话行（含 Source 来源判定用）
// 与是否存在。
func lockSessionRow(tx *gorm.DB, owner chat.Owner, sessionID string) (*chat.Session, bool, error) {
	q := tx.Where("tenant_id = ? AND user_id = ? AND id = ?", owner.TenantID, owner.UserID, sessionID)
	if tx.Dialector.Name() == "mysql" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sess chat.Session
	err := q.First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &sess, true, nil
}

// isDuplicateKey 跨方言判定唯一约束冲突（MySQL 1062 Duplicate entry /
// SQLite UNIQUE constraint failed）。
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") || strings.Contains(msg, "UNIQUE constraint failed")
}

// stateOfRow：状态行 → wire 状态。claim 占位（version=0）无快照身份三件；
// active / deleted 携带当前确认身份。
func stateOfRow(row *chat.SessionSyncState) chat.SyncState {
	st := chat.SyncState{
		SourceID: row.SourceID,
		Version:  strconv.FormatInt(row.Version, 10),
	}
	switch {
	case row.Deleted:
		st.Status = chat.StateDeleted
		st.SnapshotID, st.Digest, st.Kind = row.SnapshotID, row.Digest, row.Kind
	case row.Version == 0:
		if row.LegacyClaim {
			st.Status = chat.StateClaimedLegacy
		} else {
			st.Status = chat.StateClaimedEmpty
		}
	default:
		st.Status = chat.StateActive
		st.SnapshotID, st.Digest, st.Kind = row.SnapshotID, row.Digest, row.Kind
	}
	return st
}

// Claim 原子认领同步键：新键插入唯一占位（version=0）；imported 可认领该
// 认证用户名下既有 v1 会话（claimed-legacy），native 接管 v1 历史 → 冲突
// （占位回滚）；同 source 重复 claim 幂等；异 source / tombstone 拒绝。
func (r *ChatSyncRepository) Claim(ctx context.Context, owner chat.Owner, sessionID, sourceID, origin string) (chat.SyncState, error) {
	var out chat.SyncState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		placeholder := chat.SessionSyncState{
			TenantID:  owner.TenantID,
			UserID:    owner.UserID,
			SessionID: sessionID,
			SourceID:  sourceID,
		}
		if err := tx.Create(&placeholder).Error; err != nil {
			if !isDuplicateKey(err) {
				return fmt.Errorf("insert sync state: %w", err)
			}
			// 键已存在：锁行判定幂等/冲突/tombstone
			row, err := lockSyncRow(tx, owner, sessionID)
			if err != nil {
				return err
			}
			if row.Deleted {
				return chat.ErrSyncClaimDeleted
			}
			if row.SourceID != sourceID {
				return chat.ErrSyncClaimConflict
			}
			out = stateOfRow(row)
			return nil
		}
		// 新占位：锁读同键 v1 会话分类；runtime/其他来源不可认领、native
		// 接管 v1 → 冲突回滚占位行
		legacySess, legacy, err := lockSessionRow(tx, owner, sessionID)
		if err != nil {
			return fmt.Errorf("check legacy session: %w", err)
		}
		if legacy {
			// 契约 §3：只有 App push 来源（Source 为空）的 v1 会话可被认领；
			// runtime / agent_chat 等其他来源不允许 claim（评审 B1，探针 P3）。
			if legacySess.Source != "" || origin == "native" {
				return chat.ErrSyncClaimConflict
			}
			placeholder.LegacyClaim = true
			if err := tx.Save(&placeholder).Error; err != nil {
				return fmt.Errorf("mark legacy claim: %w", err)
			}
		}
		out = stateOfRow(&placeholder)
		return nil
	})
	if err != nil {
		return chat.SyncState{}, err
	}
	return out, nil
}

// Apply 应用一次快照（replace / delete），返回精确确认。未 claim 的键拒绝
// 自行创建；tombstone 拒绝复活；同版本幂等（duplicate）或冲突（conflict）；
// 降版本 superseded 并回显当前身份；升版本（允许跳跃）在同一事务内替换
// 展示内容并推进确认身份。
func (r *ChatSyncRepository) Apply(ctx context.Context, owner chat.Owner, e chat.Envelope) (chat.Reply, error) {
	var out chat.Reply
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockSyncRow(tx, owner, e.AppSessionID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return chat.ErrSyncNotClaimed
		}
		if err != nil {
			return err
		}
		current := stateOfRow(row)

		// source 校验先于 tombstone（评审 B2，探针 P4）：异 source 不得推进
		// 墓碑身份——归属判定优先于一切阶段分支。
		if row.SourceID != e.SourceID {
			out = chat.Reply{Status: chat.StatusConflict, State: current}
			return nil
		}

		// tombstone 已生效：replace 一律拒绝复活；delete 仅按版本推进身份
		if row.Deleted {
			switch {
			case e.Kind == chat.KindReplace:
				out = chat.Reply{Status: chat.StatusDeleted, State: current}
				return nil
			case e.Version < row.Version:
				out = chat.Reply{Status: chat.StatusSuperseded, State: current}
				return nil
			case e.Version == row.Version:
				if row.SnapshotID == e.SnapshotID && row.Digest == e.Digest {
					out = chat.Reply{Status: chat.StatusDuplicate, State: current}
				} else {
					out = chat.Reply{Status: chat.StatusConflict, State: current}
				}
				return nil
			}
			row.Version, row.SnapshotID, row.Digest, row.Kind = e.Version, e.SnapshotID, e.Digest, string(chat.KindDelete)
			if err := tx.Save(row).Error; err != nil {
				return fmt.Errorf("update tombstone: %w", err)
			}
			out = chat.Reply{Status: chat.StatusApplied, State: stateOfRow(row)}
			return nil
		}

		switch {
		case e.Version == row.Version:
			if row.SnapshotID == e.SnapshotID && row.Digest == e.Digest && row.Kind == string(e.Kind) {
				out = chat.Reply{Status: chat.StatusDuplicate, State: current}
			} else {
				out = chat.Reply{Status: chat.StatusConflict, State: current}
			}
			return nil
		case e.Version < row.Version:
			out = chat.Reply{Status: chat.StatusSuperseded, State: current}
			return nil
		}

		// e.Version > row.Version：应用快照（全量替换语义，允许跨版本跳跃）
		switch e.Kind {
		case chat.KindReplace:
			if err := replaceSyncedSession(tx, owner, e); err != nil {
				return err
			}
		case chat.KindDelete:
			if err := removeSyncedSession(tx, owner, e.AppSessionID); err != nil {
				return err
			}
		}
		row.Version, row.SnapshotID, row.Digest, row.Kind = e.Version, e.SnapshotID, e.Digest, string(e.Kind)
		row.Deleted = e.Kind == chat.KindDelete
		if err := tx.Save(row).Error; err != nil {
			return fmt.Errorf("update sync state: %w", err)
		}
		out = chat.Reply{Status: chat.StatusApplied, State: stateOfRow(row)}
		return nil
	})
	if err != nil {
		return chat.Reply{}, err
	}
	return out, nil
}

// GetState 查询同步键当前状态（无行 = 未 claim / 异租户不可见）。
func (r *ChatSyncRepository) GetState(ctx context.Context, owner chat.Owner, sessionID string) (chat.SyncState, error) {
	var row chat.SessionSyncState
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ? AND session_id = ?", owner.TenantID, owner.UserID, sessionID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return chat.SyncState{}, chat.ErrSyncNotFound
	}
	if err != nil {
		return chat.SyncState{}, err
	}
	return stateOfRow(&row), nil
}

// replaceSyncedSession：全量替换展示内容（会话元数据 + 消息整批替换）。
// model_selection_id 纳入 upsert 更新列（v2 契约元数据全字段生效）。
func replaceSyncedSession(tx *gorm.DB, owner chat.Owner, e chat.Envelope) error {
	p := e.Payload
	createdAt, err := chat.ParseSyncTime(p.Session.CreatedAt)
	if err != nil {
		return fmt.Errorf("session %s: invalid created_at: %w", e.AppSessionID, err)
	}
	updatedAt, err := chat.ParseSyncTime(p.Session.UpdatedAt)
	if err != nil {
		return fmt.Errorf("session %s: invalid updated_at: %w", e.AppSessionID, err)
	}
	sess := &chat.Session{
		TenantID: owner.TenantID, UserID: owner.UserID,
		ID:                p.Session.ID,
		Title:             p.Session.Title,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		Model:             p.Session.Model,
		ModelSelectionID:  p.Session.ModelSelectionID,
		SystemPrompt:      p.Session.SystemPrompt,
		Status:            p.Session.Status,
		Mode:              p.Session.Mode,
		ProviderID:        p.Session.ProviderID,
		AgentID:           p.Session.AgentID,
		PermissionProfile: p.Session.PermissionProfile,
		Hidden:            p.Session.Hidden,
		ExtraDirectories:  p.Session.ExtraDirectories,
		IsUserBound:       p.Session.IsUserBound,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "created_at", "updated_at", "model", "model_selection_id",
			"system_prompt", "status", "mode", "provider_id", "agent_id",
			"permission_profile", "hidden", "extra_directories", "is_user_bound",
		}),
	}).Create(sess).Error; err != nil {
		return fmt.Errorf("upsert session %s: %w", e.AppSessionID, err)
	}

	if err := TenantOwned(tx, owner.TenantID).
		Where("user_id = ? AND session_id = ?", owner.UserID, e.AppSessionID).
		Delete(&chat.Message{}).Error; err != nil {
		return fmt.Errorf("delete messages for %s: %w", e.AppSessionID, err)
	}
	msgs := make([]*chat.Message, len(p.Messages))
	for i, m := range p.Messages {
		mc, err := chat.ParseSyncTime(m.CreatedAt)
		if err != nil {
			return fmt.Errorf("message %s: invalid created_at: %w", m.ID, err)
		}
		ord := m.Ordinal // 逐条取址：写入 v2 展示序（读路径按其稳定排序）
		msgs[i] = &chat.Message{
			TenantID: owner.TenantID, UserID: owner.UserID, SessionID: e.AppSessionID,
			ID: m.ID, Role: m.Role, Content: m.Content, CreatedAt: mc,
			Hidden: m.Hidden, TokenUsage: m.TokenUsage, Feedback: m.Feedback,
			SyncOrdinal: &ord,
		}
	}
	if len(msgs) > 0 {
		// 分批插入：长会话（数千条）单条 Create 会超占位符上限
		//（SQLite 32766 / MySQL 65535），与 v1 路径同款分批（评审重要项）。
		if err := tx.CreateInBatches(&msgs, 100).Error; err != nil {
			return fmt.Errorf("insert messages for %s: %w", e.AppSessionID, err)
		}
	}
	return nil
}

// removeSyncedSession：delete 应用路径——移除可见正文（会话 + 消息 + 上传
// 记录，上传记录不得脱离会话存续），状态行由 Apply 保留为 tombstone
// （不随内容删除）。
func removeSyncedSession(tx *gorm.DB, owner chat.Owner, sessionID string) error {
	if err := TenantOwned(tx, owner.TenantID).
		Where("user_id = ? AND session_id = ?", owner.UserID, sessionID).
		Delete(&chat.Message{}).Error; err != nil {
		return fmt.Errorf("delete messages for %s: %w", sessionID, err)
	}
	if err := TenantOwned(tx, owner.TenantID).
		Where("user_id = ? AND session_id = ?", owner.UserID, sessionID).
		Delete(&chat.UploadRecord{}).Error; err != nil {
		return fmt.Errorf("delete upload records for %s: %w", sessionID, err)
	}
	if err := TenantOwned(tx, owner.TenantID).
		Where("user_id = ? AND id = ?", owner.UserID, sessionID).
		Delete(&chat.Session{}).Error; err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}
	return nil
}
