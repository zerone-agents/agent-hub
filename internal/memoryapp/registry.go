package memoryapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"control-panel/internal/memory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type applicationRow struct {
	ID             string  `gorm:"primaryKey;size:64"`
	TenantID       string  `gorm:"size:128"`
	TenantKey      string  `gorm:"index;size:64"`
	ApplicationID  string  `gorm:"size:128"`
	HubTenantID    string  `gorm:"size:128"`
	HubTenantKey   string  `gorm:"index;size:64"`
	Name           string  `gorm:"size:256"`
	Description    string  `gorm:"type:text"`
	PolicyVersion  string  `gorm:"size:128"`
	Status         string  `gorm:"size:24"`
	Origin         string  `gorm:"size:16"`
	MemoryID       string  `gorm:"size:128"`
	MemoryName     string  `gorm:"size:512"`
	ChatModel      string  `gorm:"size:256"`
	EmbeddingModel string  `gorm:"size:256"`
	OperationKey   *string `gorm:"uniqueIndex;size:64"`
	RequestHash    string  `gorm:"size:64"`
	TokenDigest    *string `gorm:"uniqueIndex;size:64"`
	TokenCipher    []byte  `gorm:"type:mediumblob"`
	ContainerKey   *string `gorm:"uniqueIndex;size:64"`
	Version        uint64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (applicationRow) TableName() string { return "memory_applications" }

type ownerRow struct {
	ID string `gorm:"primaryKey;size:64"`
}

func (ownerRow) TableName() string { return "memory_application_owners" }

// Reservations outlive configured credential/container rotations. Otherwise an
// old app token still accepted by another instance could become a manager key.
type credentialRow struct {
	ID       string `gorm:"primaryKey;size:64"`
	Purpose  string `gorm:"size:16"`
	OwnerKey string `gorm:"size:64"`
}

func (credentialRow) TableName() string { return "memory_application_credentials" }

type containerRow struct {
	ID       string `gorm:"primaryKey;size:64"`
	OwnerKey string `gorm:"size:64"`
}

func (containerRow) TableName() string { return "memory_application_containers" }

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&ownerRow{}, &applicationRow{}, &credentialRow{}, &containerRow{})
}

func claimCredential(tx *gorm.DB, digest, purpose, owner string) error {
	row := credentialRow{ID: digest, Purpose: purpose, OwnerKey: owner}
	if tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error != nil {
		return ErrUnavailable
	}
	var existing credentialRow
	if tx.First(&existing, "id = ?", digest).Error != nil {
		return ErrUnavailable
	}
	if existing.Purpose != purpose || existing.OwnerKey != owner {
		return ErrConflict
	}
	return nil
}
func claimContainer(tx *gorm.DB, key, owner string) error {
	row := containerRow{ID: key, OwnerKey: owner}
	if tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error != nil {
		return ErrUnavailable
	}
	var existing containerRow
	if tx.First(&existing, "id = ?", key).Error != nil {
		return ErrUnavailable
	}
	if existing.OwnerKey != owner {
		return ErrConflict
	}
	return nil
}

type Registry struct {
	db      *gorm.DB
	wrapper memory.KeyWrapper
	banks   BankProvisioner
}

func NewRegistry(db *gorm.DB, wrapper memory.KeyWrapper, banks BankProvisioner) (*Registry, error) {
	if db == nil || wrapper == nil || banks == nil {
		return nil, ErrInvalid
	}
	return &Registry{db: db, wrapper: wrapper, banks: banks}, nil
}

