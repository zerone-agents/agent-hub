package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/knowledge"

	"github.com/gin-gonic/gin"
)

type handlerFakeKnowledgeEngine struct {
	listDatasetsFunc func(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error)
	listDocsFunc     func(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error)
	uploadFunc       func(ctx context.Context, datasetID string, upload knowledge.UploadRequest) ([]knowledge.Document, error)
	downloadFunc     func(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error)
	imageFunc        func(ctx context.Context, imageID string) (*knowledge.StreamResult, error)
}

func (f *handlerFakeKnowledgeEngine) Health(ctx context.Context) (*knowledge.HealthStatus, error) {
	return &knowledge.HealthStatus{Configured: true, Connected: true, Status: "healthy"}, nil
}
func (f *handlerFakeKnowledgeEngine) ListDatasets(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error) {
	if f.listDatasetsFunc != nil {
		return f.listDatasetsFunc(ctx, req)
	}
	return &knowledge.DatasetListResult{}, nil
}
func (f *handlerFakeKnowledgeEngine) CreateDataset(ctx context.Context, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
	dataset := knowledge.Dataset{"id": "kb1", "name": req["name"]}
	return &dataset, nil
}
func (f *handlerFakeKnowledgeEngine) GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error) {
	dataset := knowledge.Dataset{"id": id}
	return &dataset, nil
}
func (f *handlerFakeKnowledgeEngine) UpdateDataset(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
	dataset := knowledge.Dataset{"id": id}
	return &dataset, nil
}
func (f *handlerFakeKnowledgeEngine) DeleteDatasets(ctx context.Context, req knowledge.DeleteRequest) error {
	return nil
}
func (f *handlerFakeKnowledgeEngine) ListDocuments(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
	if f.listDocsFunc != nil {
		return f.listDocsFunc(ctx, datasetID, req)
	}
	return &knowledge.DocumentListResult{}, nil
}
func (f *handlerFakeKnowledgeEngine) UploadDocuments(ctx context.Context, datasetID string, upload knowledge.UploadRequest) ([]knowledge.Document, error) {
	if f.uploadFunc != nil {
		return f.uploadFunc(ctx, datasetID, upload)
	}
	return []knowledge.Document{{"id": "doc1"}}, nil
}
func (f *handlerFakeKnowledgeEngine) DownloadDocument(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error) {
	if f.downloadFunc != nil {
		return f.downloadFunc(ctx, datasetID, documentID)
	}
	return &knowledge.StreamResult{Body: io.NopCloser(strings.NewReader("doc"))}, nil
}
func (f *handlerFakeKnowledgeEngine) GetImage(ctx context.Context, imageID string) (*knowledge.StreamResult, error) {
	if f.imageFunc != nil {
		return f.imageFunc(ctx, imageID)
	}
	return &knowledge.StreamResult{Body: io.NopCloser(strings.NewReader("image"))}, nil
}
func (f *handlerFakeKnowledgeEngine) UpdateDocument(ctx context.Context, datasetID string, documentID string, req knowledge.DocumentUpdateRequest) (*knowledge.Document, error) {
	document := knowledge.Document{"id": documentID}
	return &document, nil
}
func (f *handlerFakeKnowledgeEngine) DeleteDocuments(ctx context.Context, datasetID string, req knowledge.DeleteRequest) error {
	return nil
}
func (f *handlerFakeKnowledgeEngine) ParseDocuments(ctx context.Context, datasetID string, ids []string) error {
	return nil
}
func (f *handlerFakeKnowledgeEngine) StopParsingDocuments(ctx context.Context, datasetID string, ids []string) error {
	return nil
}
func (f *handlerFakeKnowledgeEngine) ListChunks(ctx context.Context, datasetID string, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
	return &knowledge.ChunkListResult{}, nil
}
func (f *handlerFakeKnowledgeEngine) CreateChunk(ctx context.Context, datasetID string, documentID string, req knowledge.ChunkMutationRequest) (*knowledge.Chunk, error) {
	chunk := knowledge.Chunk{"id": "chunk1"}
	return &chunk, nil
}
func (f *handlerFakeKnowledgeEngine) UpdateChunk(ctx context.Context, datasetID string, documentID string, chunkID string, req knowledge.ChunkMutationRequest) (*knowledge.Chunk, error) {
	chunk := knowledge.Chunk{"id": chunkID}
	return &chunk, nil
}
func (f *handlerFakeKnowledgeEngine) DeleteChunks(ctx context.Context, datasetID string, documentID string, req knowledge.DeleteChunksRequest) error {
	return nil
}
func (f *handlerFakeKnowledgeEngine) SwitchChunks(ctx context.Context, datasetID string, documentID string, ids []string, available bool) error {
	return nil
}
func (f *handlerFakeKnowledgeEngine) Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
	result := knowledge.RetrievalResult{"total": 0}
	return &result, nil
}

