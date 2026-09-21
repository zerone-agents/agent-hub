package handler

import (
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct{ q *services.AuditQuerier }

func NewAuditHandler(q *services.AuditQuerier) *AuditHandler { return &AuditHandler{q: q} }

// List serves GET /api/v1/admin/audit-logs（admin-only，spec §5.3）。
func (h *AuditHandler) List(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidPagination, "无效的分页参数")
		return
	}
	// page 上界：防 (page-1)*pageSize 整型溢出为负 offset（GORM 忽略负 offset
	// 会静默返回首页数据）与超大 offset 慢查询（终审 Minor#3）。
	if page > 1_000_000 {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidPagination, "无效的分页参数")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidPagination, "无效的分页参数")
		return
	}
	if pageSize > 100 {
		pageSize = 100
	}
	f := repository.AuditListFilter{
		Category: c.Query("category"),
		Action:   c.Query("action"),
		User:     c.Query("user"),
		Page:     page, PageSize: pageSize,
	}
	if s := c.Query("from"); s != "" {
		tv, err := time.Parse(time.RFC3339, s)
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidTimeRange, "无效的时间范围")
			return
		}
		f.From = &tv
	}
	if s := c.Query("to"); s != "" {
		tv, err := time.Parse(time.RFC3339, s)
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidTimeRange, "无效的时间范围")
			return
		}
		f.To = &tv
	}
	var snapshot *uint64
	if s := c.Query("snapshotId"); s != "" {
		v, err := strconv.ParseUint(s, 10, 64) // "0" 合法空集哨兵（spec §5.3）
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidSnapshotParameter, "无效的快照参数")
			return
		}
		snapshot = &v
	}
	res, err := h.q.List(tenant.GetTenantID(c), f, snapshot)
	if err != nil {
		respondError(c, http.StatusInternalServerError, ErrCodeAuditQueryFailed, "查询审计日志失败")
		return
	}
	respondSuccess(c, res)
}
