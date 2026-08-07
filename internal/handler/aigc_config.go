package handler

import (
	"net/http"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
)

type AigcConfigHandler struct {
	svc *services.AigcConfigService
}

func NewAigcConfigHandler(svc *services.AigcConfigService) *AigcConfigHandler {
	return &AigcConfigHandler{svc: svc}
}

func (h *AigcConfigHandler) Get(c *gin.Context) {
	dto, err := h.svc.Get()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
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
		respondError(c, http.StatusBadRequest, "uscc and companyName are required")
		return
	}
	dto, err := h.svc.Save(req.USCC, req.CompanyName)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, dto)
}

func (h *AigcConfigHandler) RotateKey(c *gin.Context) {
	dto, err := h.svc.RotateKey()
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, dto)
}

func (h *AigcConfigHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "aigc config deleted")
}