func setupKnowledgeRouter(engine knowledge.KnowledgeEngine) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	RegisterKnowledgeRoutes(admin, NewKnowledgeHandler(services.NewKnowledgeService(engine, nil), nil))
	return router
}

func TestKnowledgeHandler_ListDatasetsRoute(t *testing.T) {
	var gotReq knowledge.DatasetListRequest
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		listDatasetsFunc: func(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error) {
			gotReq = req
			return &knowledge.DatasetListResult{
				Total:    1,
				Datasets: []knowledge.Dataset{{"id": "kb1", "doc_num": 2}},
			}, nil
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets?page=3&page_size=9&keywords=abc", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if gotReq.Page != 3 || gotReq.PageSize != 9 || gotReq.Keywords != "abc" {
		t.Fatalf("query binding failed: %#v", gotReq)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success = %#v, want true", body["success"])
	}
}

func TestKnowledgeHandler_MissingConfigHealth(t *testing.T) {
	router := setupKnowledgeRouter(nil)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/health", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte("MULTIRAG_BASE_URL")) {
		t.Fatalf("response does not mention missing config: %s", resp.Body.String())
	}
}

func TestKnowledgeHandler_HealthyHealth(t *testing.T) {
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{})
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/health", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success = %#v, want true", body["success"])
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %#v", body["data"])
	}
	if data["configured"] != true {
		t.Fatalf("configured = %#v, want true", data["configured"])
	}
	if data["connected"] != true {
		t.Fatalf("connected = %#v, want true", data["connected"])
	}
	if data["status"] != "healthy" {
		t.Fatalf("status = %#v, want healthy", data["status"])
	}
}

func TestKnowledgeHandler_CreateDatasetBindError(t *testing.T) {
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{})
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/datasets", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body.String())
	}
}

func TestKnowledgeHandler_UploadDocuments(t *testing.T) {
	var gotDatasetID string
	var gotUpload knowledge.UploadRequest
	var gotBody []byte
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		uploadFunc: func(ctx context.Context, datasetID string, upload knowledge.UploadRequest) ([]knowledge.Document, error) {
			gotDatasetID = datasetID
			gotUpload = upload
			gotBody, _ = io.ReadAll(upload.Body)
			return []knowledge.Document{{"id": "doc1", "name": "a.txt"}}, nil
		},
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "a.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = part.Write([]byte("hello"))
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}
	payload := append([]byte(nil), body.Bytes()...)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/datasets/kb1/documents", bytes.NewReader(payload))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", resp.Code, resp.Body.String())
	}
	if gotDatasetID != "kb1" {
		t.Fatalf("dataset = %q, want kb1", gotDatasetID)
	}
	if gotUpload.ContentType != writer.FormDataContentType() {
		t.Fatalf("ContentType = %q, want %q", gotUpload.ContentType, writer.FormDataContentType())
	}
	if gotUpload.ContentLength != int64(len(payload)) {
		t.Fatalf("ContentLength = %d, want %d", gotUpload.ContentLength, len(payload))
	}
	if !bytes.Equal(gotBody, payload) {
		t.Fatal("multipart body was not forwarded unchanged")
	}
}

func TestKnowledgeHandler_UploadDocumentsRejectsInvalidContentType(t *testing.T) {
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{})
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/datasets/kb1/documents", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "application/octet-stream")

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body.String())
	}
}

