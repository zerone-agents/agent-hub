package memory

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxRecords = 1000

type Service struct {
	db     *gorm.DB
	cfg    Config
	mu     *sync.Mutex
	worker *sync.Mutex
}

// Multiple service credentials may route to one application in the same Hub.
// They share mutation and worker serialization, rather than independent locks.
var applicationLocks sync.Map

type appLocks struct {
	mutation sync.Mutex
	worker   sync.Mutex
}
type value struct {
	RejectionReason string `json:"rejectionReason,omitempty"`
	ReplaceFactID   string `json:"replaceFactId,omitempty"`
	ExpectedVersion uint64 `json:"expectedVersion,omitempty"`
	Content         string `json:"content"`
	Subject         string `json:"subject"`
	Temporal        string `json:"temporal"`
	Category        string `json:"category"`
	ConsentRef      string `json:"consentRef"`
	PolicyVersion   string `json:"policyVersion"`
	SourceMessageID string `json:"sourceMessageId"`
	TurnID          string `json:"turnId"`
}
type profileValue struct {
	Content string
	FactIDs []string
}

func NewService(db *gorm.DB, cfg Config) (*Service, error) {
	if cfg.Backend == "" {
		cfg.Backend = BackendKnowledge
	}
	if cfg.Backend != BackendKnowledge && cfg.Backend != BackendNative {
		return nil, fmt.Errorf("%w: unknown memory backend", ErrInvalid)
	}
	if db == nil || cfg.Wrapper == nil || !cfg.SyntheticOnly || cfg.TenantID == "" || cfg.ApplicationID == "" || cfg.PolicyVersion == "" || cfg.DatasetID == "" {
		return nil, fmt.Errorf("%w: synthetic memory configuration incomplete", ErrInvalid)
	}
	locks, _ := applicationLocks.LoadOrStore(digest([]string{cfg.TenantID, cfg.ApplicationID}), &appLocks{})
	shared := locks.(*appLocks)
	return &Service{db: db, cfg: cfg, mu: &shared.mutation, worker: &shared.worker}, nil
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func scopeKey(sc Scope) string { return digest(sc) }
func userKey(sc Scope) string  { return digest([]string{sc.TenantID, sc.ApplicationID, sc.UserID}) }
func (s *Service) appKey() string {
	if s.cfg.Backend == BackendNative {
		return digest([]string{s.cfg.TenantID, s.cfg.ApplicationID, BackendNative, s.cfg.DatasetID})
	}
	return digest([]string{s.cfg.TenantID, s.cfg.ApplicationID})
}
func (s *Service) check(sc Scope) error {
	if sc.TenantID != s.cfg.TenantID || sc.ApplicationID != s.cfg.ApplicationID {
		return ErrForbidden
	}
	for _, v := range []string{sc.TenantID, sc.ApplicationID, sc.UserID, sc.LifecycleID} {
		if strings.TrimSpace(v) == "" || len(v) > 128 {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Service) life(tx *gorm.DB, sc Scope, active bool) (*lifecycleRow, error) {
	if err := s.check(sc); err != nil {
		return nil, err
	}
	var l lifecycleRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&l, "id = ?", scopeKey(sc)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if l.AppKey != s.appKey() {
		return nil, ErrConflict
	}
	if active {
		var u userRow
		if err = tx.First(&u, "id = ?", l.UserKey).Error; err != nil {
			return nil, err
		}
		if l.State != "active" || u.CurrentID != l.ID || u.State != "active" {
			return nil, ErrForbidden
		}
	}
	if err = s.backfillScope(tx, &l, sc); err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Service) backfillScope(tx *gorm.DB, l *lifecycleRow, sc Scope) error {
	if l.State == "cleaned" || len(l.ScopePayload) > 0 || len(l.WrappedKey) == 0 {
		return nil
	}
	payload, err := s.encode(tx.Statement.Context, l, "scope", sc)
	if err != nil {
		return err
	}
	updated := tx.Model(&lifecycleRow{}).Where("id = ? AND state IN ? AND wrapped_key IS NOT NULL AND LENGTH(wrapped_key) > 0 AND (scope_payload IS NULL OR LENGTH(scope_payload) = 0)", l.ID, []string{"active", "frozen"}).Update("scope_payload", payload)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected > 0 {
		l.ScopePayload = payload
	}
	return nil
}
func (s *Service) key(ctx context.Context, l *lifecycleRow) ([]byte, error) {
	return s.cfg.Wrapper.Unwrap(ctx, l.WrappedKey, []byte("memory/dek/"+l.ID))
}
func (s *Service) encode(ctx context.Context, l *lifecycleRow, id string, v any) ([]byte, error) {
	k, e := s.key(ctx, l)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	return seal(k, b, []byte(l.ID+"/"+id))
}
func (s *Service) decode(ctx context.Context, l *lifecycleRow, id string, b []byte, v any) error {
	k, e := s.key(ctx, l)
	if e != nil {
		return e
	}
	p, e := open(k, b, []byte(l.ID+"/"+id))
	if e != nil {
		return e
	}
	return json.Unmarshal(p, v)
}
func (s *Service) fingerprint(ctx context.Context, l *lifecycleRow, v any) (string, error) {
	k, e := s.key(ctx, l)
	if e != nil {
		return "", e
	}
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, k)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *Service) operation(tx *gorm.DB, l *lifecycleRow, opID, action string, request any, out any) (bool, error) {
	if strings.TrimSpace(opID) == "" || len(opID) > 128 {
		return false, ErrInvalid
	}
	id := digest([]string{l.ID, opID})
	var op operationRow
	e := tx.First(&op, "id = ?", id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	hash, e := s.fingerprint(tx.Statement.Context, l, request)
	if e != nil {
		return false, e
	}
	if op.Action != action || op.RequestHash != hash {
		return false, ErrConflict
	}
	if e = s.decode(tx.Statement.Context, l, id, op.Result, out); e != nil {
		return false, e
	}
	return true, nil
}
func (s *Service) saveOperation(tx *gorm.DB, l *lifecycleRow, opID, action string, request, out any) error {
	id := digest([]string{l.ID, opID})
	b, e := s.encode(tx.Statement.Context, l, id, out)
	if e != nil {
		return e
	}
	h, e := s.fingerprint(tx.Statement.Context, l, request)
	if e != nil {
		return e
	}
	return tx.Create(&operationRow{ID: id, ScopeKey: l.ID, Action: action, RequestHash: h, Result: b}).Error
}
func (s *Service) touch(tx *gorm.DB, l *lifecycleRow) error {
	l.Revision++
	l.Profile = nil
	return tx.Save(l).Error
}
func (s *Service) enqueue(tx *gorm.DB, l *lifecycleRow, r *recordRow, action, chunk string, snapshot ...value) error {
	job := outboxRow{ID: uuid.NewString(), AppKey: l.AppKey, ScopeKey: l.ID, Fence: l.Fence, Action: action, ChunkID: chunk, Status: "pending"}
	if r != nil {
		job.RecordID = r.ID
		job.Version = r.Version
		var v value
		if len(snapshot) > 0 {
			v = snapshot[0]
		} else if e := s.decode(tx.Statement.Context, l, r.ID, r.Payload, &v); e != nil {
			return e
		}
		var e error
		job.Payload, e = s.encode(tx.Statement.Context, l, job.ID, v)
		if e != nil {
			return e
		}
	}
	return tx.Create(&job).Error
}
func (s *Service) record(tx *gorm.DB, l *lifecycleRow, id, kind string) (*recordRow, error) {
	var r recordRow
	e := tx.First(&r, "id = ? AND scope_key = ? AND kind = ?", id, l.ID, kind).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &r, e
}
func (s *Service) fact(ctx context.Context, l *lifecycleRow, r *recordRow) (Fact, error) {
	var v value
	if e := s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
		return Fact{}, e
	}
	return Fact{ID: r.ID, Version: r.Version, Content: v.Content, Subject: v.Subject, Temporal: v.Temporal, Category: v.Category, Status: r.Status, SyncStatus: r.SyncStatus, ConsentRef: v.ConsentRef, UpdatedAt: r.UpdatedAt}, nil
}
func (s *Service) candidate(ctx context.Context, l *lifecycleRow, r *recordRow) (Candidate, error) {
	var v value
	if e := s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
		return Candidate{}, e
	}
	return Candidate{ID: r.ID, Version: r.Version, Quote: v.Content, Subject: v.Subject, Temporal: v.Temporal, Category: v.Category, SourceMessageID: v.SourceMessageID, TurnID: v.TurnID, Status: r.Status, ReplaceFactID: v.ReplaceFactID, ExpectedVersion: v.ExpectedVersion, RejectionReason: v.RejectionReason}, nil
}

func (s *Service) Activate(ctx context.Context, sc Scope, opID string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Operation
	if e := s.check(sc); e != nil {
		return out, e
	}
	if strings.TrimSpace(opID) == "" || len(opID) > 128 {
		return out, ErrInvalid
	}
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing lifecycleRow
		e := tx.First(&existing, "id = ?", scopeKey(sc)).Error
		if e == nil {
			if existing.AppKey != s.appKey() {
				return ErrConflict
			}
			if existing.State != "active" {
				return ErrConflict
			}
			if e = s.backfillScope(tx, &existing, sc); e != nil {
				return e
			}
			found, e := s.operation(tx, &existing, opID, "activate", sc, &out)
			if e != nil {
				return e
			}
			if found {
				return nil
			}
			return ErrConflict
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var u userRow
		e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, "id = ?", userKey(sc)).Error
		if e == nil && u.State == "active" {
			return ErrConflict
		}
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		k := make([]byte, 32)
		if _, e = rand.Read(k); e != nil {
			return e
		}
		l := lifecycleRow{ID: scopeKey(sc), UserKey: userKey(sc), AppKey: s.appKey(), State: "active", Revision: 1, Fence: 1, DocumentState: "new"}
		l.WrappedKey, e = s.cfg.Wrapper.Wrap(ctx, k, []byte("memory/dek/"+l.ID))
		if e != nil {
			return e
		}
		l.ScopePayload, e = s.encode(ctx, &l, "scope", sc)
		if e != nil {
			return e
		}
		if e = tx.Create(&l).Error; e != nil {
			return e
		}
		u = userRow{ID: l.UserKey, CurrentID: l.ID, State: "active"}
		if e = tx.Save(&u).Error; e != nil {
			return e
		}
		out = Operation{ID: opID, Status: "completed", LedgerRetained: true}
		return s.saveOperation(tx, &l, opID, "activate", sc, out)
	})
	return out, e
}
func (s *Service) SubmitCandidate(ctx context.Context, sc Scope, in CandidateInput) (Candidate, error) {
	if s.cfg.Backend == BackendNative && s.cfg.Native != nil {
		return Candidate{}, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Candidate
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		found, e := s.operation(tx, l, in.OperationID, "candidate", in, &out)
		if e != nil || found {
			return e
		}
		if in.Source != "trustedJava" || in.DataClass != "synthetic" || in.PolicyVersion != s.cfg.PolicyVersion || in.Risk || in.Sensitive || strings.TrimSpace(in.ConsentRef) == "" || len(in.ConsentRef) > 128 || in.TurnID == "" || in.SourceMessageID == "" || len(in.TurnID) > 128 || len(in.SourceMessageID) > 128 || in.Quote == "" || utf8.RuneCountInString(in.Quote) > 200 || utf8.RuneCountInString(in.UserText) > 6000 || in.UserText != in.Quote {
			return ErrInvalid
		}
		if in.Subject != "asQuoted" || in.Temporal != "asQuoted" || (in.Category != "event" && in.Category != "person" && in.Category != "preference" && in.Category != "situation") {
			return ErrInvalid
		}
		if e = s.consentAllowed(tx, l, in.ConsentRef); e != nil {
			return e
		}
		if e = s.bodyAllowed(tx, l, in.Quote); e != nil {
			return e
		}
		var count int64
		if e = tx.Model(&recordRow{}).Where("scope_key = ?", l.ID).Count(&count).Error; e != nil {
			return e
		}
		if count >= maxRecords {
			return ErrConflict
		}
		fingerprint, e := s.fingerprint(ctx, l, []string{in.SourceMessageID, in.TurnID, in.Quote})
		if e != nil {
			return e
		}
		var old recordRow
		e = tx.First(&old, "scope_key = ? AND kind = ? AND fingerprint = ?", l.ID, "candidate", fingerprint).Error
		if e == nil {
			if old.Status != "pending" {
				return ErrConflict
			}
			out, e = s.candidate(ctx, l, &old)
			if e != nil {
				return e
			}
			return s.saveOperation(tx, l, in.OperationID, "candidate", in, out)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		v := value{ReplaceFactID: in.ReplaceFactID, ExpectedVersion: in.ExpectedVersion, Content: in.Quote, Subject: in.Subject, Temporal: in.Temporal, Category: in.Category, ConsentRef: in.ConsentRef, PolicyVersion: in.PolicyVersion, SourceMessageID: in.SourceMessageID, TurnID: in.TurnID}
		if (in.ReplaceFactID == "") != (in.ExpectedVersion == 0) {
			return ErrInvalid
		}
		if in.ReplaceFactID != "" {
			old, e := s.record(tx, l, in.ReplaceFactID, "fact")
			if e != nil {
				return e
			}
			if old.Status != "active" || old.Version != in.ExpectedVersion {
				return ErrConflict
			}
			if e = s.invalidate(tx, l, old, "superseded"); e != nil {
				return e
			}
			if e = s.touch(tx, l); e != nil {
				return e
			}
		}
		r := recordRow{ID: uuid.NewString(), ScopeKey: l.ID, Kind: "candidate", Version: 1, Status: "pending", Fingerprint: fingerprint}
		r.Payload, e = s.encode(ctx, l, r.ID, v)
		if e != nil {
			return e
		}
		if e = tx.Create(&r).Error; e != nil {
			return e
		}
		out, e = s.candidate(ctx, l, &r)
		if e != nil {
			return e
		}
		return s.saveOperation(tx, l, in.OperationID, "candidate", in, out)
	})
	return out, e
}
func (s *Service) ClaimTask(ctx context.Context, sc Scope, opID string) (Task, error) {
	if s.cfg.Backend == BackendNative && s.cfg.Native != nil {
		return Task{}, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Task
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		found, e := s.operation(tx, l, opID, "claim", sc, &out)
		if e != nil || found {
			return e
		}
		var pending taskRow
		e = tx.First(&pending, "scope_key = ? AND status = ?", l.ID, "pending").Error
		if e == nil {
			if e = s.decode(ctx, l, pending.ID, pending.Payload, &out); e != nil {
				return e
			}
			return s.saveOperation(tx, l, opID, "claim", sc, out)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var taskCount int64
		if e = tx.Model(&taskRow{}).Where("scope_key = ?", l.ID).Count(&taskCount).Error; e != nil {
			return e
		}
		if taskCount >= 1000 {
			return ErrConflict
		}
		var rows []recordRow
		if e = tx.Where("scope_key = ? AND kind = ? AND status = ?", l.ID, "candidate", "pending").Order("created_at, id").Limit(100).Find(&rows).Error; e != nil {
			return e
		}
		out = Task{ID: uuid.NewString(), Candidates: []Candidate{}}
		if len(rows) == 0 {
			out.ID = ""
			return s.saveOperation(tx, l, opID, "claim", sc, out)
		}
		for i := range rows {
			c, e := s.candidate(ctx, l, &rows[i])
			if e != nil {
				return e
			}
			out.Candidates = append(out.Candidates, c)
		}
		payload, e := s.encode(ctx, l, out.ID, out)
		if e != nil {
			return e
		}
		if e = tx.Create(&taskRow{ID: out.ID, ScopeKey: l.ID, Fence: l.Fence, Status: "pending", Payload: payload}).Error; e != nil {
			return e
		}
		return s.saveOperation(tx, l, opID, "claim", sc, out)
	})
	return out, e
}
func (s *Service) Publish(ctx context.Context, sc Scope, in PublishInput) ([]Fact, error) {
	if s.cfg.Backend == BackendNative && s.cfg.Native != nil {
		return nil, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Fact{}
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		found, e := s.operation(tx, l, in.OperationID, "publish", in, &out)
		if e != nil || found {
			return e
		}
		if len(in.Items) > 100 || in.TaskID == "" || utf8.RuneCountInString(in.Profile) > 300 {
			return ErrInvalid
		}
		var task taskRow
		if e = tx.First(&task, "id = ? AND scope_key = ?", in.TaskID, l.ID).Error; e != nil {
			return ErrNotFound
		}
		if task.Status != "pending" || task.Fence != l.Fence {
			return ErrConflict
		}
		var snapshot Task
		if e = s.decode(ctx, l, task.ID, task.Payload, &snapshot); e != nil {
			return e
		}
		allowed := map[string]uint64{}
		for _, c := range snapshot.Candidates {
			allowed[c.ID] = c.Version
		}
		seen := map[string]bool{}
		var count int64
		if e = tx.Model(&recordRow{}).Where("scope_key = ?", l.ID).Count(&count).Error; e != nil {
			return e
		}
		if int(count)+len(in.Items) > maxRecords {
			return ErrConflict
		}
		for _, item := range in.Items {
			if seen[item.CandidateID] || allowed[item.CandidateID] != item.ExpectedVersion || item.ExpectedVersion == 0 {
				return ErrConflict
			}
			seen[item.CandidateID] = true
			r, e := s.record(tx, l, item.CandidateID, "candidate")
			if e != nil {
				return e
			}
			if r.Status != "pending" || r.Version != item.ExpectedVersion {
				return ErrConflict
			}
			var v value
			if e = s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
				return e
			}
			if v.ReplaceFactID != "" {
				old, e := s.record(tx, l, v.ReplaceFactID, "fact")
				if e != nil {
					return e
				}
				if old.Status != "superseded" || old.Version != v.ExpectedVersion+1 {
					return ErrConflict
				}
			}
			if item.Content != v.Content {
				return ErrInvalid
			}
			if e = s.bodyAllowed(tx, l, v.Content); e != nil {
				return e
			}
			fingerprint, e := s.fingerprint(ctx, l, []string{v.Content, v.Subject, v.Temporal, v.Category})
			if e != nil {
				return e
			}
			var blocked int64
			if e = tx.Model(&recordRow{}).Where("scope_key = ? AND kind = ? AND fingerprint = ?", l.ID, "tombstone", fingerprint).Count(&blocked).Error; e != nil {
				return e
			}
			if blocked > 0 {
				return ErrConflict
			}
			if e = s.consentAllowed(tx, l, v.ConsentRef); e != nil {
				return e
			}
			var old recordRow
			e = tx.First(&old, "scope_key = ? AND kind = ? AND fingerprint = ?", l.ID, "fact", fingerprint).Error
			if e == nil {
				if old.Status != "active" {
					return ErrConflict
				}
				f, e := s.fact(ctx, l, &old)
				if e != nil {
					return e
				}
				out = append(out, f)
			} else if errors.Is(e, gorm.ErrRecordNotFound) {
				frow := recordRow{ID: uuid.NewString(), ScopeKey: l.ID, Kind: "fact", Version: 1, Status: "active", SyncStatus: "pending", Fingerprint: fingerprint}
				frow.Payload, e = s.encode(ctx, l, frow.ID, v)
				if e != nil {
					return e
				}
				if e = tx.Create(&frow).Error; e != nil {
					return e
				}
				if e = s.enqueue(tx, l, &frow, "put", ""); e != nil {
					return e
				}
				f, e := s.fact(ctx, l, &frow)
				if e != nil {
					return e
				}
				out = append(out, f)
			} else {
				return e
			}
			r.Status = "published"
			r.Version++
			if e = tx.Save(r).Error; e != nil {
				return e
			}
		}
		task.Status = "completed"
		if e = tx.Save(&task).Error; e != nil {
			return e
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		if in.Profile != "" {
			if len(in.ProfileFactIDs) == 0 || len(in.ProfileFactIDs) > 100 {
				return ErrInvalid
			}
			contents := []string{}
			ids := map[string]bool{}
			for _, id := range in.ProfileFactIDs {
				if ids[id] {
					return ErrInvalid
				}
				ids[id] = true
				r, e := s.record(tx, l, id, "fact")
				if e != nil || r.Status != "active" {
					return ErrInvalid
				}
				f, e := s.fact(ctx, l, r)
				if e != nil {
					return e
				}
				contents = append(contents, f.Content)
			}
			if in.Profile != strings.Join(contents, "\n") {
				return ErrInvalid
			}
			l.Profile, e = s.encode(ctx, l, "profile", profileValue{in.Profile, in.ProfileFactIDs})
			if e != nil {
				return e
			}
			if e = tx.Save(l).Error; e != nil {
				return e
			}
		}
		return s.saveOperation(tx, l, in.OperationID, "publish", in, out)
	})
	return out, e
}
func (s *Service) List(ctx context.Context, sc Scope) ([]Fact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Fact{}
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		var rows []recordRow
		if e = tx.Where("scope_key = ? AND kind = ? AND status = ?", l.ID, "fact", "active").Order("updated_at DESC, id").Limit(maxRecords).Find(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			f, e := s.fact(ctx, l, &rows[i])
			if e != nil {
				return e
			}
			out = append(out, f)
		}
		return nil
	})
	return out, e
}
func (s *Service) Get(ctx context.Context, sc Scope, id string) (Fact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Fact
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		r, e := s.record(tx, l, id, "fact")
		if e != nil {
			return e
		}
		if r.Status != "active" {
			return ErrNotFound
		}
		out, e = s.fact(ctx, l, r)
		return e
	})
	return out, e
}
func (s *Service) Update(ctx context.Context, sc Scope, id string, expected uint64, content, opID string) (Fact, error) {
	if s.cfg.Backend == BackendNative && s.cfg.Native != nil {
		return Fact{}, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Fact
	req := []any{id, expected, content}
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		found, e := s.operation(tx, l, opID, "update", req, &out)
		if e != nil || found {
			return e
		}
		if strings.TrimSpace(content) == "" || utf8.RuneCountInString(content) > 200 {
			return ErrInvalid
		}
		if e = s.bodyAllowed(tx, l, content); e != nil {
			return e
		}
		r, e := s.record(tx, l, id, "fact")
		if e != nil {
			return e
		}
		if r.Status != "active" || r.Version != expected {
			return ErrConflict
		}
		var v value
		if e = s.decode(ctx, l, r.ID, r.Payload, &v); e != nil {
			return e
		}
		if r.ChunkID != "" {
			if e = s.enqueue(tx, l, r, "remove", r.ChunkID); e != nil {
				return e
			}
		}
		if content != v.Content {
			if e = s.blockBody(tx, l, v.Content); e != nil {
				return e
			}
			if e = tx.Create(&recordRow{ID: uuid.NewString(), ScopeKey: l.ID, Kind: "tombstone", Status: "deleted", Fingerprint: r.Fingerprint}).Error; e != nil {
				return e
			}
		}
		v.Content = content
		v.SourceMessageID = "user-edit:" + opID
		v.TurnID = opID
		r.Version++
		r.SyncStatus = "pending"
		r.ChunkID = ""
		r.Fingerprint, e = s.fingerprint(ctx, l, []string{v.Content, v.Subject, v.Temporal, v.Category})
		if e != nil {
			return e
		}
		var tombstones int64
		if e = tx.Model(&recordRow{}).Where("scope_key = ? AND kind IN ? AND fingerprint = ? AND status != ?", l.ID, []string{"fact", "tombstone"}, r.Fingerprint, "active").Count(&tombstones).Error; e != nil {
			return e
		}
		if tombstones > 0 {
			return ErrConflict
		}
		r.Payload, e = s.encode(ctx, l, r.ID, v)
		if e != nil {
			return e
		}
		if e = tx.Save(r).Error; e != nil {
			return e
		}
		if e = s.enqueue(tx, l, r, "put", ""); e != nil {
			return e
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		out, e = s.fact(ctx, l, r)
		if e != nil {
			return e
		}
		return s.saveOperation(tx, l, opID, "update", req, out)
	})
	return out, e
}
func (s *Service) Delete(ctx context.Context, sc Scope, id string, expected uint64, opID string) (Operation, error) {
	if !s.Capabilities().PhysicalDeleteSupported {
		return Operation{}, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Operation
	req := []any{id, expected}
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		found, e := s.operation(tx, l, opID, "delete", req, &out)
		if e != nil || found {
			return e
		}
		r, e := s.record(tx, l, id, "fact")
		if e != nil {
			return e
		}
		if r.Status != "active" || r.Version != expected {
			return ErrConflict
		}
		if e = s.invalidate(tx, l, r, "deleted"); e != nil {
			return e
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		out = Operation{ID: opID, Status: "pending", StoppedSupply: true, CleanupScope: "retrievalReplica", LedgerRetained: true}
		return s.saveOperation(tx, l, opID, "delete", req, out)
	})
	return out, e
}
func (s *Service) invalidate(tx *gorm.DB, l *lifecycleRow, r *recordRow, status string) error {
	var v value
	if e := s.decode(tx.Statement.Context, l, r.ID, r.Payload, &v); e != nil {
		return e
	}
	if e := s.blockBody(tx, l, v.Content); e != nil {
		return e
	}
	r.Status = status
	r.Version++
	r.SyncStatus = "pending"
	if e := tx.Save(r).Error; e != nil {
		return e
	}
	return s.enqueue(tx, l, r, "remove", r.ChunkID)
}
func (s *Service) Revoke(ctx context.Context, sc Scope, consentRef, opID string) (Operation, error) {
	if !s.Capabilities().ConsentRevokeSupported {
		return Operation{}, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Operation
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, true)
		if e != nil {
			return e
		}
		found, e := s.operation(tx, l, opID, "revoke", consentRef, &out)
		if e != nil || found {
			return e
		}
		if consentRef == "" || len(consentRef) > 128 {
			return ErrInvalid
		}
		fp, e := s.fingerprint(ctx, l, consentRef)
		if e != nil {
			return e
		}
		if e = tx.Create(&recordRow{ID: uuid.NewString(), ScopeKey: l.ID, Kind: "consent", Status: "revoked", Fingerprint: fp}).Error; e != nil {
			return e
		}
		var rows []recordRow
		if e = tx.Where("scope_key = ? AND kind IN ? AND status IN ?", l.ID, []string{"fact", "candidate"}, []string{"active", "pending"}).Limit(maxRecords).Find(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			var v value
			if e = s.decode(ctx, l, rows[i].ID, rows[i].Payload, &v); e != nil {
				return e
			}
			if v.ConsentRef == consentRef {
				if e = s.invalidate(tx, l, &rows[i], "revoked"); e != nil {
					return e
				}
			}
		}
		if e = s.touch(tx, l); e != nil {
			return e
		}
		out = Operation{ID: opID, Status: "pending", StoppedSupply: true, CleanupScope: "retrievalReplica", LedgerRetained: true}
		return s.saveOperation(tx, l, opID, "revoke", consentRef, out)
	})
	return out, e
}
func (s *Service) Deactivate(ctx context.Context, sc Scope, opID string) (Operation, error) {
	if !s.Capabilities().DeactivateSupported {
		return Operation{}, ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Operation
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, false)
		if e != nil {
			return e
		}
		if l.State == "cleaned" {
			var op operationRow
			if e = tx.First(&op, "id = ? AND action = ?", digest([]string{l.ID, opID}), "deactivate").Error; e != nil {
				return ErrConflict
			}
			out = Operation{ID: opID, Status: "completed", StoppedSupply: true, PhysicalCleanup: true, CleanupScope: "hubAndRetrievalReplica", LedgerRetained: false}
			return nil
		}
		found, e := s.operation(tx, l, opID, "deactivate", sc, &out)
		if e != nil || found {
			return e
		}
		if l.State != "active" {
			return ErrConflict
		}
		l.State = "frozen"
		l.Fence++
		if e = s.touch(tx, l); e != nil {
			return e
		}
		if e = tx.Model(&userRow{}).Where("id = ? AND current_id = ?", l.UserKey, l.ID).Update("state", "frozen").Error; e != nil {
			return e
		}
		if e = tx.Model(&taskRow{}).Where("scope_key = ? AND status = ?", l.ID, "pending").Update("status", "cancelled").Error; e != nil {
			return e
		}
		if e = tx.Model(&recordRow{}).Where("scope_key = ?", l.ID).Updates(map[string]any{"status": "deleted", "sync_status": "pending"}).Error; e != nil {
			return e
		}
		if e = s.enqueue(tx, l, nil, "drop", ""); e != nil {
			return e
		}
		out = Operation{ID: opID, Status: "pending", StoppedSupply: true, CleanupScope: "retrievalReplica", LedgerRetained: true}
		return s.saveOperation(tx, l, opID, "deactivate", sc, out)
	})
	return out, e
}
func (s *Service) GetOperation(ctx context.Context, sc Scope, opID string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Operation
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, e := s.life(tx, sc, false)
		if e != nil {
			return e
		}
		var op operationRow
		if e = tx.First(&op, "id = ? AND scope_key = ?", digest([]string{l.ID, opID}), l.ID).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return e
		}
		if l.State == "cleaned" {
			control := op.Action == "delete" || op.Action == "revoke" || op.Action == "deactivate"
			out = Operation{ID: opID, Status: "completed", StoppedSupply: control, PhysicalCleanup: control}
			if control {
				out.CleanupScope = "retrievalReplica"
			}
			if op.Action == "deactivate" {
				out.CleanupScope = "hubAndRetrievalReplica"
			}
			return nil
		}
		if op.Action == "delete" || op.Action == "revoke" || op.Action == "deactivate" {
			if e = s.decode(ctx, l, op.ID, op.Result, &out); e != nil {
				return e
			}
		} else {
			out = Operation{ID: opID, Status: "completed", LedgerRetained: true}
		}
		if out.ID == "" {
			out.ID = opID
			out.Status = "completed"
		}
		if out.StoppedSupply {
			out.CleanupScope = "retrievalReplica"
			out.LedgerRetained = true
			var n int64
			if e = tx.Model(&outboxRow{}).Where("scope_key = ? AND status NOT IN ?", l.ID, []string{"done", "cancelled"}).Count(&n).Error; e != nil {
				return e
			}
			out.PhysicalCleanup = n == 0
			out.Status = "pending"
			if n == 0 {
				out.Status = "completed"
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) Validate(ctx context.Context, sc Scope, version uint64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(sc); e != nil {
		return false, e
	}
	l, e := s.life(s.db.WithContext(ctx), sc, false)
	if e != nil {
		return false, e
	}
	return l.State == "active" && l.Revision == version, nil
}
func (s *Service) Recall(ctx context.Context, sc Scope, query string) (Context, error) {
	return s.RecallCategories(ctx, sc, query, nil)
}

func (s *Service) lockForRecall(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.mu.TryLock() {
			if err := ctx.Err(); err != nil {
				s.mu.Unlock()
				return err
			}
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (s *Service) RecallCategories(ctx context.Context, sc Scope, query string, categories []string) (Context, error) {
	if len(categories) > 4 {
		return Context{}, ErrInvalid
	}
	allowedCategories := map[string]bool{}
	for _, category := range categories {
		if category != "event" && category != "person" && category != "preference" && category != "situation" {
			return Context{}, ErrInvalid
		}
		allowedCategories[category] = true
	}
	if utf8.RuneCountInString(query) > 2000 {
		return Context{}, ErrInvalid
	}
	if err := s.lockForRecall(ctx); err != nil {
		return Context{}, err
	}
	l, e := s.life(s.db.WithContext(ctx), sc, false)
	if e != nil {
		s.mu.Unlock()
		return Context{}, e
	}
	out := Context{Version: l.Revision, Facts: []Fact{}, Status: "normal"}
	if l.State != "active" {
		s.mu.Unlock()
		return out, nil
	}
	revision := l.Revision
	doc := l.DocumentID
	s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Context{}, err
	}
	if doc == "" || s.cfg.Index == nil {
		out.Status = "degraded"
		return out, nil
	}
	searchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	hits, e := s.cfg.Index.Search(searchCtx, s.cfg.DatasetID, doc, query, 20)
	if err := ctx.Err(); err != nil {
		return Context{}, err
	}
	if e != nil {
		out.Status = "degraded"
		return out, nil
	}
	if err := s.lockForRecall(ctx); err != nil {
		return Context{}, err
	}
	defer s.mu.Unlock()
	l, e = s.life(s.db.WithContext(ctx), sc, false)
	if e != nil {
		return Context{}, e
	}
	if l.State != "active" || l.Revision != revision {
		out.Facts = []Fact{}
		out.Profile = ""
		out.Status = "stale"
		return out, nil
	}
	seen := map[string]bool{}
	size := 0
	for _, hit := range hits {
		if hit.DocumentID != doc {
			out.Facts = []Fact{}
			out.Status = "degraded"
			return out, nil
		}
		if hit.ChunkID == "" || seen[hit.ChunkID] {
			continue
		}
		seen[hit.ChunkID] = true
		var rows []recordRow
		if e = s.db.WithContext(ctx).Where("scope_key = ? AND kind = ? AND status = ? AND chunk_id = ? AND sync_status = ?", l.ID, "fact", "active", hit.ChunkID, "confirmed").Order("updated_at DESC, id").Limit(maxRecords).Find(&rows).Error; e != nil {
			return Context{}, e
		}
		for i := range rows {
			f, e := s.fact(ctx, l, &rows[i])
			if e != nil {
				return Context{}, e
			}
			if f.Content != hit.Content || (len(allowedCategories) > 0 && !allowedCategories[f.Category]) {
				continue
			}
			length := utf8.RuneCountInString(f.Content)
			if size+length > 200 {
				continue
			}
			out.Facts = append(out.Facts, f)
			size += length
			break
		}
		if len(out.Facts) >= 5 {
			break
		}
	}
	if len(l.Profile) > 0 && len(allowedCategories) == 0 {
		var p profileValue
		if e = s.decode(ctx, l, "profile", l.Profile, &p); e != nil {
			return Context{}, e
		}
		out.Profile = p.Content
	}
	if err := ctx.Err(); err != nil {
		return Context{}, err
	}
	return out, nil
}

func (s *Service) consentAllowed(tx *gorm.DB, l *lifecycleRow, ref string) error {
	fp, e := s.fingerprint(tx.Statement.Context, l, ref)
	if e != nil {
		return e
	}
	var count int64
	if e = tx.Model(&recordRow{}).Where("scope_key = ? AND kind = ? AND fingerprint = ?", l.ID, "consent", fp).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return ErrForbidden
	}
	return nil
}

func (s *Service) bodyAllowed(tx *gorm.DB, l *lifecycleRow, content string) error {
	fp, e := s.fingerprint(tx.Statement.Context, l, []string{"body", content})
	if e != nil {
		return e
	}
	var n int64
	if e = tx.Model(&recordRow{}).Where("scope_key = ? AND kind = ? AND fingerprint = ?", l.ID, "body_tombstone", fp).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return ErrConflict
	}
	return nil
}
func (s *Service) blockBody(tx *gorm.DB, l *lifecycleRow, content string) error {
	fp, e := s.fingerprint(tx.Statement.Context, l, []string{"body", content})
	if e != nil {
		return e
	}
	return tx.Create(&recordRow{ID: uuid.NewString(), ScopeKey: l.ID, Kind: "body_tombstone", Status: "deleted", Fingerprint: fp}).Error
}

