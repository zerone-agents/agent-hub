package handler

import (
	"errors"
	"log"
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type ToolHandler struct {
	service *services.ToolService
}

func NewToolHandler(service *services.ToolService) *ToolHandler {
	return &ToolHandler{service: service}
}

// respondToolError 映射 Tool 领域错误（issue #88「可行动的错误层级」）：
// 校验/领域冲突 → 400；仍被挂载 → 409（带 data.agents）；不存在 → 404；
// 存储未配置 → 503（可行动的配置提示）；基础设施故障（OSS/DB）→ 500 中性
// 文案，完整错误链只在服务端日志（英文诊断包装已由 service 层保证）。
func respondToolError(c *gin.Context, err error) {
	var inUse *agent.ToolInUseError
	if errors.As(err, &inUse) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": inUse.Error(), "data": gin.H{"agents": inUse.Agents}})
		return
	}
	switch {
	case errors.Is(err, agent.ErrToolNotFound), errors.Is(err, agent.ErrAgentNotFound):
		respondError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, agent.ErrToolStorageDisabled):
		respondError(c, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, agent.ErrInvalidToolName),
		errors.Is(err, agent.ErrToolNameExists),
		errors.Is(err, agent.ErrToolIsBuiltin),
		errors.Is(err, agent.ErrInvalidToolFile),
		errors.Is(err, agent.ErrToolFileEmpty),
		errors.Is(err, agent.ErrToolFileTooLarge),
		errors.Is(err, agent.ErrToolArtifactMissing):
		respondError(c, http.StatusBadRequest, err.Error())
	default:
		log.Printf("[ToolHandler] internal error: %v", err)
		respondError(c, http.StatusInternalServerError, "服务器内部错误，请稍后重试")
	}
}

func (h *ToolHandler) List(c *gin.Context) {
	tools, err := h.service.ListAll(tenant.GetTenantID(c))
	if err != nil {
		// DB 故障经 default 桶 → 500 中性文案（expert review round 4：
		// 不再直写 err.Error() 泄漏 DB/驱动诊断）。
		respondToolError(c, err)
		return
	}
	respondSuccess(c, tools)
}

func (h *ToolHandler) Get(c *gin.Context) {
	t, err := h.service.GetByName(tenant.GetTenantID(c), c.Param("name"))
	if err != nil {
		// not-found → 404 语义不变；DB 故障经 default 桶如实 500
		// （expert review round 3：不再一律伪装 404）。
		respondToolError(c, err)
		return
	}
	respondSuccess(c, t)
}

// parseToolFile 解析 multipart file 字段为 ToolFileInput（Create 与
// UploadFile 单一构造点）；缺失时写 400 响应并返回 nil。调用方负责在成功
// 路径 defer 关闭（返回的 closeFn）。
func parseToolFile(c *gin.Context) (input *services.ToolFileInput, closeFn func()) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, "工具文件不能为空（仅支持 .ts/.mts/.js/.mjs 单文件，≤5 MiB）")
		return nil, nil
	}
	return &services.ToolFileInput{
		FileName: header.Filename,
		File:     file,
		FileSize: header.Size,
	}, func() { _ = file.Close() }
}

// Create 处理自定义工具创建（multipart：name/title/description/file）。
func (h *ToolHandler) Create(c *gin.Context) {
	name := c.PostForm("name")
	if name == "" {
		respondError(c, http.StatusBadRequest, "name 参数不能为空")
		return
	}
	in, closeFn := parseToolFile(c)
	if in == nil {
		return
	}
	defer closeFn()

	t, err := h.service.CreateCustomTool(tenant.GetTenantID(c), &services.CreateCustomToolInput{
		Name:          name,
		Title:         c.PostForm("title"),
		Description:   c.PostForm("description"),
		ToolFileInput: *in,
	})
	if err != nil {
		respondToolError(c, err)
		return
	}
	respondCreated(c, t)
}

// Update 仅更新展示元数据（JSON：title/description），builtin 拒绝。
func (h *ToolHandler) Update(c *gin.Context) {
	var input services.UpdateToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	t, err := h.service.Update(tenant.GetTenantID(c), c.Param("name"), &input)
	if err != nil {
		respondToolError(c, err)
		return
	}
	respondSuccess(c, t)
}

// UploadFile 补传（missing）与替换（ready）共用：multipart file。
func (h *ToolHandler) UploadFile(c *gin.Context) {
	in, closeFn := parseToolFile(c)
	if in == nil {
		return
	}
	defer closeFn()

	t, err := h.service.UploadToolFile(tenant.GetTenantID(c), c.Param("name"), in)
	if err != nil {
		respondToolError(c, err)
		return
	}
	respondSuccess(c, t)
}

// Download 返回短期下载地址（ready 自定义工具 only）。
func (h *ToolHandler) Download(c *gin.Context) {
	dto, err := h.service.Download(tenant.GetTenantID(c), c.Param("name"))
	if err != nil {
		respondToolError(c, err)
		return
	}
	respondSuccess(c, dto)
}

func (h *ToolHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(tenant.GetTenantID(c), c.Param("name")); err != nil {
		respondToolError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "Tool 已删除")
}

type updateAgentToolsReq struct {
	ToolNames []string `json:"toolNames" binding:"required"`
}

func (h *ToolHandler) UpdateAgentTools(c *gin.Context) {
	var req updateAgentToolsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.UpdateAgentTools(tenant.GetTenantID(c), c.Param("name"), req.ToolNames); err != nil {
		respondToolError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "Agent Tool 关系已更新")
}

func (h *ToolHandler) GetAgentTools(c *gin.Context) {
	toolNames, err := h.service.GetAgentTools(tenant.GetTenantID(c), c.Param("name"))
	if err != nil {
		// agent 不存在 → 404；DB 故障经 default 桶如实 500（对齐 UpdateAgentTools）
		respondToolError(c, err)
		return
	}
	respondSuccess(c, toolNames)
}