func TestKnowledgeHandler_UploadDocumentsStartsBeforeSourceEOF(t *testing.T) {
	engineCalled := make(chan struct{})
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		uploadFunc: func(ctx context.Context, datasetID string, upload knowledge.UploadRequest) ([]knowledge.Document, error) {
			close(engineCalled)
			_, err := io.Copy(io.Discard, upload.Body)
			return []knowledge.Document{{"id": "doc1"}}, err
		},
	})

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()
	releaseEOF := make(chan struct{})
	producerDone := make(chan error, 1)
	go func() {
		part, err := writer.CreateFormFile("files", "large.mp4")
		if err == nil {
			_, err = part.Write([]byte("first chunk"))
		}
		if err == nil {
			<-releaseEOF
			err = writer.Close()
		}
		if closeErr := pipeWriter.CloseWithError(err); err == nil {
			err = closeErr
		}
		producerDone <- err
	}()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/datasets/kb1/documents", pipeReader)
	req.Header.Set("Content-Type", contentType)
	requestDone := make(chan struct{})
	go func() {
		router.ServeHTTP(resp, req)
		close(requestDone)
	}()

	select {
	case <-engineCalled:
		close(releaseEOF)
	case <-time.After(time.Second):
		close(releaseEOF)
		_ = pipeReader.CloseWithError(errors.New("handler streaming test timed out"))
		t.Fatal("handler parsed the complete multipart body before calling the upload service")
	}
	if err := <-producerDone; err != nil {
		t.Fatalf("multipart producer failed: %v", err)
	}
	<-requestDone
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", resp.Code, resp.Body.String())
	}
}

func TestKnowledgeHandler_ListDocumentsBindsAdvancedFilters(t *testing.T) {
	var gotReq knowledge.DocumentListRequest
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		listDocsFunc: func(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
			if datasetID != "kb1" {
				t.Fatalf("datasetID = %q, want kb1", datasetID)
			}
			gotReq = req
			return &knowledge.DocumentListResult{Total: 0}, nil
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/documents?page=2&page_size=20&suffix=pdf&suffix=docx&run=1&run=4&create_time_from=11&create_time_to=22&metadata_condition=author", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if gotReq.Page != 2 || gotReq.PageSize != 20 || gotReq.CreateTimeFrom != 11 || gotReq.CreateTimeTo != 22 || gotReq.MetadataCondition != "author" {
		t.Fatalf("query binding failed: %#v", gotReq)
	}
	if len(gotReq.Suffix) != 2 || gotReq.Suffix[0] != "pdf" || gotReq.Suffix[1] != "docx" {
		t.Fatalf("suffix = %#v", gotReq.Suffix)
	}
	if len(gotReq.Run) != 2 || gotReq.Run[0] != "1" || gotReq.Run[1] != "4" {
		t.Fatalf("run = %#v", gotReq.Run)
	}
}

func TestKnowledgeHandler_DownloadDocumentStreamsGatewayResponse(t *testing.T) {
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		downloadFunc: func(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error) {
			if datasetID != "kb1" || documentID != "doc1" {
				t.Fatalf("ids = %q/%q, want kb1/doc1", datasetID, documentID)
			}
			return &knowledge.StreamResult{
				Body:               io.NopCloser(strings.NewReader("file-content")),
				ContentType:        "application/pdf",
				ContentDisposition: `attachment; filename="guide.pdf"`,
				ContentLength:      int64(len("file-content")),
			}, nil
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/documents/doc1/download", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if resp.Body.String() != "file-content" {
		t.Fatalf("body = %q", resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header().Get("Content-Disposition"); got != `attachment; filename="guide.pdf"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
}

func TestKnowledgeHandler_GetImageStreamsGatewayResponse(t *testing.T) {
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		imageFunc: func(ctx context.Context, imageID string) (*knowledge.StreamResult, error) {
			if imageID != "img1" {
				t.Fatalf("imageID = %q, want img1", imageID)
			}
			return &knowledge.StreamResult{
				Body:          io.NopCloser(strings.NewReader("jpeg")),
				ContentType:   "image/jpeg",
				ContentLength: int64(len("jpeg")),
			}, nil
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/images/img1", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if resp.Body.String() != "jpeg" || resp.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("unexpected image response: content-type=%q body=%q", resp.Header().Get("Content-Type"), resp.Body.String())
	}
}

func TestKnowledgeHandler_GetImageMapsUpstreamError(t *testing.T) {
	router := setupKnowledgeRouter(&handlerFakeKnowledgeEngine{
		imageFunc: func(ctx context.Context, imageID string) (*knowledge.StreamResult, error) {
			return nil, knowledge.NewUpstreamError("multirag image response is not an image", errors.New("image not found"))
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb1/images/img1", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", resp.Code, resp.Body.String())
	}
	if resp.Header().Get("Content-Type") == "image/jpeg" {
		t.Fatalf("unexpected image content type for upstream error")
	}
	if !strings.Contains(resp.Body.String(), "not an image") || !strings.Contains(resp.Body.String(), "image not found") {
		t.Fatalf("response does not include upstream error: %s", resp.Body.String())
	}
}