func hash(parts ...string) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func TokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:", r)) {
			return false
		}
	}
	return true
}
func textOK(s string, max int, required bool) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max || (required && strings.TrimSpace(s) == "") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func appView(r applicationRow) Application {
	return Application{TenantID: r.TenantID, ApplicationID: r.ApplicationID, HubTenantID: r.HubTenantID, Name: r.Name, Description: r.Description, PolicyVersion: r.PolicyVersion, Status: r.Status, MemoryID: r.MemoryID, MemoryName: r.MemoryName, ChatModel: r.ChatModel, EmbeddingModel: r.EmbeddingModel, Origin: r.Origin, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func binding(r applicationRow) Binding {
	return Binding{TenantID: r.TenantID, ApplicationID: r.ApplicationID, HubTenantID: r.HubTenantID, PolicyVersion: r.PolicyVersion, MemoryID: r.MemoryID, Name: r.Name, Status: r.Status}
}
func (r *Registry) lockOwner(tx *gorm.DB, tenant string) error {
	row := ownerRow{ID: hash(tenant)}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", row.ID).Error
}

// ReserveStatic keeps configured identities, credentials and containers from
// being reused by managed applications, including across Hub instances.
// A reservation is never an authentication source for a removed static client.
func (r *Registry) ReserveStatic(ctx context.Context, entries []StaticReservation) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, s := range entries {
			if !validID(s.TenantID) || !validID(s.ApplicationID) || len(s.TokenDigest) != 64 || s.ContainerID == "" {
				return ErrInvalid
			}
			if err := r.lockOwner(tx, s.TenantID); err != nil {
				return ErrUnavailable
			}
			id := hash(s.TenantID, s.ApplicationID)
			container := hash(s.Backend, strings.ToLower(s.ContainerID))
			if err := claimCredential(tx, s.TokenDigest, "application", id); err != nil {
				return err
			}
			if err := claimContainer(tx, container, id); err != nil {
				return err
			}
			var old applicationRow
			err := tx.First(&old, "id = ?", id).Error
			if err == nil {
				if old.Origin != "static" {
					return ErrConflict
				}
				if old.TokenDigest != nil {
					if err := claimCredential(tx, *old.TokenDigest, "application", id); err != nil {
						return err
					}
				}
				if old.ContainerKey != nil {
					if err := claimContainer(tx, *old.ContainerKey, id); err != nil {
						return err
					}
				}
				// Configuration remains authoritative; rotation must still satisfy
				// the database uniqueness constraints held by other applications.
				if err = tx.Model(&old).Updates(map[string]any{"hub_tenant_id": s.HubTenantID, "hub_tenant_key": hash(s.HubTenantID), "container_key": container, "token_digest": s.TokenDigest}).Error; err != nil {
					return ErrConflict
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUnavailable
			}
			row := applicationRow{ID: id, TenantID: s.TenantID, TenantKey: hash(s.TenantID), HubTenantKey: hash(s.HubTenantID), ApplicationID: s.ApplicationID, HubTenantID: s.HubTenantID, Origin: "static", Status: "reserved", TokenDigest: &s.TokenDigest, ContainerKey: &container, Version: 1}
			if tx.Create(&row).Error != nil {
				return ErrConflict
			}
		}
		return nil
	})
}

func (r *Registry) Create(ctx context.Context, owner Owner, in CreateInput) (CreateResult, error) {
	return r.create(ctx, owner, in, false)
}

// CreateWithModels binds console-selected models to the idempotent request.
// Legacy machine callers retain their original configuration-change semantics.
func (r *Registry) CreateWithModels(ctx context.Context, owner Owner, in CreateInput) (CreateResult, error) {
	return r.create(ctx, owner, in, true)
}

