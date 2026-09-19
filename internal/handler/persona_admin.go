package handler

import (
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/emotion"
	"control-panel/internal/domain/reldynamics"
	"control-panel/internal/domain/subjectivememory"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// PersonaAdminHandler exposes read-only, run-scoped views of the four H6
// persona capability packs for administration and audit. Tenant identity
// always comes from auth context; only RunService.States and the pack read
// models are used, so no write path or cross-tenant leakage exists here.
type PersonaAdminHandler struct {
	runService *services.RunService
	belief     *services.BeliefService
}

func NewPersonaAdminHandler(runService *services.RunService, belief *services.BeliefService) *PersonaAdminHandler {
	return &PersonaAdminHandler{runService: runService, belief: belief}
}

type personaStateEntry struct {
	Namespace   string         `json:"namespace"`
	SchemaName  string         `json:"schemaName"`
	SubjectType string         `json:"subjectType"`
	SubjectID   string         `json:"subjectId"`
	Revision    uint64         `json:"revision"`
	Data        map[string]any `json:"data"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// PersonaState aggregates one Run's states for the four H6 namespaces,
// grouped per pack. States of other namespaces (generic run state) are not
// included to keep the view minimal.
func (h *PersonaAdminHandler) PersonaState(c *gin.Context) {
	tenantID, runID := tenant.GetTenantID(c), c.Param("id")
	states, err := h.runService.States(tenantID, runID)
	if err != nil {
		writeRunError(c, err)
		return
	}
	out := map[string]any{
		"runId":            runID,
		"emotion":          []personaStateEntry{},
		"belief":           []personaStateEntry{},
		"memory":           []personaStateEntry{},
		"relationDynamics": []personaStateEntry{},
	}
	bucket := map[string]string{
		emotion.Namespace:          "emotion",
		services.BeliefNamespace:   "belief",
		subjectivememory.Namespace: "memory",
		reldynamics.Namespace:      "relationDynamics",
	}
	for i := range states {
		key, ok := bucket[states[i].Namespace]
		if !ok {
			continue
		}
		out[key] = append(out[key].([]personaStateEntry), personaStateEntry{
			Namespace:   states[i].Namespace,
			SchemaName:  states[i].SchemaName,
			SubjectType: states[i].SubjectType,
			SubjectID:   states[i].SubjectID,
			Revision:    states[i].Revision,
			Data:        states[i].Data,
			UpdatedAt:   states[i].UpdatedAt,
		})
	}
	respondSuccess(c, out)
}

// BeliefDisputes lists factRefs held with materially different stances by ≥2
// agents in the run (the "两个角色对同一丑闻认知不同" admin view).
func (h *PersonaAdminHandler) BeliefDisputes(c *gin.Context) {
	if h.belief == nil {
		respondError(c, 501, "belief 能力尚未启用")
		return
	}
	disputes, err := h.belief.Disputes(tenant.GetTenantID(c), c.Param("id"))
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, disputes)
}