// RejectCandidate consumes a pending candidate without publishing a fact.
// Its encrypted evidence remains under the lifecycle retention policy.
func (s *Service) RejectCandidate(ctx context.Context, sc Scope, candidateID string, expectedVersion uint64, reason, opID string) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Candidate
	request := []any{candidateID, expectedVersion, reason}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l, err := s.life(tx, sc, true)
		if err != nil {
			return err
		}
		replayed, err := s.operation(tx, l, opID, "reject", request, &out)
		if err != nil || replayed {
			return err
		}
		if reason != "notDurable" && reason != "duplicate" && reason != "policy" && reason != "unsupported" {
			return ErrInvalid
		}
		row, err := s.record(tx, l, candidateID, "candidate")
		if err != nil {
			return err
		}
		if row.Status != "pending" || row.Version != expectedVersion {
			return ErrConflict
		}
		var payload value
		if err = s.decode(ctx, l, row.ID, row.Payload, &payload); err != nil {
			return err
		}
		payload.RejectionReason = reason
		row.Payload, err = s.encode(ctx, l, row.ID, payload)
		if err != nil {
			return err
		}
		row.Status = "rejected"
		row.Version++
		if err = tx.Save(row).Error; err != nil {
			return err
		}
		// A task is an immutable snapshot. Discard pending snapshots so the next
		// claim includes only the candidates still available for publication.
		if err = tx.Model(&taskRow{}).Where("scope_key = ? AND status = ?", l.ID, "pending").Update("status", "cancelled").Error; err != nil {
			return err
		}
		out, err = s.candidate(ctx, l, row)
		if err != nil {
			return err
		}
		return s.saveOperation(tx, l, opID, "reject", request, out)
	})
	return out, err
}
