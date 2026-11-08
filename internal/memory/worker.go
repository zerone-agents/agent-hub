package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Sync processes a bounded batch. Unknown writes are reconciled by reads only.
func (s *Service) Sync(ctx context.Context, limit int) error {
	s.worker.Lock()
	defer s.worker.Unlock()
	if s.cfg.Index == nil {
		return nil
	}
	if limit < 1 || limit > 100 {
		limit = 100
	}
	now := time.Now()
	leaseQuery := s.db.WithContext(ctx).Model(&outboxRow{}).Where("app_key = ? AND status = ? AND lease_until < ?", s.appKey(), "processing", now)
	if s.cfg.Backend == BackendNative {
		leaseQuery = leaseQuery.Where("action IN ?", []string{"put", "remove"})
	}
	if e := leaseQuery.Update("status", "unknown").Error; e != nil {
		return e
	}
	var jobs []outboxRow
	jobQuery := s.db.WithContext(ctx).Where("app_key = ? AND status IN ?", s.appKey(), []string{"pending", "unknown"})
	if s.cfg.Backend == BackendNative {
		jobQuery = jobQuery.Where("action IN ?", []string{"put", "remove"})
	}
	if e := jobQuery.Order("updated_at, id").Limit(limit).Find(&jobs).Error; e != nil {
		return e
	}
	var failures []error
	for i := range jobs {
		job := &jobs[i]
		unknown := job.Status == "unknown"
		claim := s.db.WithContext(ctx).Model(&outboxRow{}).Where("id = ? AND status = ? AND attempts = ?", job.ID, job.Status, job.Attempts).Updates(map[string]any{"status": "processing", "lease_until": time.Now().Add(5 * time.Minute), "attempts": gorm.Expr("attempts + 1")})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			continue
		}
		if e := s.syncOne(ctx, job, unknown); e != nil {
			state := "pending"
			if errors.Is(e, ErrIndexUnknown) || job.Action == "put" {
				state = "unknown"
			}
			// The upstream attempt may consume the entire caller deadline. Persist
			// its uncertain result with a separate bounded context, so a completed
			// remote write does not remain stuck until the crash-recovery lease.
			// The attempt CAS prevents a late failure from changing a newer lease.
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			persistErr := s.db.WithContext(recoveryCtx).Model(&outboxRow{}).Where("id = ? AND status = ? AND attempts = ?", job.ID, "processing", job.Attempts+1).Update("status", state).Error
			cancel()
			if persistErr != nil {
				e = errors.Join(e, fmt.Errorf("memory outcome state persistence failed: %w", persistErr))
			}
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}
func (s *Service) jobState(ctx context.Context, j *outboxRow, state string) error {
	return s.db.WithContext(ctx).Model(&outboxRow{}).Where("id = ?", j.ID).Update("status", state).Error
}
func documentName(l *lifecycleRow) string { return "memory-" + l.ID }
func (s *Service) document(ctx context.Context, l *lifecycleRow) (string, error) {
	if l.DocumentID != "" {
		return l.DocumentID, nil
	}
	allow := l.DocumentState == "new"
	if allow {
		res := s.db.WithContext(ctx).Model(&lifecycleRow{}).Where("id = ? AND document_state = ?", l.ID, "new").Update("document_state", "unknown")
		if res.Error != nil {
			return "", res.Error
		}
		allow = res.RowsAffected == 1
	}
	doc, e := s.cfg.Index.EnsureDocument(ctx, s.cfg.DatasetID, documentName(l), allow)
	if e != nil {
		return "", e
	}
	if doc == "" {
		return "", ErrIndexUnknown
	}
	if e = s.db.WithContext(ctx).Model(&lifecycleRow{}).Where("id = ? AND document_id = ?", l.ID, "").Updates(map[string]any{"document_id": doc, "document_state": "confirmed"}).Error; e != nil {
		return "", e
	}
	l.DocumentID = doc
	l.DocumentState = "confirmed"
	return doc, nil
}
func (s *Service) syncOne(ctx context.Context, j *outboxRow, unknown bool) error {
	if s.cfg.Backend == BackendNative && j.Action == "drop" {
		return ErrUnsupported
	}
	var l lifecycleRow
	if e := s.db.WithContext(ctx).First(&l, "id = ? AND app_key = ?", j.ScopeKey, s.appKey()).Error; e != nil {
		return e
	}
	if l.State == "cleaned" {
		return s.jobState(ctx, j, "cancelled")
	}
	if j.Action == "put" {
		var r recordRow
		if e := s.db.WithContext(ctx).First(&r, "id = ? AND scope_key = ?", j.RecordID, l.ID).Error; e != nil {
			return e
		}
		current := l.State == "active" && l.Fence == j.Fence && r.Status == "active" && r.Version == j.Version
		if !current && !unknown {
			return s.jobState(ctx, j, "cancelled")
		}
		doc, e := s.document(ctx, &l)
		if e != nil {
			return e
		}
		var v value
		if e = s.decode(ctx, &l, j.ID, j.Payload, &v); e != nil {
			return e
		}
		chunk := ""
		if unknown {
			chunk, e = s.cfg.Index.Find(ctx, s.cfg.DatasetID, doc, v.Content)
			if e != nil {
				return e
			}
			if chunk == "" {
				return ErrIndexUnknown
			}
		} else {
			chunk, e = s.cfg.Index.Put(ctx, s.cfg.DatasetID, doc, v.Content)
			if e != nil {
				return e
			}
			if chunk == "" {
				return ErrIndexUnknown
			}
		}
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var latest lifecycleRow
			if e := tx.First(&latest, "id = ?", l.ID).Error; e != nil {
				return e
			}
			var record recordRow
			if e := tx.First(&record, "id = ?", r.ID).Error; e != nil {
				return e
			}
			if latest.State != "active" || latest.Fence != j.Fence || record.Status != "active" || record.Version != j.Version {
				if e := s.enqueue(tx, &latest, &record, "remove", chunk, v); e != nil {
					return e
				}
				return tx.Model(&outboxRow{}).Where("id = ?", j.ID).Update("status", "cancelled").Error
			}
			res := tx.Model(&recordRow{}).Where("id = ? AND version = ? AND status = ?", r.ID, j.Version, "active").Updates(map[string]any{"chunk_id": chunk, "sync_status": "confirmed"})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrConflict
			}
			return tx.Model(&outboxRow{}).Where("id = ?", j.ID).Update("status", "done").Error
		})
	}
	if j.Action == "remove" {
		if j.ChunkID != "" && l.DocumentID != "" {
			var v value
			if e := s.decode(ctx, &l, j.ID, j.Payload, &v); e != nil {
				return e
			}
			var rows []recordRow
			if e := s.db.WithContext(ctx).Where("scope_key = ? AND kind = ? AND status = ?", l.ID, "fact", "active").Limit(maxRecords).Find(&rows).Error; e != nil {
				return e
			}
			refs := 0
			for i := range rows {
				var active value
				if e := s.decode(ctx, &l, rows[i].ID, rows[i].Payload, &active); e != nil {
					return e
				}
				if rows[i].ChunkID == j.ChunkID || active.Content == v.Content {
					refs++
				}
			}

			if refs == 0 {
				if e := s.cfg.Index.Remove(ctx, s.cfg.DatasetID, l.DocumentID, j.ChunkID); e != nil {
					return e
				}
				if e := s.repairReferences(ctx, &l, j.ChunkID, v.Content); e != nil {
					return e
				}
			}
		}
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Model(&recordRow{}).Where("id = ? AND version = ? AND status != ?", j.RecordID, j.Version, "active").Update("sync_status", "confirmed").Error; e != nil {
				return e
			}
			return tx.Model(&outboxRow{}).Where("id = ?", j.ID).Update("status", "done").Error
		})
	}
	if j.Action == "drop" {
		if l.State != "frozen" {
			return s.jobState(ctx, j, "cancelled")
		}
		var waiting int64
		if e := s.db.WithContext(ctx).Model(&outboxRow{}).Where("scope_key = ? AND action = ? AND status IN ?", l.ID, "put", []string{"processing", "unknown"}).Count(&waiting).Error; e != nil {
			return e
		}
		if waiting > 0 {
			return fmt.Errorf("memory cleanup awaits in-flight writes")
		}
		doc := l.DocumentID
		if doc == "" && l.DocumentState != "new" {
			var e error
			doc, e = s.cfg.Index.EnsureDocument(ctx, s.cfg.DatasetID, documentName(&l), false)
			if e != nil {
				return e
			}
			if doc == "" {
				return ErrIndexUnknown
			}
		}
		if doc != "" {
			if e := s.cfg.Index.DropDocument(ctx, s.cfg.DatasetID, doc); e != nil {
				return e
			}
		}
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Where("scope_key = ?", l.ID).Delete(&recordRow{}).Error; e != nil {
				return e
			}
			if e := tx.Where("scope_key = ?", l.ID).Delete(&taskRow{}).Error; e != nil {
				return e
			}
			if e := tx.Model(&operationRow{}).Where("scope_key = ?", l.ID).Updates(map[string]any{"result": nil, "request_hash": ""}).Error; e != nil {
				return e
			}
			if e := tx.Model(&outboxRow{}).Where("scope_key = ?", l.ID).Updates(map[string]any{"status": "done", "chunk_id": "", "record_id": "", "payload": nil}).Error; e != nil {
				return e
			}
			return tx.Model(&lifecycleRow{}).Where("id = ? AND state = ?", l.ID, "frozen").Updates(map[string]any{"state": "cleaned", "wrapped_key": nil, "scope_payload": nil, "profile": nil, "document_id": "", "document_state": "deleted"}).Error
		})
	}
	return ErrInvalid
}

// A second Hub process may have published a reference while a remote delete
// was in progress. Recheck under the lifecycle lock and rebuild such facts.
func (s *Service) repairReferences(ctx context.Context, l *lifecycleRow, chunk, content string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current lifecycleRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", l.ID).Error; e != nil {
			return e
		}
		if current.State != "active" {
			return nil
		}
		var rows []recordRow
		if e := tx.Where("scope_key = ? AND kind = ? AND status = ?", l.ID, "fact", "active").Limit(maxRecords).Find(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			var v value
			if e := s.decode(ctx, &current, rows[i].ID, rows[i].Payload, &v); e != nil {
				return e
			}
			if rows[i].ChunkID != chunk && v.Content != content {
				continue
			}
			rows[i].ChunkID = ""
			rows[i].SyncStatus = "pending"
			if e := tx.Save(&rows[i]).Error; e != nil {
				return e
			}
			if e := s.enqueue(tx, &current, &rows[i], "put", ""); e != nil {
				return e
			}
		}
		return nil
	})
}
