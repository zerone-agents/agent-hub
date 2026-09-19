package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"control-panel/internal/extensionmanifest"

	"github.com/gin-gonic/gin"
)

const maxInlineManifestBytes = 256 << 10

type ExtensionVerificationHandler struct{}

func NewExtensionVerificationHandler() *ExtensionVerificationHandler {
	return &ExtensionVerificationHandler{}
}

type contributionDescription struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Overview returns stable, product-neutral protocol metadata. Consumer
// fixtures deliberately live outside the server binary.
func (h *ExtensionVerificationHandler) Overview(c *gin.Context) {
	respondSuccess(c, gin.H{
		"platformVersion": "0.9.0-h0",
		"protocolVersion": "v1alpha1",
		"status":          "可验收",
		"contributionCategories": []contributionDescription{
			{Key: "stateSchemas", Label: "状态 Schema", Description: "声明由能力包拥有的运行状态结构。"},
			{Key: "events", Label: "事件", Description: "声明能力包消费和产生的领域事件。"},
			{Key: "tools", Label: "工具", Description: "声明可授权给 Agent 的操作能力。"},
			{Key: "relationTypes", Label: "关系类型", Description: "声明 Agent 之间可配置的协作关系。"},
			{Key: "promptFragments", Label: "提示词注入", Description: "声明按运行上下文合成的提示词片段。"},
			{Key: "uiViews", Label: "UI 展示", Description: "声明能力包提供的管理或运行视图。"},
			{Key: "templates", Label: "模板", Description: "声明可复用的初始配置模板。"},
			{Key: "handlers", Label: "处理器", Description: "声明响应事件的运行时处理器。"},
		},
	})
}

type validateExtensionRequest struct {
	Manifest string `json:"manifest" binding:"required"`
}

// Validate checks pasted YAML in memory. File references are checked for
// schema shape only; this endpoint never reads paths named by a manifest.
func (h *ExtensionVerificationHandler) Validate(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxInlineManifestBytes)
	var request validateExtensionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		if strings.Contains(err.Error(), "request body too large") || errors.Is(err, io.ErrUnexpectedEOF) && c.Request.ContentLength > maxInlineManifestBytes {
			respondError(c, http.StatusRequestEntityTooLarge, "manifest must not exceed 256 KiB")
			return
		}
		respondError(c, http.StatusBadRequest, "manifest is required")
		return
	}

	report := extensionmanifest.ValidateManifest([]byte(request.Manifest))
	respondSuccess(c, gin.H{
		"valid":         report.Valid,
		"package":       report.Package,
		"contributions": report.Contributions,
		"errors":        extensionValidationErrors(report.Errors),
	})
}

type extensionValidationError struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func extensionValidationErrors(messages []string) []extensionValidationError {
	output := make([]extensionValidationError, 0, len(messages))
	for _, message := range messages {
		item := extensionValidationError{Message: message}
		if strings.HasPrefix(message, "/") {
			if separator := strings.Index(message, ":"); separator > 0 {
				item.Path = message[:separator]
				item.Message = strings.TrimSpace(message[separator+1:])
			}
		}
		output = append(output, item)
	}
	return output
}
