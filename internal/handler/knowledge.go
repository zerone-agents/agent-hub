package handler

import (
	"errors"
	"mime"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/provider"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type KnowledgeHandler struct {
	service        *services.KnowledgeService
	multiragMyLLMs provider.MultiRAGMyLLMsSource
}

// NewKnowledgeHandler constructs a KnowledgeHandler. multiragMyLLMs is the
// read-only source for MultiRAG's configured models; pass nil when MultiRAG
// is unconfigured (the /knowledge/multirag/models endpoint will return 503).
func NewKnowledgeHandler(service *services.KnowledgeService, multiragMyLLMs provider.MultiRAGMyLLMsSource) *KnowledgeHandler {
	return &KnowledgeHandler{service: service, multiragMyLLMs: multiragMyLLMs}
}

// RegisterKnowledgeRoutes 把知识库管理端点注册到两个同路径组：
// writeGroup 挂 adminWrite（admin|maintainer）——全部写方法（POST/PUT/DELETE）
// 及 POST /retrieval；readGroup 挂 adminRead（admin|maintainer|member）——
// 非敏感 GET（health、datasets/documents/chunks/images 的查询与下载、
// multirag 模型列表）。GET 与写方法路径不重复，gin 允许共存。
func RegisterKnowledgeRoutes(writeGroup, readGroup *gin.RouterGroup, h *KnowledgeHandler) {
	knowledgeGroup := writeGroup.Group("/knowledge")
	knowledgeReadGroup := readGroup.Group("/knowledge")
	{
		knowledgeReadGroup.GET("/health", h.Health)
		knowledgeReadGroup.GET("/datasets", h.ListDatasets)
		knowledgeGroup.POST("/datasets", h.CreateDataset)
		knowledgeGroup.DELETE("/datasets", h.DeleteDatasets)
		knowledgeReadGroup.GET("/datasets/:datasetId", h.GetDataset)
		knowledgeGroup.PUT("/datasets/:datasetId", h.UpdateDataset)
		knowledgeReadGroup.GET("/datasets/:datasetId/documents", h.ListDocuments)
		knowledgeGroup.POST("/datasets/:datasetId/documents", h.UploadDocuments)
		knowledgeGroup.DELETE("/datasets/:datasetId/documents", h.DeleteDocuments)
		knowledgeReadGroup.GET("/datasets/:datasetId/documents/:documentId/download", h.DownloadDocument)
		knowledgeGroup.PUT("/datasets/:datasetId/documents/:documentId", h.UpdateDocument)
		knowledgeGroup.POST("/datasets/:datasetId/documents/parse", h.ParseDocuments)
		knowledgeGroup.DELETE("/datasets/:datasetId/documents/parse", h.StopParsingDocuments)
		knowledgeReadGroup.GET("/datasets/:datasetId/images/:imageId", h.GetImage)
		knowledgeReadGroup.GET("/datasets/:datasetId/documents/:documentId/chunks", h.ListChunks)
		knowledgeGroup.POST("/datasets/:datasetId/documents/:documentId/chunks", h.CreateChunk)
		knowledgeGroup.DELETE("/datasets/:datasetId/documents/:documentId/chunks", h.DeleteChunks)
		knowledgeGroup.PUT("/datasets/:datasetId/documents/:documentId/chunks/:chunkId", h.UpdateChunk)
		knowledgeGroup.POST("/datasets/:datasetId/documents/:documentId/chunks/switch", h.SwitchChunks)
		// POST /retrieval 是带 body 的查询，但按 spec「写保持原样」留在 write 组
		knowledgeGroup.POST("/retrieval", h.Retrieval)
		knowledgeReadGroup.GET("/multirag/models", h.ListMultiRAGModels)
	}
}

func (h *KnowledgeHandler) Health(c *gin.Context) {
	status, err := h.service.Health(c.Request.Context())
	if err != nil {
		c.JSON(knowledge.StatusCode(err), gin.H{
			"success": false,
			"error":   err.Error(),
			"data":    status,
		})
		return
	}
	respondSuccess(c, status)
}