func (r *Registry) create(ctx context.Context, owner Owner, in CreateInput, selectedModels bool) (CreateResult, error) {
	if !validID(owner.TenantID) || !validID(owner.HubTenantID) || !validID(in.ApplicationID) || !validID(in.OperationID) || !validID(in.PolicyVersion) || !textOK(in.Name, 64, true) || !textOK(in.Description, 1024, false) || !textOK(owner.ChatModel, 128, true) || !textOK(owner.EmbeddingModel, 128, true) || owner.MaxApplications < 1 || owner.MaxApplications > 1000 {
		return CreateResult{}, ErrInvalid
	}
	request, _ := json.Marshal(in)
	requestHash := hash(string(request))
	if selectedModels {
		requestHash = hash(string(request), owner.ChatModel, owner.EmbeddingModel)
	}
	id := hash(owner.TenantID, in.ApplicationID)
	opKey := hash(owner.TenantID, in.OperationID)
	var row applicationRow
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.lockOwner(tx, owner.TenantID); err != nil {
			return ErrUnavailable
		}
		err := tx.First(&row, "id = ?", id).Error
		if err == nil {
			if row.Origin != "managed" || row.OperationKey == nil || *row.OperationKey != opKey || row.RequestHash != requestHash || row.HubTenantID != owner.HubTenantID {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		var count int64
		if tx.Model(&applicationRow{}).Where("tenant_key = ?", hash(owner.TenantID)).Count(&count).Error != nil {
			return ErrUnavailable
		}
		if count >= int64(owner.MaxApplications) {
			return ErrQuota
		}
		token, err := randomHex(32)
		if err != nil {
			return ErrUnavailable
		}
		marker, err := randomHex(16)
		if err != nil {
			return ErrUnavailable
		}
		cipher, err := r.wrapper.Wrap(ctx, []byte(token), []byte("memory/application-token/"+id+"/"+opKey))
		if err != nil {
			return ErrUnavailable
		}
		digest := TokenDigest(token)
		if err := claimCredential(tx, digest, "application", id); err != nil {
			return err
		}
		row = applicationRow{ID: id, TenantID: owner.TenantID, TenantKey: hash(owner.TenantID), HubTenantKey: hash(owner.HubTenantID), ApplicationID: in.ApplicationID, HubTenantID: owner.HubTenantID, Name: in.Name, Description: in.Description, PolicyVersion: in.PolicyVersion, Origin: "managed", Status: "provisioning", MemoryName: "hub-" + marker, ChatModel: owner.ChatModel, EmbeddingModel: owner.EmbeddingModel, OperationKey: &opKey, RequestHash: requestHash, TokenDigest: &digest, TokenCipher: cipher, Version: 1}
		if tx.Create(&row).Error != nil {
			return ErrConflict
		}
		created = true
		return nil
	})
	if err != nil {
		return CreateResult{}, err
	}
	// Committing the reservation grants exactly this caller one dispatch.
	// Every other caller, including after process restart, can only reconcile.
	if created {
		bank, bankErr := r.banks.Create(ctx, spec(row))
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if bankErr == nil {
			row, err = r.bind(finishCtx, row, bank)
		} else {
			row, err = r.markUnknown(finishCtx, row)
		}
	} else if row.MemoryID == "" {
		row, err = r.reconcile(ctx, row)
	}
	if err != nil {
		return CreateResult{}, err
	}
	secret, err := r.wrapper.Unwrap(ctx, row.TokenCipher, []byte("memory/application-token/"+row.ID+"/"+*row.OperationKey))
	if err != nil {
		return CreateResult{}, ErrUnavailable
	}
	out := CreateResult{Application: appView(row), ApplicationToken: string(secret)}
	if row.MemoryID == "" {
		return out, ErrUnknown
	}
	return out, nil
}
func spec(row applicationRow) BankSpec {
	return BankSpec{Name: row.MemoryName, ChatModel: row.ChatModel, EmbeddingModel: row.EmbeddingModel}
}
func (r *Registry) markUnknown(ctx context.Context, row applicationRow) (applicationRow, error) {
	if r.db.WithContext(ctx).Model(&applicationRow{}).Where("id = ? AND memory_id = ? AND status = ?", row.ID, "", "provisioning").Updates(map[string]any{"status": "unknown", "version": gorm.Expr("version + 1")}).Error != nil {
		return row, ErrUnavailable
	}
	return r.load(ctx, row.TenantID, row.ApplicationID)
}
func (r *Registry) bind(ctx context.Context, row applicationRow, bank Bank) (applicationRow, error) {
	if !validID(bank.ID) || bank.Name != row.MemoryName || bank.ChatModel != row.ChatModel || bank.EmbeddingModel != row.EmbeddingModel {
		return r.markUnknown(ctx, row)
	}
	container := hash(memory.BackendNative, strings.ToLower(bank.ID))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current applicationRow
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", row.ID).Error != nil {
			return ErrUnavailable
		}
		if current.MemoryID != "" {
			if current.MemoryID != bank.ID {
				return ErrConflict
			}
			return nil
		}
		if err := claimContainer(tx, container, row.ID); err != nil {
			return err
		}
		if tx.Model(&current).Updates(map[string]any{"memory_id": bank.ID, "container_key": container, "status": "active", "version": gorm.Expr("version + 1")}).Error != nil {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return row, err
	}
	return r.load(ctx, row.TenantID, row.ApplicationID)
}
func (r *Registry) reconcile(ctx context.Context, row applicationRow) (applicationRow, error) {
	if row.MemoryID != "" {
		return row, nil
	}
	bank, err := r.banks.Find(ctx, spec(row))
	if err != nil {
		return r.markUnknown(ctx, row)
	}
	return r.bind(ctx, row, bank)
}
func (r *Registry) load(ctx context.Context, tenant, app string) (applicationRow, error) {
	var row applicationRow
	err := r.db.WithContext(ctx).Where("id = ? AND origin = ?", hash(tenant, app), "managed").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, ErrUnavailable
	}
	return row, nil
}
func (r *Registry) Get(ctx context.Context, tenant, app string, reconcile bool) (Application, error) {
	if !validID(tenant) || !validID(app) {
		return Application{}, ErrInvalid
	}
	row, err := r.load(ctx, tenant, app)
	if err == nil && reconcile {
		row, err = r.reconcile(ctx, row)
	}
	if err != nil {
		return Application{}, err
	}
	return appView(row), nil
}
func (r *Registry) List(ctx context.Context, tenant string, offset, limit int) ([]Application, error) {
	if !validID(tenant) || offset < 0 || offset > 100000 || limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	var rows []applicationRow
	if r.db.WithContext(ctx).Where("tenant_key = ? AND origin = ?", hash(tenant), "managed").Order("created_at ASC, id ASC").Offset(offset).Limit(limit).Find(&rows).Error != nil {
		return nil, ErrUnavailable
	}
	out := make([]Application, 0, len(rows))
	for _, row := range rows {
		out = append(out, appView(row))
	}
	return out, nil
}
func (r *Registry) Update(ctx context.Context, tenant, app string, in UpdateInput) (Application, error) {
	if !validID(tenant) || !validID(app) || in.ExpectedVersion == 0 || (in.Name == nil && in.Description == nil && in.Enabled == nil) || (in.Name != nil && !textOK(*in.Name, 64, true)) || (in.Description != nil && !textOK(*in.Description, 1024, false)) {
		return Application{}, ErrInvalid
	}
	row, err := r.load(ctx, tenant, app)
	if err != nil {
		return Application{}, err
	}
	if row.MemoryID == "" {
		return Application{}, ErrConflict
	}
	changes := map[string]any{"version": gorm.Expr("version + 1")}
	if in.Name != nil {
		changes["name"] = *in.Name
	}
	if in.Description != nil {
		changes["description"] = *in.Description
	}
	if in.Enabled != nil {
		if *in.Enabled {
			changes["status"] = "active"
		} else {
			changes["status"] = "disabled"
		}
	}
	result := r.db.WithContext(ctx).Model(&applicationRow{}).Where("id = ? AND version = ?", row.ID, in.ExpectedVersion).Updates(changes)
	if result.Error != nil {
		return Application{}, ErrUnavailable
	}
	if result.RowsAffected != 1 {
		return Application{}, ErrConflict
	}
	return r.Get(ctx, tenant, app, false)
}
func (r *Registry) Authenticate(ctx context.Context, token string) (Binding, error) {
	if len(token) != 64 {
		return Binding{}, ErrNotFound
	}
	var row applicationRow
	err := r.db.WithContext(ctx).Where("token_digest = ? AND origin = ?", TokenDigest(token), "managed").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Binding{}, ErrNotFound
	}
	if err != nil {
		return Binding{}, ErrUnavailable
	}
	if row.Status != "active" || row.MemoryID == "" {
		return Binding{}, ErrDisabled
	}
	return binding(row), nil
}

