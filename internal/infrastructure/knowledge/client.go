package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "control-panel/internal/domain/knowledge"
)

type RemoteMultiragEngine struct {
	baseURL      string
	apiKey       string
	client       *http.Client
	uploadClient *http.Client
}

func NewRemoteMultiragEngine(baseURL, apiKey string, timeout, uploadTimeout time.Duration) *RemoteMultiragEngine {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if uploadTimeout <= 0 {
		uploadTimeout = time.Hour
	}
	return &RemoteMultiragEngine{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		client:       &http.Client{Timeout: timeout},
		uploadClient: &http.Client{Timeout: uploadTimeout},
	}
}

type envelope struct {
	RetCode       *int            `json:"retcode"`
	Code          *int            `json:"code"`
	RetMsg        string          `json:"retmsg"`
	Message       string          `json:"message"`
	Error         string          `json:"error"`
	Data          json.RawMessage `json:"data"`
	Total         *int            `json:"total"`
	TotalDatasets *int            `json:"total_datasets"`
}

func (e *envelope) statusCode() (int, bool) {
	if e.RetCode != nil {
		return *e.RetCode, true
	}
	if e.Code != nil {
		return *e.Code, true
	}
	return 0, false
}

func (e *envelope) message() string {
	if e.RetMsg != "" {
		return e.RetMsg
	}
	if e.Message != "" {
		return e.Message
	}
	return "multirag request failed"
}

func (e *envelope) total() int {
	if e.Total != nil {
		return *e.Total
	}
	if e.TotalDatasets != nil {
		return *e.TotalDatasets
	}
	return 0
}

func (c *RemoteMultiragEngine) Health(ctx context.Context) (*domain.HealthStatus, error) {
	_, err := c.ListDatasets(ctx, domain.DatasetListRequest{Page: 1, PageSize: 1})
	if err != nil {
		return &domain.HealthStatus{
			Configured: true,
			Connected:  false,
			Status:     "unhealthy",
			Message:    err.Error(),
		}, err
	}
	return &domain.HealthStatus{
		Configured: true,
		Connected:  true,
		Status:     "healthy",
	}, nil
}

func (c *RemoteMultiragEngine) ListDatasets(ctx context.Context, req domain.DatasetListRequest) (*domain.DatasetListResult, error) {
	localNameFilter := req.Name != "" || req.Keywords != ""
	query := url.Values{}
	addString(query, "id", req.ID)
	if localNameFilter {
		addInt(query, "page", 1)
		addInt(query, "page_size", datasetFilterFetchSize(req.Page, req.PageSize))
	} else {
		addString(query, "name", req.Name)
		addInt(query, "page", req.Page)
		addInt(query, "page_size", req.PageSize)
	}
	addString(query, "orderby", req.OrderBy)
	addBoolPtr(query, "desc", req.Desc)
	if !localNameFilter {
		addString(query, "keywords", req.Keywords)
	}
	addString(query, "parser_id", req.ParserID)
	addBool(query, "include_parsing_status", req.IncludeParsingStatus)
	for _, ownerID := range req.OwnerIDs {
		if ownerID != "" {
			query.Add("owner_ids", ownerID)
		}
	}

	var raw []map[string]any
	total, err := c.doJSON(ctx, http.MethodGet, "/api/v1/datasets", query, nil, &raw)
	if err != nil {
		return nil, err
	}
	datasets := make([]domain.Dataset, 0, len(raw))
	for _, item := range raw {
		datasets = append(datasets, domain.NormalizeDataset(item))
	}
	if localNameFilter {
		datasets = filterDatasetsByDisplayName(datasets, req.Name, req.Keywords)
		return &domain.DatasetListResult{
			Total:    len(datasets),
			Datasets: paginateDatasets(datasets, req.Page, req.PageSize),
		}, nil
	}
	return &domain.DatasetListResult{Total: total, Datasets: datasets}, nil
}

func (c *RemoteMultiragEngine) CreateDataset(ctx context.Context, req domain.DatasetMutationRequest) (*domain.Dataset, error) {
	body := domain.DatasetCreateToRemote(req)
	var raw map[string]any
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/datasets", nil, body, &raw); err != nil {
		return nil, err
	}
	displayName := domain.DatasetMutationDisplayName(req)
	if id := toString(raw["id"]); id != "" {
		updateBody := domain.DatasetUpdateToRemote(req)
		if len(updateBody) > 0 {
			var updated map[string]any
			if _, err := c.doJSON(ctx, http.MethodPut, "/api/v1/datasets/"+url.PathEscape(id), nil, updateBody, &updated); err == nil {
				raw = updated
			} else if displayName != "" {
				raw["display_name"] = displayName
			}
		}
	} else if displayName != "" {
		raw["display_name"] = displayName
	}
	dataset := domain.NormalizeDataset(raw)
	return &dataset, nil
}

