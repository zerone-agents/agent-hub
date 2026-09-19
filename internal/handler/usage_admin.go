// UsageAdminHandler 是 H7.5 用量与运维的管理 API。
//
// 全部端点挂 /api/v1/admin 分组（RequireManager/RequireRole 由
// cmd/server/main.go 接线决定），按租户隔离查询（tenant.GetTenantID）。
// 导出端点为 CSV 流式响应；其余端点走统一 respondSuccess envelope。
package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UsageAdminHandler 依赖 UsageService（Record 采集与统计同源）。
type UsageAdminHandler struct {
	svc *services.UsageService
}

// NewUsageAdminHandler 构造 handler。
func NewUsageAdminHandler(svc *services.UsageService) *UsageAdminHandler {
	return &UsageAdminHandler{svc: svc}
}

// rangeFromQuery 解析 from/to（YYYY-MM-DD），非法值回 400 并返回 ok=false。
func (h *UsageAdminHandler) rangeFromQuery(c *gin.Context) (start, end time.Time, ok bool) {
	start, end, err := services.NormalizeRange(c.Query("from"), c.Query("to"))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return start, end, false
	}
	return start, end, true
}

func usagePage(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ = strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	return page, pageSize
}

// Summary 按 kind 汇总 + 按天趋势：GET /usage/summary?from=&to=
func (h *UsageAdminHandler) Summary(c *gin.Context) {
	start, end, ok := h.rangeFromQuery(c)
	if !ok {
		return
	}
	out, err := h.svc.Summary(tenant.GetTenantID(c), start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, out)
}

// ByExtension 扩展维度排行。
func (h *UsageAdminHandler) ByExtension(c *gin.Context) {
	h.dimension(c, h.svc.ByExtension)
}

// ByAgent Agent 维度排行。
func (h *UsageAdminHandler) ByAgent(c *gin.Context) {
	h.dimension(c, h.svc.ByAgent)
}

// ByRun Run 维度排行。
func (h *UsageAdminHandler) ByRun(c *gin.Context) {
	h.dimension(c, h.svc.ByRun)
}

// ByModel 模型维度排行。
func (h *UsageAdminHandler) ByModel(c *gin.Context) {
	h.dimension(c, h.svc.ByModel)
}

func (h *UsageAdminHandler) dimension(c *gin.Context,
	fn func(string, time.Time, time.Time, int, int) (*services.DimensionPage, error)) {
	start, end, ok := h.rangeFromQuery(c)
	if !ok {
		return
	}
	page, pageSize := usagePage(c)
	out, err := fn(tenant.GetTenantID(c), start, end, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, out)
}

// Errors 错误聚合列表：GET /usage/errors?from=&to=
func (h *UsageAdminHandler) Errors(c *gin.Context) {
	start, end, ok := h.rangeFromQuery(c)
	if !ok {
		return
	}
	rows, err := h.svc.Errors(tenant.GetTenantID(c), start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": rows})
}

// Storage 各表行数 + 字节估算：GET /usage/storage
func (h *UsageAdminHandler) Storage(c *gin.Context) {
	rows, err := h.svc.Storage(tenant.GetTenantID(c))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": rows})
}

// Health 合成健康状态：GET /usage/health
func (h *UsageAdminHandler) Health(c *gin.Context) {
	respondSuccess(c, h.svc.Health(tenant.GetTenantID(c)))
}

// Export CSV 明细导出：GET /usage/export?from=&to=&kind=
func (h *UsageAdminHandler) Export(c *gin.Context) {
	start, end, ok := h.rangeFromQuery(c)
	if !ok {
		return
	}
	filename := "usage_records.csv"
	if kind := strings.TrimSpace(c.Query("kind")); kind != "" {
		if len(kind) > 32 {
			respondError(c, http.StatusBadRequest, "kind 过长")
			return
		}
		filename = "usage_records_" + kind + ".csv"
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// 先写 UTF-8 BOM，Excel 可直接打开中文列头。
	c.Writer.Write([]byte("\xef\xbb\xbf"))
	if err := h.svc.ExportCSV(c.Writer, tenant.GetTenantID(c), start, end, c.Query("kind")); err != nil {
		log.Printf("[usage] export failed: %v", err)
	}
}

// ListBudgets 预算 + 进度：GET /usage/budgets
func (h *UsageAdminHandler) ListBudgets(c *gin.Context) {
	views, err := h.svc.Budgets(tenant.GetTenantID(c), time.Now())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": views})
}

// UpsertBudget 创建/更新预算：PUT /usage/budgets
func (h *UsageAdminHandler) UpsertBudget(c *gin.Context) {
	var in services.BudgetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON："+err.Error())
		return
	}
	b, err := h.svc.UpsertBudget(tenant.GetTenantID(c), in)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, b)
}

// DeleteBudget 删除预算：DELETE /usage/budgets/:id
func (h *UsageAdminHandler) DeleteBudget(c *gin.Context) {
	id, err := parseUsageID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid budget id")
		return
	}
	if err := h.svc.DeleteBudget(tenant.GetTenantID(c), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "预算不存在")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondMessage(c, http.StatusOK, "已删除")
}

// ListAlerts 告警规则列表：GET /usage/alerts
func (h *UsageAdminHandler) ListAlerts(c *gin.Context) {
	rows, err := h.svc.Alerts(tenant.GetTenantID(c))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": rows})
}

// UpsertAlert 创建/更新告警规则：PUT /usage/alerts
func (h *UsageAdminHandler) UpsertAlert(c *gin.Context) {
	var in services.AlertInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON："+err.Error())
		return
	}
	a, err := h.svc.UpsertAlert(tenant.GetTenantID(c), in)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, a)
}

// DeleteAlert 删除告警规则：DELETE /usage/alerts/:id
func (h *UsageAdminHandler) DeleteAlert(c *gin.Context) {
	id, err := parseUsageID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid alert id")
		return
	}
	if err := h.svc.DeleteAlert(tenant.GetTenantID(c), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "告警规则不存在")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondMessage(c, http.StatusOK, "已删除")
}

// AlertEvents 告警触发记录：GET /usage/alerts/events?limit=
func (h *UsageAdminHandler) AlertEvents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := h.svc.AlertEvents(tenant.GetTenantID(c), limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": rows})
}

// GetPricing 读 model_pricing：GET /usage/pricing
func (h *UsageAdminHandler) GetPricing(c *gin.Context) {
	raw, err := h.svc.GetPricingRaw()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{"key": services.ModelPricingKey, "value": raw})
}

// PutPricing 写 model_pricing：PUT /usage/pricing {"value":"{...}"}
func (h *UsageAdminHandler) PutPricing(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请求体必须是 JSON：{\"value\":\"{...}\"}")
		return
	}
	if err := h.svc.SetPricingRaw(req.Value); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "单价配置已更新")
}

func parseUsageID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
