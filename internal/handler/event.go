package handler

import (
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type EventHandler struct{ service *services.EventService }

func NewEventHandler(service *services.EventService) *EventHandler {
	return &EventHandler{service: service}
}

func (h *EventHandler) ListRun(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	root := c.Query("rootEventId")
	var rows []services.EventRecord
	var err error
	if root == "" {
		rows, err = h.service.ListRun(tenant.GetTenantID(c), c.Param("id"), limit)
	} else {
		rows, err = h.service.ListChain(tenant.GetTenantID(c), c.Param("id"), root, limit)
	}
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}