func (c *RemoteMultiragEngine) GetDataset(ctx context.Context, id string) (*domain.Dataset, error) {
	result, err := c.ListDatasets(ctx, domain.DatasetListRequest{ID: id, Page: 1, PageSize: 1})
	if err != nil {
		return nil, err
	}
	if len(result.Datasets) == 0 {
		return nil, domain.NewNotFoundError("知识库不存在")
	}
	return &result.Datasets[0], nil
}

func (c *RemoteMultiragEngine) UpdateDataset(ctx context.Context, id string, req domain.DatasetMutationRequest) (*domain.Dataset, error) {
	body := domain.DatasetUpdateToRemote(req)
	var raw map[string]any
	if _, err := c.doJSON(ctx, http.MethodPut, "/api/v1/datasets/"+url.PathEscape(id), nil, body, &raw); err != nil {
		return nil, err
	}
	dataset := domain.NormalizeDataset(raw)
	return &dataset, nil
}

func (c *RemoteMultiragEngine) DeleteDatasets(ctx context.Context, req domain.DeleteRequest) error {
	_, err := c.doJSON(ctx, http.MethodDelete, "/api/v1/datasets", nil, req, nil)
	return err
}

func (c *RemoteMultiragEngine) ListDocuments(ctx context.Context, datasetID string, req domain.DocumentListRequest) (*domain.DocumentListResult, error) {
	query := url.Values{}
	addInt(query, "page", req.Page)
	addInt(query, "page_size", req.PageSize)
	addString(query, "orderby", req.OrderBy)
	addBoolPtr(query, "desc", req.Desc)
	addString(query, "keywords", req.Keywords)
	addString(query, "id", req.ID)
	addString(query, "name", req.Name)
	addInt64(query, "create_time_from", req.CreateTimeFrom)
	addInt64(query, "create_time_to", req.CreateTimeTo)
	addString(query, "metadata_condition", req.MetadataCondition)
	for _, suffix := range req.Suffix {
		if suffix != "" {
			query.Add("suffix", suffix)
		}
	}
	for _, run := range req.Run {
		if run != "" {
			query.Add("run", run)
		}
	}

	var raw struct {
		Total int              `json:"total"`
		Docs  []map[string]any `json:"docs"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/documents", query, nil, &raw); err != nil {
		return nil, err
	}
	documents := make([]domain.Document, 0, len(raw.Docs))
	for _, item := range raw.Docs {
		documents = append(documents, domain.NormalizeDocument(item))
	}
	return &domain.DocumentListResult{Total: raw.Total, Documents: documents}, nil
}

func (c *RemoteMultiragEngine) UploadDocuments(ctx context.Context, datasetID string, upload domain.UploadRequest) ([]domain.Document, error) {
	var raw []map[string]any
	if _, err := c.doMultipart(ctx, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/documents", upload, &raw); err != nil {
		return nil, err
	}
	documents := make([]domain.Document, 0, len(raw))
	for _, item := range raw {
		documents = append(documents, domain.NormalizeDocument(item))
	}
	return documents, nil
}

func (c *RemoteMultiragEngine) UpdateDocument(ctx context.Context, datasetID string, documentID string, req domain.DocumentUpdateRequest) (*domain.Document, error) {
	body := domain.DocumentUpdateToRemote(req)
	var raw map[string]any
	if _, err := c.doJSON(ctx, http.MethodPut, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/documents/"+url.PathEscape(documentID), nil, body, &raw); err != nil {
		return nil, err
	}
	document := domain.NormalizeDocument(raw)
	return &document, nil
}

func (c *RemoteMultiragEngine) DeleteDocuments(ctx context.Context, datasetID string, req domain.DeleteRequest) error {
	_, err := c.doJSON(ctx, http.MethodDelete, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/documents", nil, req, nil)
	return err
}

func (c *RemoteMultiragEngine) ParseDocuments(ctx context.Context, datasetID string, ids []string) error {
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/chunks", nil, map[string]any{"document_ids": ids}, nil)
	return err
}

func (c *RemoteMultiragEngine) StopParsingDocuments(ctx context.Context, datasetID string, ids []string) error {
	_, err := c.doJSON(ctx, http.MethodDelete, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/chunks", nil, map[string]any{"document_ids": ids}, nil)
	return err
}

func (c *RemoteMultiragEngine) ListChunks(ctx context.Context, datasetID string, documentID string, req domain.ChunkListRequest) (*domain.ChunkListResult, error) {
	query := url.Values{}
	addInt(query, "page", req.Page)
	addInt(query, "page_size", req.PageSize)
	addString(query, "keywords", req.Keywords)
	addString(query, "id", req.ID)
	addBoolPtr(query, "available", req.Available)

	var raw struct {
		Total  int              `json:"total"`
		Chunks []map[string]any `json:"chunks"`
		Doc    map[string]any   `json:"doc"`
	}
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID) + "/chunks"
	if _, err := c.doJSON(ctx, http.MethodGet, path, query, nil, &raw); err != nil {
		return nil, err
	}
	chunks := make([]domain.Chunk, 0, len(raw.Chunks))
	for _, item := range raw.Chunks {
		chunks = append(chunks, domain.NormalizeChunk(item))
	}
	var document domain.Document
	if raw.Doc != nil {
		document = domain.NormalizeDocument(raw.Doc)
	}
	return &domain.ChunkListResult{Total: raw.Total, Chunks: chunks, Document: document}, nil
}

func (c *RemoteMultiragEngine) CreateChunk(ctx context.Context, datasetID string, documentID string, req domain.ChunkMutationRequest) (*domain.Chunk, error) {
	body := domain.ChunkMutationToRemote(req)
	var raw struct {
		Chunk map[string]any `json:"chunk"`
	}
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID) + "/chunks"
	if _, err := c.doJSON(ctx, http.MethodPost, path, nil, body, &raw); err != nil {
		return nil, err
	}
	chunk := domain.NormalizeChunk(raw.Chunk)
	return &chunk, nil
}

func (c *RemoteMultiragEngine) UpdateChunk(ctx context.Context, datasetID string, documentID string, chunkID string, req domain.ChunkMutationRequest) (*domain.Chunk, error) {
	body := domain.ChunkMutationToRemote(req)
	var raw map[string]any
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID) + "/chunks/" + url.PathEscape(chunkID)
	if _, err := c.doJSON(ctx, http.MethodPut, path, nil, body, &raw); err != nil {
		return nil, err
	}
	chunk := domain.NormalizeChunk(raw)
	return &chunk, nil
}

func (c *RemoteMultiragEngine) DeleteChunks(ctx context.Context, datasetID string, documentID string, req domain.DeleteChunksRequest) error {
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID) + "/chunks"
	_, err := c.doJSON(ctx, http.MethodDelete, path, nil, req, nil)
	return err
}

func (c *RemoteMultiragEngine) SwitchChunks(ctx context.Context, datasetID string, documentID string, ids []string, available bool) error {
	body := map[string]any{"chunk_ids": ids, "available": available}
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID) + "/chunks/switch"
	_, err := c.doJSON(ctx, http.MethodPost, path, nil, body, nil)
	return err
}

func (c *RemoteMultiragEngine) Retrieval(ctx context.Context, req domain.RetrievalRequest) (*domain.RetrievalResult, error) {
	body := domain.CloneObject(map[string]any(req))
	var raw map[string]any
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/retrieval", nil, body, &raw); err != nil {
		return nil, err
	}
	normalizeRetrievalChunks(raw)
	result := domain.RetrievalResult(raw)
	return &result, nil
}

func (c *RemoteMultiragEngine) DownloadDocument(ctx context.Context, datasetID string, documentID string) (*domain.StreamResult, error) {
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID)
	return c.doStream(ctx, path, "application/octet-stream, */*")
}

func (c *RemoteMultiragEngine) GetImage(ctx context.Context, imageID string) (*domain.StreamResult, error) {
	path := "/v1/document/image/" + url.PathEscape(imageID)
	result, err := c.doStream(ctx, path, "image/*, */*")
	if err != nil {
		return nil, err
	}
	if result.ContentType == "" {
		result.ContentType = "image/jpeg"
		return result, nil
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(result.ContentType, ";")[0]))
	if !strings.HasPrefix(mediaType, "image/") {
		body, _ := io.ReadAll(io.LimitReader(result.Body, 4096))
		result.Body.Close()
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = result.ContentType
		}
		return nil, domain.NewUpstreamError("multirag image response is not an image", errors.New(message))
	}
	return result, nil
}

func (c *RemoteMultiragEngine) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, domain.NewBadRequestError(fmt.Sprintf("序列化请求失败: %v", err))
		}
		reader = bytes.NewReader(payload)
	}
	req, err := c.newRequest(ctx, method, path, query, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, domain.NewUpstreamError("调用 multirag 失败", err)
	}
	defer resp.Body.Close()
	return decodeEnvelopeResponse(resp, out)
}

func (c *RemoteMultiragEngine) doMultipart(ctx context.Context, path string, upload domain.UploadRequest, out any) (int, error) {
	req, err := c.newRequest(ctx, http.MethodPost, path, nil, upload.Body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", upload.ContentType)
	if upload.ContentLength >= 0 {
		req.ContentLength = upload.ContentLength
	}
	resp, err := c.uploadClient.Do(req)
	if err != nil {
		return 0, domain.NewUpstreamError("调用 multirag 上传文档失败", err)
	}
	defer resp.Body.Close()
	return decodeEnvelopeResponse(resp, out)
}

func (c *RemoteMultiragEngine) doStream(ctx context.Context, path string, accept string) (*domain.StreamResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, domain.NewUpstreamError("调用 multirag stream 失败", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, domain.NewUpstreamError(fmt.Sprintf("multirag returned HTTP %d", resp.StatusCode), errors.New(strings.TrimSpace(string(body))))
	}
	contentType := resp.Header.Get("Content-Type")
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if mediaType == "application/json" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		message := streamJSONErrorMessage(body)
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		return nil, domain.NewUpstreamError("multirag stream returned JSON instead of file content", errors.New(message))
	}
	return &domain.StreamResult{
		Body:               resp.Body,
		ContentType:        contentType,
		ContentDisposition: resp.Header.Get("Content-Disposition"),
		ContentLength:      resp.ContentLength,
	}, nil
}

func streamJSONErrorMessage(body []byte) string {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	switch {
	case env.RetMsg != "":
		return env.RetMsg
	case env.Message != "":
		return env.Message
	case env.Error != "":
		return env.Error
	default:
		return ""
	}
}

func (c *RemoteMultiragEngine) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	if c.baseURL == "" {
		return nil, domain.NewUnavailableError("MULTIRAG_BASE_URL 未配置")
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, domain.NewBadRequestError(fmt.Sprintf("构造 multirag 请求失败: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func decodeEnvelopeResponse(resp *http.Response, out any) (int, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, domain.NewUpstreamError(fmt.Sprintf("multirag returned HTTP %d", resp.StatusCode), errors.New(strings.TrimSpace(string(body))))
	}

	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return 0, domain.NewUpstreamError("解析 multirag 响应失败", err)
	}
	code, ok := env.statusCode()
	if !ok {
		return 0, domain.NewUpstreamError("multirag 响应缺少 code/retcode", nil)
	}
	if code != 0 {
		return 0, domain.NewUpstreamError(fmt.Sprintf("multirag error %d: %s", code, env.message()), nil)
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return 0, domain.NewUpstreamError("解析 multirag data 失败", err)
		}
	}
	return env.total(), nil
}

func normalizeRetrievalChunks(raw map[string]any) {
	chunks, ok := raw["chunks"].([]any)
	if !ok {
		return
	}
	normalized := make([]any, 0, len(chunks))
	for _, item := range chunks {
		chunkMap, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		normalized = append(normalized, map[string]any(domain.NormalizeChunk(chunkMap)))
	}
	raw["chunks"] = normalized
}

func addString(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}

func addInt(values url.Values, key string, value int) {
	if value > 0 {
		values.Set(key, strconv.Itoa(value))
	}
}

func addInt64(values url.Values, key string, value int64) {
	if value > 0 {
		values.Set(key, strconv.FormatInt(value, 10))
	}
}

func addBool(values url.Values, key string, value bool) {
	if value {
		values.Set(key, "true")
	}
}

func addBoolPtr(values url.Values, key string, value *bool) {
	if value != nil {
		values.Set(key, strconv.FormatBool(*value))
	}
}

func datasetFilterFetchSize(page int, pageSize int) int {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}
	size := page * pageSize
	if size < 1000 {
		return 1000
	}
	return size
}

func filterDatasetsByDisplayName(datasets []domain.Dataset, name string, keywords string) []domain.Dataset {
	name = strings.TrimSpace(strings.ToLower(name))
	keywords = strings.TrimSpace(strings.ToLower(keywords))
	filtered := make([]domain.Dataset, 0, len(datasets))
	for _, dataset := range datasets {
		item := map[string]any(dataset)
		displayName := strings.TrimSpace(toString(item["display_name"]))
		stableName := strings.TrimSpace(toString(item["name"]))
		collectionName := strings.TrimSpace(toString(item["collection_name"]))
		description := strings.TrimSpace(toString(item["description"]))
		if name != "" && !strings.EqualFold(displayName, name) && !strings.EqualFold(stableName, name) && !strings.EqualFold(collectionName, name) {
			continue
		}
		if keywords != "" {
			haystack := strings.ToLower(strings.Join([]string{displayName, stableName, collectionName, description}, "\n"))
			if !strings.Contains(haystack, keywords) {
				continue
			}
		}
		filtered = append(filtered, dataset)
	}
	return filtered
}

func paginateDatasets(datasets []domain.Dataset, page int, pageSize int) []domain.Dataset {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		return datasets
	}
	start := (page - 1) * pageSize
	if start >= len(datasets) {
		return []domain.Dataset{}
	}
	end := start + pageSize
	if end > len(datasets) {
		end = len(datasets)
	}
	return datasets[start:end]
}

func toString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