func (h *KnowledgeHandler) ListDatasets(c *gin.Context) {
	result, err := h.service.ListDatasets(c.Request.Context(), knowledge.DatasetListRequest{
		ID:                   c.Query("id"),
		Name:                 c.Query("name"),
		Page:                 queryInt(c, "page"),
		PageSize:             queryInt(c, "page_size"),
		OrderBy:              c.Query("orderby"),
		Desc:                 queryBoolPtr(c, "desc"),
		Keywords:             c.Query("keywords"),
		ParserID:             c.Query("parser_id"),
		OwnerIDs:             c.QueryArray("owner_ids"),
		IncludeParsingStatus: queryBool(c, "include_parsing_status"),
	})
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *KnowledgeHandler) CreateDataset(c *gin.Context) {
	var req knowledge.DatasetMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	dataset, err := h.service.CreateDataset(c.Request.Context(), tenant.GetTenantID(c), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondCreated(c, dataset)
}

func (h *KnowledgeHandler) GetDataset(c *gin.Context) {
	dataset, err := h.service.GetDataset(c.Request.Context(), c.Param("datasetId"))
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, dataset)
}

func (h *KnowledgeHandler) UpdateDataset(c *gin.Context) {
	var req knowledge.DatasetMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	dataset, err := h.service.UpdateDataset(c.Request.Context(), tenant.GetTenantID(c), c.Param("datasetId"), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, dataset)
}

func (h *KnowledgeHandler) DeleteDatasets(c *gin.Context) {
	var req knowledge.DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteDatasets(c.Request.Context(), req); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "知识库已删除")
}

func (h *KnowledgeHandler) ListDocuments(c *gin.Context) {
	result, err := h.service.ListDocuments(c.Request.Context(), c.Param("datasetId"), knowledge.DocumentListRequest{
		Page:              queryInt(c, "page"),
		PageSize:          queryInt(c, "page_size"),
		OrderBy:           c.Query("orderby"),
		Desc:              queryBoolPtr(c, "desc"),
		Keywords:          c.Query("keywords"),
		ID:                c.Query("id"),
		Name:              c.Query("name"),
		Suffix:            c.QueryArray("suffix"),
		Run:               c.QueryArray("run"),
		CreateTimeFrom:    queryInt64(c, "create_time_from"),
		CreateTimeTo:      queryInt64(c, "create_time_to"),
		MetadataCondition: c.Query("metadata_condition"),
	})
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *KnowledgeHandler) UploadDocuments(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		respondError(c, http.StatusBadRequest, "上传请求必须是 multipart/form-data")
		return
	}
	documents, err := h.service.UploadDocuments(c.Request.Context(), c.Param("datasetId"), knowledge.UploadRequest{
		Body:          c.Request.Body,
		ContentType:   contentType,
		ContentLength: c.Request.ContentLength,
	})
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondCreated(c, documents)
}

func (h *KnowledgeHandler) DownloadDocument(c *gin.Context) {
	stream, err := h.service.DownloadDocument(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"))
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondStream(c, stream, "application/octet-stream")
}

func (h *KnowledgeHandler) GetImage(c *gin.Context) {
	stream, err := h.service.GetImage(c.Request.Context(), c.Param("datasetId"), c.Param("imageId"))
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondStream(c, stream, "image/jpeg")
}

func (h *KnowledgeHandler) UpdateDocument(c *gin.Context) {
	var req knowledge.DocumentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	document, err := h.service.UpdateDocument(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, document)
}

func (h *KnowledgeHandler) DeleteDocuments(c *gin.Context) {
	var req knowledge.DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteDocuments(c.Request.Context(), c.Param("datasetId"), req); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "文档已删除")
}

type documentIDsRequest struct {
	DocumentIDs []string `json:"document_ids"`
	IDs         []string `json:"ids"`
}

func (r documentIDsRequest) ids() []string {
	if len(r.DocumentIDs) > 0 {
		return r.DocumentIDs
	}
	return r.IDs
}

func (h *KnowledgeHandler) ParseDocuments(c *gin.Context) {
	var req documentIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.ParseDocuments(c.Request.Context(), c.Param("datasetId"), req.ids()); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "文档已加入解析队列")
}

