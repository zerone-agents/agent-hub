package handler

import (
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type AigcConfigHandler struct {
	svc   *services.AigcConfigService
	audit *services.AuditRecorder
}

func NewAigcConfigHandler(svc *services.AigcConfigService, ar *services.AuditRecorder) *AigcConfigHandler {
	return &AigcConfigHandler{svc: svc, audit: ar}
}

func (h *AigcConfigHandler) Get(c *gin.Context) {
	tenantID := tenant.GetTenantID(c)
	dto, err := h.svc.Get(tenantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	respondSuccess(c, dto)
}

type saveAigcConfigReq struct {
	USCC        string `json:"uscc" binding:"required"`
	CompanyName string `json:"companyName" binding:"required"`
}

func (h *AigcConfigHandler) Save(c *gin.Context) {
	var req saveAigcConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeAigcFieldsRequired, "uscc and companyName are required")
		return
	}
	tenantID := tenant.GetTenantID(c)
	dto, rcpt, err := h.svc.Save(tenantID, req.USCC, req.CompanyName)
	if err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
		return
	}
	// 幂等保存（ChangedFields=[]）仍记录事件：操作意图本身可审计（Task 9 语义）。
	h.audit.AigcSaved(c, rcpt.ChangedFields)
	respondSuccess(c, dto)
}

func (h *AigcConfigHandler) RotateKey(c *gin.Context) {
	dto, err := h.svc.RotateKey(tenant.GetTenantID(c))
	if err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
		return
	}
	h.audit.Simple(c, audit.ActionAigcRotateKey, audit.TargetAigcConfig, tenant.GetTenantID(c), "")
	respondSuccess(c, dto)
}

func (h *AigcConfigHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(tenant.GetTenantID(c)); err != nil {
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	h.audit.Simple(c, audit.ActionAigcDelete, audit.TargetAigcConfig, tenant.GetTenantID(c), "")
	respondMessage(c, http.StatusOK, "aigc config deleted")
}
