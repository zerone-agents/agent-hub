package repository

import (
	"time"

	"control-panel/internal/domain/audit"

	"gorm.io/gorm"
)

// AuditListFilter：items 与 COUNT 必须复用同一条件组合（spec §5.3）。
type AuditListFilter struct {
	TenantID   string
	Category   string
	Action     string
	User       string // UserName/UserID 模糊
	From, To   *time.Time
	SnapshotID *uint64 // nil = 不加快照上界
	Page       int
	PageSize   int
}

type AuditRepository struct{ db *gorm.DB }

func NewAuditRepository(db *gorm.DB) *AuditRepository { return &AuditRepository{db: db} }

func (r *AuditRepository) Create(e *audit.Log) error { return r.db.Create(e).Error }

// MaxID 返回租户全集 MAX(id)（不受 category/action/user 筛选影响，spec §5.3）；空表 0。
func (r *AuditRepository) MaxID(tenantID string) (uint64, error) {
	var max struct{ Max int64 }
	if err := r.db.Model(&audit.Log{}).Where("tenant_id = ?", tenantID).
		Select("COALESCE(MAX(id), 0) AS max").Scan(&max).Error; err != nil {
		return 0, err
	}
	return uint64(max.Max), nil
}

func (r *AuditRepository) applyFilter(q *gorm.DB, f AuditListFilter) *gorm.DB {
	q = q.Where("tenant_id = ?", f.TenantID)
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.User != "" {
		like := "%" + f.User + "%"
		q = q.Where("user_name LIKE ? OR user_id LIKE ?", like, like)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}
	if f.SnapshotID != nil {
		q = q.Where("id <= ?", *f.SnapshotID)
	}
	return q
}

// List：COUNT 与 items 复用 applyFilter（同一 WHERE），仅分页/排序子句不同。
func (r *AuditRepository) List(f AuditListFilter) ([]audit.Log, int64, error) {
	var total int64
	if err := r.applyFilter(r.db.Model(&audit.Log{}), f).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []audit.Log
	err := r.applyFilter(r.db.Model(&audit.Log{}), f).
		Order("created_at DESC, id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Find(&logs).Error
	return logs, total, err
}
