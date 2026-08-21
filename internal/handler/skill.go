package handler

import (
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/skill"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// SkillHandler handles HTTP requests for skill CRUD and agent-skill associations.
type SkillHandler struct {
	service *services.SkillService
}

// NewSkillHandler creates a new SkillHandler with the given service.
func NewSkillHandler(service *services.SkillService) *SkillHandler {
	return &SkillHandler{service: service}
}

// ListPublic returns all skills for unauthenticated users.
func (h *SkillHandler) ListPublic(c *gin.Context) {
	skillType := c.Query("type")

	skills, err := h.service.ListAll(tenant.GetTenantID(c), skillType)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondSuccess(c, skills)
}

// GetPublic returns a single skill by name for unauthenticated users.
func (h *SkillHandler) GetPublic(c *gin.Context) {
	name := c.Param("name")

	sk, err := h.service.GetSkill(tenant.GetTenantID(c), name)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}

	respondSuccess(c, sk)
}

// Download generates and returns a presigned download URL for a skill file.
func (h *SkillHandler) Download(c *gin.Context) {
	name := c.Param("name")

	dto, err := h.service.Download(tenant.GetTenantID(c), name)
	if err != nil {
		if err == skill.ErrSkillNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		if err == skill.ErrSkillFileNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondSuccess(c, dto)
}

// GetSkillMd returns the SKILL.md entries of a skill for preview.
//
// Response shape: {entries: [{path, content}, ...]} where each entry is
// one SKILL.md found anywhere in the zip tree (matches SDK glob
// semantics). For a single-skill zip the slice has one element; for a
// bundle zip it has N. The frontend renders a tab switcher when N > 1.
func (h *SkillHandler) GetSkillMd(c *gin.Context) {
	name := c.Param("name")

	entries, err := h.service.GetSkillMd(tenant.GetTenantID(c), name)
	if err != nil {
		if err == skill.ErrSkillNotFound || err == skill.ErrSkillFileNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		if err == skill.ErrInvalidSkillFile {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondSuccess(c, gin.H{"entries": entries})
}

// ListAdmin returns all skills for admin users.
func (h *SkillHandler) ListAdmin(c *gin.Context) {
	skillType := c.Query("type")

	skills, err := h.service.ListAll(tenant.GetTenantID(c), skillType)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondSuccess(c, skills)
}

// Create handles skill creation with file upload via multipart form.
func (h *SkillHandler) Create(c *gin.Context) {
	name := c.PostForm("name")
	if name == "" {
		respondError(c, http.StatusBadRequest, "name 参数不能为空")
		return
	}

	title := c.PostForm("title")
	titleEn := c.PostForm("titleEn")
	description := c.PostForm("description")
	descriptionEn := c.PostForm("descriptionEn")
	skillType := c.PostForm("type")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, "文件上传失败: "+err.Error())
		return
	}
	defer file.Close()

	sk, err := h.service.CreateSkill(tenant.GetTenantID(c), &services.CreateSkillInput{
		Name:          name,
		Type:          skillType,
		Title:         title,
		TitleEn:       titleEn,
		Description:   description,
		DescriptionEn: descriptionEn,
		File:          file,
		FileSize:      header.Size,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	respondCreated(c, sk)
}

// updateSkillRequest binds the form fields for a skill update.
type updateSkillRequest struct {
	Title         string `form:"title"`
	TitleEn       string `form:"titleEn"`
	Description   string `form:"description"`
	DescriptionEn string `form:"descriptionEn"`
}

// Update handles skill modification with optional file replacement.
func (h *SkillHandler) Update(c *gin.Context) {
	name := c.Param("name")

	var req updateSkillRequest
	if err := c.ShouldBind(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil && err != http.ErrMissingFile {
		respondError(c, http.StatusBadRequest, "文件上传失败: "+err.Error())
		return
	}
	if file != nil {
		defer file.Close()
	}

	input := &services.UpdateSkillInput{
		Title:         req.Title,
		TitleEn:       req.TitleEn,
		Description:   req.Description,
		DescriptionEn: req.DescriptionEn,
	}
	if file != nil {
		input.File = file
		input.FileSize = header.Size
	}

	sk, err := h.service.UpdateSkill(tenant.GetTenantID(c), name, input)
	if err != nil {
		if err == skill.ErrSkillNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	respondSuccess(c, sk)
}

// Delete removes a skill by name.
func (h *SkillHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	if err := h.service.DeleteSkill(tenant.GetTenantID(c), name); err != nil {
		if err == skill.ErrSkillNotFound {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	respondMessage(c, http.StatusOK, "技能已删除")
}

// updateAgentSkillsRequest binds the JSON body for updating agent-skill associations.
type updateAgentSkillsRequest struct {
	SkillNames []string `json:"skillNames" binding:"required"`
}

// UpdateAgentSkills replaces the skill list for an agent.
func (h *SkillHandler) UpdateAgentSkills(c *gin.Context) {
	agentName := c.Param("name")
	var req updateAgentSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.UpdateAgentSkills(tenant.GetTenantID(c), agentName, req.SkillNames); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "Agent Skill 关系已更新")
}

// GetAgentSkills returns the skill names associated with an agent.
func (h *SkillHandler) GetAgentSkills(c *gin.Context) {
	agentName := c.Param("name")
	skillNames, err := h.service.GetAgentSkills(tenant.GetTenantID(c), agentName)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, skillNames)
}