// Reject accidental promotion of an existing application credential during
// bootstrap. Manager credentials have their own configuration and purpose.
func (r *Registry) CheckManagerToken(ctx context.Context, token string) error {
	var count int64
	if r.db.WithContext(ctx).Model(&applicationRow{}).Where("token_digest = ?", TokenDigest(token)).Count(&count).Error != nil {
		return ErrUnavailable
	}
	if count != 0 {
		return ErrConflict
	}
	if r.db.WithContext(ctx).Model(&credentialRow{}).Where("id = ? AND purpose <> ?", TokenDigest(token), "manager").Count(&count).Error != nil {
		return ErrUnavailable
	}
	if count != 0 {
		return ErrConflict
	}
	return nil
}

func (r *Registry) ReserveManager(ctx context.Context, token, tenant string) error {
	if !validID(tenant) || len(token) < 32 {
		return ErrInvalid
	}
	if err := r.CheckManagerToken(ctx, token); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return claimCredential(tx, TokenDigest(token), "manager", hash(tenant)) })
}
func (r *Registry) ForHubTenant(ctx context.Context, tenant string) ([]Binding, error) {
	var rows []applicationRow
	if r.db.WithContext(ctx).Where("hub_tenant_key = ? AND origin = ? AND memory_id <> ?", hash(tenant), "managed", "").Order("id ASC").Find(&rows).Error != nil {
		return nil, ErrUnavailable
	}
	out := make([]Binding, 0, len(rows))
	for _, row := range rows {
		out = append(out, binding(row))
	}
	return out, nil
}

// ApplicationsForHubTenant includes unfinished provisioning receipts, but never
// credentials. Only completed bindings are admitted by ForHubTenant.
func (r *Registry) ApplicationsForHubTenant(ctx context.Context, tenant string) ([]Application, error) {
	if !validID(tenant) {
		return nil, ErrInvalid
	}
	var rows []applicationRow
	if r.db.WithContext(ctx).Where("hub_tenant_key = ? AND origin = ?", hash(tenant), "managed").Order("created_at ASC, id ASC").Find(&rows).Error != nil {
		return nil, ErrUnavailable
	}
	out := make([]Application, 0, len(rows))
	for _, row := range rows {
		out = append(out, appView(row))
	}
	return out, nil
}