func (h *KnowledgeHandler) StopParsingDocuments(c *gin.Context) {
	var req documentIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.StopParsingDocuments(c.Request.Context(), c.Param("datasetId"), req.ids()); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "文档解析已停止")
}

func (h *KnowledgeHandler) ListChunks(c *gin.Context) {
	result, err := h.service.ListChunks(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"), knowledge.ChunkListRequest{
		Page:      queryInt(c, "page"),
		PageSize:  queryInt(c, "page_size"),
		Keywords:  c.Query("keywords"),
		ID:        c.Query("id"),
		Available: queryBoolPtr(c, "available"),
	})
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *KnowledgeHandler) CreateChunk(c *gin.Context) {
	var req knowledge.ChunkMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	chunk, err := h.service.CreateChunk(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondCreated(c, chunk)
}

func (h *KnowledgeHandler) UpdateChunk(c *gin.Context) {
	var req knowledge.ChunkMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	chunk, err := h.service.UpdateChunk(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"), c.Param("chunkId"), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, chunk)
}

func (h *KnowledgeHandler) DeleteChunks(c *gin.Context) {
	var req knowledge.DeleteChunksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteChunks(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"), req); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "分块已删除")
}

type switchChunksRequest struct {
	ChunkIDs     []string `json:"chunk_ids"`
	Available    *bool    `json:"available"`
	AvailableInt *int     `json:"available_int"`
}

func (r switchChunksRequest) availableValue() (bool, bool) {
	if r.Available != nil {
		return *r.Available, true
	}
	if r.AvailableInt != nil {
		return *r.AvailableInt != 0, true
	}
	return false, false
}

func (h *KnowledgeHandler) SwitchChunks(c *gin.Context) {
	var req switchChunksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	available, ok := req.availableValue()
	if !ok {
		respondError(c, http.StatusBadRequest, "available 或 available_int 必填")
		return
	}
	if err := h.service.SwitchChunks(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"), req.ChunkIDs, available); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, gin.H{"available": available})
}

func (h *KnowledgeHandler) Retrieval(c *gin.Context) {
	var req knowledge.RetrievalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.Retrieval(c.Request.Context(), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, result)
}

// respondKnowledgeError 映射知识域错误：删除保护（issue #122）→ 409 +
// data.datasets 名单；其余沿用 knowledge.StatusCode 桶。
func respondKnowledgeError(c *gin.Context, err error) {
	var inUse *agent.DatasetInUseError
	if errors.As(err, &inUse) {
		datasets := make([]gin.H, 0, len(inUse.Datasets))
		for _, d := range inUse.Datasets {
			datasets = append(datasets, gin.H{"id": d.ID, "agents": d.Agents})
		}
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": inUse.Error(), "data": gin.H{"datasets": datasets}})
		return
	}
	respondError(c, knowledge.StatusCode(err), err.Error())
}

func respondStream(c *gin.Context, stream *knowledge.StreamResult, fallbackContentType string) {
	if stream == nil || stream.Body == nil {
		respondKnowledgeError(c, knowledge.NewUpstreamError("multirag stream 响应为空", nil))
		return
	}
	defer stream.Body.Close()
	headers := map[string]string{}
	if stream.ContentDisposition != "" {
		headers["Content-Disposition"] = stream.ContentDisposition
	}
	contentType := stream.ContentType
	if contentType == "" {
		contentType = fallbackContentType
	}
	c.DataFromReader(http.StatusOK, stream.ContentLength, contentType, stream.Body, headers)
}

func queryInt(c *gin.Context, key string) int {
	value := c.Query(key)
	if value == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func queryInt64(c *gin.Context, key string) int64 {
	value := c.Query(key)
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func queryBool(c *gin.Context, key string) bool {
	value := c.Query(key)
	if value == "" {
		return false
	}
	parsed, _ := strconv.ParseBool(value)
	return parsed
}

func queryBoolPtr(c *gin.Context, key string) *bool {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil
	}
	return &parsed
}
