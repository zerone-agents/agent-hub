package services

import (
	"encoding/json"
	"strconv"
	"time"

	"control-panel/internal/domain/audit"
	repository "control-panel/internal/infrastructure/persistence"
)

// AuditItemDTO：id 十进制字符串、detail JSON 对象（spec §4 DB/API 分离）。
type AuditItemDTO struct {
	ID         string           `json:"id"`
	TenantID   string           `json:"tenantId"`
	UserID     string           `json:"userId"`
	UserName   string           `json:"userName"`
	Category   audit.Category   `json:"category"`
	Action     audit.Action     `json:"action"`
	TargetType audit.TargetType `json:"targetType"`
	TargetID   string           `json:"targetId"`
	TargetName string           `json:"targetName"`
	Status     audit.Status     `json:"status"`
	Detail     json.RawMessage  `json:"detail"`
	RemoteIP   string           `json:"remoteIp"`
	UserAgent  string           `json:"userAgent"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type AuditListResult struct {
	Items      []AuditItemDTO `json:"items"`
	Total      int64          `json:"total"`
	SnapshotID string         `json:"snapshotId"`
}

type AuditQuerier struct{ repo *repository.AuditRepository }

func NewAuditQuerier(repo *repository.AuditRepository) *AuditQuerier {
	return &AuditQuerier{repo: repo}
}

// List：snapshotID 为 nil 时服务端捕获租户全集 MAX(id)（空表 → 0 哨兵）；
// 之后 items 与 COUNT 复用同一含 id<=snapshotId 的 WHERE（spec §5.3）。
func (q *AuditQuerier) List(tenantID string, f repository.AuditListFilter, snapshotID *uint64) (*AuditListResult, error) {
	if snapshotID == nil {
		max, err := q.repo.MaxID(tenantID)
		if err != nil {
			return nil, err
		}
		snapshotID = &max
	}
	f.TenantID = tenantID
	f.SnapshotID = snapshotID
	logs, total, err := q.repo.List(f)
	if err != nil {
		return nil, err
	}
	items := make([]AuditItemDTO, 0, len(logs))
	for _, l := range logs {
		detail := json.RawMessage(nil)
		if l.Detail != "" {
			detail = json.RawMessage(l.Detail)
		}
		items = append(items, AuditItemDTO{
			ID: strconv.FormatUint(l.ID, 10), TenantID: l.TenantID, UserID: l.UserID,
			UserName: l.UserName, Category: l.Category, Action: l.Action,
			TargetType: l.TargetType, TargetID: l.TargetID, TargetName: l.TargetName,
			Status: l.Status, Detail: detail, RemoteIP: l.RemoteIP,
			UserAgent: l.UserAgent, CreatedAt: l.CreatedAt,
		})
	}
	return &AuditListResult{Items: items, Total: total, SnapshotID: strconv.FormatUint(*snapshotID, 10)}, nil
}
