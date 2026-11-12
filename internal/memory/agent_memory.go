package memory

// agent_memory.go implements the agent-curated memory path for the memory
// MCP (spec docs/superpowers/specs/2026-11-11-memory-mcp-design.md §5/§6):
// direct ledger writes with a dedicated kind, a distinct fingerprint
// domain (digest(scopeKey, "mcp-content", content) — never the manual
// ledger's SourceMessageID+TurnID+Quote input), source-authority marking
// for update guards, and a diagnosable pool-full error. Storage, CAS,
// payload encryption, outbox index sync, and audit plumbing are all the
// existing ledger machinery — this file only adds the agent-facing entry
// points.

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentMemoryKind marks records written via the memory MCP tools. Update is
// restricted to this kind plus a payload Source match (only records the
// agent itself wrote — spec §5).
const AgentMemoryKind = "agent-memory"

type AgentMemoryInput struct {
	Subject, Category, Content, Quote, TurnID string
}

type AgentMemoryRecord struct {
	ID        string    `json:"id"`
	Version   uint64    `json:"version"`
	Subject   string    `json:"subject"`
	Category  string    `json:"category"`
	Content   string    `json:"content"`
	Quote     string    `json:"quote"`
	TurnID    string    `json:"turnId,omitempty"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ErrPoolFull is the dedicated pool-exhaustion error (maxRecords per scope,
// write-reject-no-evict semantics shared with the manual ledger — spec §7):
// handlers map it to an actionable message so the agent can stop retrying.
var ErrPoolFull = errors.New("memory pool full")

// agentMemoryValue embeds the ledger value: the outbox worker decodes job
// payloads back into value to index Content (worker.go syncOne), so the
// embedded fields must keep the value JSON shape. Quote/Source are this
// kind's extensions; Source ("agent:<name>") is the update-authority mark.
type agentMemoryValue struct {
	value
	Quote  string `json:"quote,omitempty"`
	Source string `json:"source,omitempty"`
}

func (v agentMemoryValue) record(id string, version uint64, at time.Time) AgentMemoryRecord {
	return AgentMemoryRecord{ID: id, Version: version, Subject: v.Subject, Category: v.Category, Content: v.Content, Quote: v.Quote, TurnID: v.TurnID, Source: v.Source, UpdatedAt: at}
}

// agentNameFromScope extracts the bare agent name from the pinned
// "agent:<name>" UserID (handlers always pass this shape; defensive for
// direct service callers).
func agentNameFromScope(sc Scope) string {
	const prefix = "agent:"
	if len(sc.UserID) > len(prefix) && sc.UserID[:len(prefix)] == prefix {
		return sc.UserID[len(prefix):]
	}
	return sc.UserID
}

func agentFingerprint(scopeKey, content string) string {
	return digest([]string{scopeKey, "mcp-content", content})
}

func validAgentMemoryInput(in AgentMemoryInput) bool {
	if in.Content == "" || in.Quote == "" {
		return false
	}
	if utf8.RuneCountInString(in.Content) > 2000 || utf8.RuneCountInString(in.Quote) > 600 {
		return false
	}
	// schema 承诺「≤128 字符」——按 Unicode 码点计（与正文/摘录一致），
	// 字节判定会拒绝合法 CJK 主体（review 重要项 8）。
	return utf8.RuneCountInString(in.Subject) <= 128 &&
		utf8.RuneCountInString(in.Category) <= 64 &&
		utf8.RuneCountInString(in.TurnID) <= 128
}

// EnsureAgentLifecycle idempotently provisions the pinned scope (spec §3:
// done server-side on first tool call, invisible to callers). Activate
// takes the mutation mutex itself and is NOT idempotent across op IDs (a
// fresh op ID against an existing lifecycle returns ErrConflict,
// service.go Activate), so: check active first, provision with a
// deterministic op ID, and re-check active on conflict (concurrent or
// manually pre-provisioned lifecycles).
func (s *Service) EnsureAgentLifecycle(ctx context.Context, sc Scope) error {
	isActive := func() bool {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, e := s.life(tx, sc, true)
			return e
		}) == nil
	}
	if isActive() {
		return nil
	}
	// Deterministic op ID: repeated provisioning replays the same operation
	// (Activate's operation ledger) instead of conflicting. 18+64=82 ≤ 128.
	opID := "agent-memory-auto-" + scopeKey(sc)
	if _, err := s.Activate(ctx, sc, opID); err != nil {
		if isActive() {
			return nil
		}
		return err
	}
	return nil
}

// RememberAgentFact writes one curated fact directly (no candidate/task
// flow). Idempotent per (scope, content) fingerprint: a repeat returns the
// existing record. Pool exhaustion returns ErrPoolFull.
func (s *Service) RememberAgentFact(ctx context.Context, sc Scope, in AgentMemoryInput) (AgentMemoryRecord, error) {
	if !validAgentMemoryInput(in) {
		return AgentMemoryRecord{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out AgentMemoryRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		// Scope-wide cap, identical semantics to the manual ledger's
		// write-reject-no-evict check (SubmitCandidate).
		var count int64
		if err = tx.Model(&recordRow{}).Where("scope_key = ?", l.ID).Count(&count).Error; err != nil {
			return err
		}
		fp := agentFingerprint(l.ID, in.Content)
		var old recordRow
		err = tx.First(&old, "scope_key = ? AND kind = ? AND fingerprint = ?", l.ID, AgentMemoryKind, fp).Error
		if err == nil {
			var v agentMemoryValue
			if e := s.decode(ctx, l, old.ID, old.Payload, &v); e != nil {
				return e
			}
			out = v.record(old.ID, old.Version, old.UpdatedAt)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if count >= maxRecords {
			return ErrPoolFull
		}
		v := agentMemoryValue{
			value: value{Content: in.Content, Subject: in.Subject, Category: in.Category, TurnID: in.TurnID},
			Quote: in.Quote, Source: "agent:" + agentNameFromScope(sc),
		}
		r := recordRow{ID: uuid.NewString(), ScopeKey: l.ID, Kind: AgentMemoryKind, Version: 1, Status: "active", SyncStatus: "pending", Fingerprint: fp}
		var e error
		if r.Payload, e = s.encode(ctx, l, r.ID, v); e != nil {
			return e
		}
		if e = tx.Create(&r).Error; e != nil {
			return e
		}
		// No snapshot arg: enqueue decodes the record payload back into
		// value (embedded fields) for the worker's index sync.
		if e = s.enqueue(tx, l, &r, "put", ""); e != nil {
			return e
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		out = v.record(r.ID, r.Version, r.UpdatedAt)
		return nil
	})
	return out, err
}

// UpdateAgentFact CAS-updates one record the calling agent itself wrote
// (kind + payload Source guards). A fresh quote is mandatory evidence for
// the correction (spec §5).
func (s *Service) UpdateAgentFact(ctx context.Context, sc Scope, id string, expectedVersion uint64, in AgentMemoryInput) (AgentMemoryRecord, error) {
	if !validAgentMemoryInput(in) {
		return AgentMemoryRecord{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out AgentMemoryRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		var r recordRow
		if err = tx.First(&r, "id = ? AND scope_key = ?", id, l.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if r.Kind != AgentMemoryKind {
			return ErrForbidden
		}
		var v agentMemoryValue
		if e := s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
			return e
		}
		if v.Source != "agent:"+agentNameFromScope(sc) {
			return ErrForbidden
		}
		if r.Status != "active" || r.Version != expectedVersion {
			return ErrConflict
		}
		nv := v
		nv.Content = in.Content
		nv.Quote = in.Quote
		if in.Subject != "" {
			nv.Subject = in.Subject
		}
		if in.Category != "" {
			nv.Category = in.Category
		}
		if in.TurnID != "" {
			nv.TurnID = in.TurnID
		}
		r.Version++
		r.Fingerprint = agentFingerprint(l.ID, nv.Content)
		r.SyncStatus = "pending"
		var e error
		if r.Payload, e = s.encode(ctx, l, r.ID, nv); e != nil {
			return e
		}
		if e = tx.Save(&r).Error; e != nil {
			return e
		}
		if e = s.enqueue(tx, l, &r, "put", ""); e != nil {
			return e
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		out = nv.record(r.ID, r.Version, r.UpdatedAt)
		return nil
	})
	return out, err
}

// ListAgentMemories returns the calling agent's own records (kind + active
// filter). memoryID non-empty does a direct single fetch — bypassing the
// 50-item recency window so older memories remain CAS-updatable (spec §5).
// category filtering happens BEFORE the recency window: the payload is
// encrypted so SQL cannot filter it — a category query scans all active rows
// (bounded by maxRecords) and truncates after filtering, so matching records
// are never evicted from the window by non-matching categories (review
// 重要项 7).
func (s *Service) ListAgentMemories(ctx context.Context, sc Scope, category, memoryID string, limit int) ([]AgentMemoryRecord, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}
	var out []AgentMemoryRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		q := tx.Where("scope_key = ? AND kind = ? AND status = ?", l.ID, AgentMemoryKind, "active")
		if memoryID != "" {
			q = q.Where("id = ?", memoryID)
		}
		var rows []recordRow
		if category == "" {
			if err = q.Order("updated_at DESC, id").Limit(limit).Find(&rows).Error; err != nil {
				return err
			}
		} else {
			// 类目在加密 payload 内：全量拉取（≤maxRecords）解密过滤后再截断。
			if err = q.Order("updated_at DESC, id").Limit(maxRecords).Find(&rows).Error; err != nil {
				return err
			}
			filtered := make([]recordRow, 0, len(rows))
			for i := range rows {
				var v agentMemoryValue
				if e := s.decode(ctx, l, rows[i].ID, rows[i].Payload, &v); e != nil {
					return e
				}
				if v.Category == category {
					filtered = append(filtered, rows[i])
				}
			}
			if len(filtered) > limit {
				filtered = filtered[:limit]
			}
			rows = filtered
		}
		out = make([]AgentMemoryRecord, 0, len(rows))
		for i := range rows {
			var v agentMemoryValue
			if e := s.decode(ctx, l, rows[i].ID, rows[i].Payload, &v); e != nil {
				return e
			}
			out = append(out, v.record(rows[i].ID, rows[i].Version, rows[i].UpdatedAt))
		}
		return nil
	})
	return out, err
}

// UpdateAgentFactAdmin is the management-channel correction variant: content
// is required but a conversation quote is not — admins act outside a
// dialogue, so the evidence requirement is waived (review 阻塞②). The kind +
// Source authority guards and CAS still apply; a non-empty quote still
// updates the stored evidence.
func (s *Service) UpdateAgentFactAdmin(ctx context.Context, sc Scope, id string, expectedVersion uint64, content, quote string) (AgentMemoryRecord, error) {
	if content == "" || utf8.RuneCountInString(content) > 2000 {
		return AgentMemoryRecord{}, ErrInvalid
	}
	if quote != "" && utf8.RuneCountInString(quote) > 600 {
		return AgentMemoryRecord{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out AgentMemoryRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		var r recordRow
		if err = tx.First(&r, "id = ? AND scope_key = ?", id, l.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if r.Kind != AgentMemoryKind {
			return ErrForbidden
		}
		var v agentMemoryValue
		if e := s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
			return e
		}
		if v.Source != "agent:"+agentNameFromScope(sc) {
			return ErrForbidden
		}
		if r.Status != "active" || r.Version != expectedVersion {
			return ErrConflict
		}
		nv := v
		nv.Content = content
		if quote != "" {
			nv.Quote = quote
		}
		r.Version++
		r.Fingerprint = agentFingerprint(l.ID, nv.Content)
		r.SyncStatus = "pending"
		var e error
		if r.Payload, e = s.encode(ctx, l, r.ID, nv); e != nil {
			return e
		}
		if e = tx.Save(&r).Error; e != nil {
			return e
		}
		if e = s.enqueue(tx, l, &r, "put", ""); e != nil {
			return e
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		out = nv.record(r.ID, r.Version, r.UpdatedAt)
		return nil
	})
	return out, err
}

// DeleteAgentFact physically removes one record the calling agent itself
// wrote (kind + Source guards, CAS). Physical deletion is deliberate here —
// unlike the manual ledger's soft invalidate, this is the pool-full remedy
// path (spec §7「可经管理台清理腾位」): the row must actually leave the
// scope for the maxRecords budget to free up. The retrieval-replica chunk is
// cleaned via the outbox "remove" action before the row goes.
func (s *Service) DeleteAgentFact(ctx context.Context, sc Scope, id string, expectedVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		var r recordRow
		if err = tx.First(&r, "id = ? AND scope_key = ?", id, l.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if r.Kind != AgentMemoryKind {
			return ErrForbidden
		}
		var v agentMemoryValue
		if e := s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
			return e
		}
		if v.Source != "agent:"+agentNameFromScope(sc) {
			return ErrForbidden
		}
		if r.Status != "active" || r.Version != expectedVersion {
			return ErrConflict
		}
		// 先入队 remove（清检索副本 chunk），行随后物理删除——腾位生效。
		if e := s.enqueue(tx, l, &r, "remove", r.ChunkID); e != nil {
			return e
		}
		if e := tx.Delete(&recordRow{}, "id = ? AND scope_key = ?", r.ID, l.ID).Error; e != nil {
			return e
		}
		return s.touch(tx, l)
	})
}

// RecallAgentMemories searches the retrieval replica for the scope's
// document and maps chunk hits back to active agent-memory records.
// Read-only; index visibility is eventually consistent via the outbox
// worker (tool descriptions say newly written memories are searchable
// after a short delay — spec §7).
func (s *Service) RecallAgentMemories(ctx context.Context, sc Scope, query string, limit int) ([]AgentMemoryRecord, error) {
	if limit < 1 || limit > 20 {
		limit = 5
	}
	var l lifecycleRow
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		l = *row
		return nil
	}); err != nil {
		return nil, err
	}
	doc, err := s.document(ctx, &l)
	if err != nil {
		return nil, err
	}
	hits, err := s.cfg.Index.Search(ctx, s.cfg.DatasetID, doc, query, limit)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return []AgentMemoryRecord{}, nil
	}
	out := make([]AgentMemoryRecord, 0, len(hits))
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l2, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		seen := make(map[string]bool, len(hits))
		for _, h := range hits {
			if h.ChunkID == "" || seen[h.ChunkID] {
				continue
			}
			seen[h.ChunkID] = true
			var r recordRow
			// Chunks of other kinds in the same document (manual ledger
			// facts share the lifecycle document) are skipped silently.
			if e = tx.First(&r, "scope_key = ? AND chunk_id = ? AND kind = ? AND status = ?", l2.ID, h.ChunkID, AgentMemoryKind, "active").Error; e != nil {
				continue
			}
			var v agentMemoryValue
			if e = s.decode(ctx, l2, r.ID, r.Payload, &v); e != nil {
				continue
			}
			out = append(out, v.record(r.ID, r.Version, r.UpdatedAt))
		}
		return nil
	})
	return out, err
}
