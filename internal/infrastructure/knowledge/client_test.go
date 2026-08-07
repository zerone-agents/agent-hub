package knowledge

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

	domain "control-panel/internal/domain/knowledge"
)

func TestListDatasets_AuthorizationPaginationAndFieldMapping(t *testing.T) {
	var gotAuth string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/api/v1/datasets" {
			t.Errorf("path = %q, want /api/v1/datasets", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": []map[string]any{
				{
					"id":              "kb1",
					"name":            "Docs",
					"document_count":  float64(2),
					"chunk_count":     float64(3),
					"chunk_method":    "naive",
					"embedding_model": "text-embedding@test",
				},
			},
			"total_datasets": 7,
		})
	}))
	defer server.Close()

	desc := true
	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	result, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{
		Page:     2,
		PageSize: 5,
		OrderBy:  "update_time",
		Desc:     &desc,
	})
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	for _, want := range []string{"page=2", "page_size=5", "orderby=update_time", "desc=true"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
	if result.Total != 7 {
		t.Errorf("Total = %d, want 7", result.Total)
	}
	got := map[string]any(result.Datasets[0])
	if got["doc_num"] != float64(2) || got["chunk_num"] != float64(3) || got["parser_id"] != "naive" || got["embd_id"] != "text-embedding@test" {
		t.Errorf("field mapping failed: %#v", got)
	}
	for _, oldKey := range []string{"document_count", "chunk_count", "chunk_method", "embedding_model"} {
		if _, ok := got[oldKey]; ok {
			t.Errorf("unexpected remote key %q in mapped dataset: %#v", oldKey, got)
		}
	}
}

func TestListDatasets_FiltersByControlPanelDisplayName(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": []map[string]any{
				{
					"id":   "kb1",
					"name": "kb_physical_one",
					"parser_config": map[string]any{
						"control_panel": map[string]any{"display_name": "校园网服务指南"},
					},
				},
				{
					"id":          "kb2",
					"name":        "kb_physical_two",
					"description": "英文资料",
					"parser_config": map[string]any{
						"control_panel": map[string]any{"display_name": "财务制度"},
					},
				},
			},
			"total_datasets": 2,
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	result, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{
		Page:     1,
		PageSize: 10,
		Keywords: "校园",
	})
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if strings.Contains(gotQuery, "keywords=") {
		t.Fatalf("keywords should be filtered in gateway for display names, query=%q", gotQuery)
	}
	if !strings.Contains(gotQuery, "page=1") || !strings.Contains(gotQuery, "page_size=1000") {
		t.Fatalf("query should fetch enough rows for display-name filtering, got %q", gotQuery)
	}
	if result.Total != 1 || len(result.Datasets) != 1 {
		t.Fatalf("unexpected filtered result: %#v", result)
	}
	got := map[string]any(result.Datasets[0])
	if got["name"] != "校园网服务指南" || got["display_name"] != "校园网服务指南" || got["collection_name"] != "kb_physical_one" {
		t.Fatalf("display/collection mapping failed: %#v", got)
	}
}

func TestCreateDataset_UsesStrictCreateBodyAndPostCreateDisplayBridge(t *testing.T) {
	var gotCreateBody map[string]any
	var gotUpdateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/datasets":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("decode create request body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"retcode": 0,
				"data": map[string]any{
					"id":              "kb1",
					"name":            gotCreateBody["name"],
					"chunk_method":    gotCreateBody["chunk_method"],
					"embedding_model": gotCreateBody["embedding_model"],
					"parser_config":   gotCreateBody["parser_config"],
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/datasets/kb1":
			if err := json.NewDecoder(r.Body).Decode(&gotUpdateBody); err != nil {
				t.Fatalf("decode update request body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"retcode": 0,
				"data": map[string]any{
					"id":              "kb1",
					"name":            gotCreateBody["name"],
					"chunk_method":    gotUpdateBody["chunk_method"],
					"embedding_model": gotUpdateBody["embedding_model"],
					"parser_config":   gotUpdateBody["parser_config"],
				},
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	dataset, err := client.CreateDataset(context.Background(), domain.DatasetMutationRequest{
		"name":      "校园网服务指南",
		"parser_id": "manual",
		"embd_id":   "bge@test",
		"parser_config": map[string]any{
			"layout_recognize":           "DeepDOC",
			"chunk_token_num":            256,
			"delimiter":                  "\n",
			"enable_children":            true,
			"children_delimiter":         "\n###",
			"image_table_context_window": 5,
			"image_context_size":         5,
			"table_context_size":         5,
			"auto_keywords":              2,
			"auto_questions":             4,
			"toc_extraction":             true,
			"html4excel":                 true,
			"mineru_parse_method":        "auto",
			"mineru_lang":                "Chinese",
			"mineru_formula_enable":      false,
			"mineru_table_enable":        true,
			"raptor":                     map[string]any{"use_raptor": true, "unexpected": true},
			"graphrag":                   map[string]any{"enabled": false, "unexpected": true},
			"metadata":                   map[string]any{"source": "manual"},
			"control_panel":              map[string]any{"display_name": "old"},
		},
	})
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if gotCreateBody["parser_id"] != nil || gotCreateBody["embd_id"] != nil {
		t.Errorf("remote body leaked local fields: %#v", gotCreateBody)
	}
	if gotCreateBody["chunk_method"] != "manual" || gotCreateBody["embedding_model"] != "bge@test" {
		t.Errorf("reverse mapping failed: %#v", gotCreateBody)
	}
	physicalName, _ := gotCreateBody["name"].(string)
	if !strings.HasPrefix(physicalName, "kb_") || physicalName == "校园网服务指南" {
		t.Fatalf("physical dataset name = %q, want generated kb_* name", physicalName)
	}
	if _, ok := gotCreateBody["display_name"]; ok {
		t.Fatalf("display_name should not be sent to current multirag: %#v", gotCreateBody)
	}
	createConfig, ok := gotCreateBody["parser_config"].(map[string]any)
	if !ok {
		t.Fatalf("create parser_config missing: %#v", gotCreateBody)
	}
	for _, forbidden := range []string{
		"control_panel",
		"enable_children",
		"children_delimiter",
		"image_table_context_window",
		"image_context_size",
		"table_context_size",
		"toc_extraction",
		"mineru_parse_method",
		"mineru_lang",
		"mineru_formula_enable",
		"mineru_table_enable",
		"metadata",
	} {
		if _, ok := createConfig[forbidden]; ok {
			t.Fatalf("create parser_config should not include %q: %#v", forbidden, createConfig)
		}
	}
	parentChild, ok := createConfig["parent_child"].(map[string]any)
	if !ok || parentChild["use_parent_child"] != true || parentChild["children_delimiter"] != "\n###" {
		t.Fatalf("parent_child mapping failed: %#v", createConfig)
	}
	raptor, ok := createConfig["raptor"].(map[string]any)
	if !ok || raptor["use_raptor"] != true || raptor["unexpected"] != nil {
		t.Fatalf("strict raptor config failed: %#v", createConfig["raptor"])
	}
	graphrag, ok := createConfig["graphrag"].(map[string]any)
	if !ok || graphrag["use_graphrag"] != false || graphrag["enabled"] != nil || graphrag["unexpected"] != nil {
		t.Fatalf("strict graphrag config failed: %#v", createConfig["graphrag"])
	}
	if gotUpdateBody == nil {
		t.Fatal("expected post-create update request")
	}
	if _, ok := gotUpdateBody["name"]; ok {
		t.Fatalf("post-create update should not rename physical collection: %#v", gotUpdateBody)
	}
	updateConfig, ok := gotUpdateBody["parser_config"].(map[string]any)
	if !ok {
		t.Fatalf("post-create parser_config missing: %#v", gotUpdateBody)
	}
	controlPanel, ok := updateConfig["control_panel"].(map[string]any)
	if !ok || controlPanel["display_name"] != "校园网服务指南" {
		t.Fatalf("display bridge missing: %#v", updateConfig)
	}
	if updateConfig["image_context_size"] != 5.0 && updateConfig["image_context_size"] != 5 {
		t.Fatalf("post-create update should preserve advanced parser config: %#v", updateConfig)
	}
	got := map[string]any(*dataset)
	if got["parser_id"] != "manual" || got["embd_id"] != "bge@test" {
		t.Errorf("response mapping failed: %#v", got)
	}
	if got["name"] != "校园网服务指南" || got["display_name"] != "校园网服务指南" || got["collection_name"] != physicalName {
		t.Errorf("display response mapping failed: %#v", got)
	}
}

func TestUpdateDataset_StoresDisplayNameWithoutRenamingCollection(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets/kb1" {
			t.Errorf("path = %q, want /api/v1/datasets/kb1", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"retcode": 0,
			"data": map[string]any{
				"id":            "kb1",
				"name":          "kb_physical_one",
				"parser_config": gotBody["parser_config"],
			},
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	dataset, err := client.UpdateDataset(context.Background(), "kb1", domain.DatasetMutationRequest{
		"name": "新中文名",
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if _, ok := gotBody["name"]; ok {
		t.Fatalf("update should not rename current multirag physical collection: %#v", gotBody)
	}
	parserConfig, ok := gotBody["parser_config"].(map[string]any)
	if !ok {
		t.Fatalf("parser_config missing display bridge: %#v", gotBody)
	}
	controlPanel, ok := parserConfig["control_panel"].(map[string]any)
	if !ok || controlPanel["display_name"] != "新中文名" {
		t.Fatalf("display bridge missing: %#v", parserConfig)
	}
	got := map[string]any(*dataset)
	if got["name"] != "新中文名" || got["collection_name"] != "kb_physical_one" {
		t.Fatalf("display response mapping failed: %#v", got)
	}
}

func TestClient_RetCodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"retcode": 101,
			"retmsg":  "bad request",
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	_, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{})
	if err == nil || !strings.Contains(err.Error(), "multirag error 101: bad request") {
		t.Fatalf("expected retcode error, got %v", err)
	}
}

func TestClient_Non2xxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream down")
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	_, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected HTTP 502 error, got %v", err)
	}
}

func TestClient_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{not json")
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	_, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{})
	if err == nil || !strings.Contains(err.Error(), "解析 multirag 响应失败") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
}

func TestUploadDocuments_Multipart(t *testing.T) {
	var gotAuth string
	var gotContent string
	var gotContentLength int64
	var gotTransferEncoding []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentLength = r.ContentLength
		gotTransferEncoding = append([]string(nil), r.TransferEncoding...)
		if r.URL.Path != "/api/v1/datasets/kb1/documents" {
			t.Errorf("path = %q, want /api/v1/datasets/kb1/documents", r.URL.Path)
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, _, err := r.FormFile("files")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		gotContent = string(content)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": []map[string]any{
				{
					"id":           "doc1",
					"name":         "a.txt",
					"chunk_count":  4,
					"chunk_method": "naive",
				},
			},
		})
	}))
	defer server.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "a.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = part.Write([]byte("hello"))
	if err := writer.WriteField("parent_path", "videos/2026"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}
	payload := append([]byte(nil), body.Bytes()...)

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	docs, err := client.UploadDocuments(context.Background(), "kb1", domain.UploadRequest{
		Body:          bytes.NewReader(payload),
		ContentType:   writer.FormDataContentType(),
		ContentLength: int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("UploadDocuments failed: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotContent != "hello" {
		t.Errorf("uploaded content = %q, want hello", gotContent)
	}
	if gotContentLength != int64(len(payload)) {
		t.Errorf("Content-Length = %d, want %d", gotContentLength, len(payload))
	}
	if len(gotTransferEncoding) != 0 {
		t.Errorf("Transfer-Encoding = %v, want none for known content length", gotTransferEncoding)
	}
	got := map[string]any(docs[0])
	if got["chunk_num"] != float64(4) && got["chunk_num"] != 4 {
		t.Errorf("chunk_count mapping failed: %#v", got)
	}
	if got["parser_id"] != "naive" {
		t.Errorf("chunk_method mapping failed: %#v", got)
	}
}

func TestUploadDocuments_StreamsBeforeSourceEOF(t *testing.T) {
	firstChunkSeen := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader: %v", err)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Errorf("NextPart: %v", err)
			return
		}
		chunk := make([]byte, len("hello"))
		if _, err := io.ReadFull(part, chunk); err != nil {
			t.Errorf("ReadFull: %v", err)
			return
		}
		if string(chunk) != "hello" {
			t.Errorf("first chunk = %q, want hello", chunk)
		}
		close(firstChunkSeen)
		_, _ = io.Copy(io.Discard, part)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []map[string]any{}})
	}))
	defer server.Close()

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()
	releaseEOF := make(chan struct{})
	producerDone := make(chan error, 1)
	go func() {
		part, err := writer.CreateFormFile("files", "large.mp4")
		if err == nil {
			_, err = part.Write([]byte("hello"))
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

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Second)
	uploadDone := make(chan error, 1)
	go func() {
		_, err := client.UploadDocuments(context.Background(), "kb1", domain.UploadRequest{
			Body:          pipeReader,
			ContentType:   contentType,
			ContentLength: -1,
		})
		uploadDone <- err
	}()

	select {
	case <-firstChunkSeen:
		close(releaseEOF)
	case <-time.After(time.Second):
		close(releaseEOF)
		_ = pipeReader.CloseWithError(errors.New("streaming test timed out"))
		t.Fatal("upstream did not receive data before the multipart source reached EOF")
	}
	if err := <-producerDone; err != nil {
		t.Fatalf("multipart producer failed: %v", err)
	}
	if err := <-uploadDone; err != nil {
		t.Fatalf("UploadDocuments failed: %v", err)
	}
}

func TestUploadDocuments_UsesDedicatedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(75 * time.Millisecond)
		if r.Method == http.MethodPost {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 0, "datasets": []any{}}})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", 20*time.Millisecond, time.Second)
	if _, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{}); err == nil {
		t.Fatal("ordinary API call should use the short timeout")
	}
	if _, err := client.UploadDocuments(context.Background(), "kb1", domain.UploadRequest{
		Body:          strings.NewReader("--boundary--\r\n"),
		ContentType:   "multipart/form-data; boundary=boundary",
		ContentLength: int64(len("--boundary--\r\n")),
	}); err != nil {
		t.Fatalf("upload should use the dedicated timeout: %v", err)
	}
}

func TestListDocuments_QueryPassthrough(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/api/v1/datasets/kb1/documents" {
			t.Errorf("path = %q, want /api/v1/datasets/kb1/documents", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"total": 1,
				"docs":  []map[string]any{{"id": "doc1", "chunk_count": 3}},
			},
		})
	}))
	defer server.Close()

	desc := true
	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	result, err := client.ListDocuments(context.Background(), "kb1", domain.DocumentListRequest{
		Page:              2,
		PageSize:          20,
		OrderBy:           "create_time",
		Desc:              &desc,
		Keywords:          "guide",
		Suffix:            []string{"pdf", "docx"},
		Run:               []string{"1", "4"},
		CreateTimeFrom:    11,
		CreateTimeTo:      22,
		MetadataCondition: "author",
	})
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	for _, want := range []string{"page=2", "page_size=20", "orderby=create_time", "desc=true", "keywords=guide", "suffix=pdf", "suffix=docx", "run=1", "run=4", "create_time_from=11", "create_time_to=22", "metadata_condition=author"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
	if result.Total != 1 || len(result.Documents) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := map[string]any(result.Documents[0]); got["chunk_num"] != float64(3) {
		t.Fatalf("chunk_count mapping failed: %#v", got)
	}
}

func TestListChunks_RichFieldMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets/kb1/documents/doc1/chunks" {
			t.Errorf("path = %q, want chunks path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"total": 1,
				"chunks": []map[string]any{
					{
						"chunk_id":            "c1",
						"content_with_weight": "hello",
						"doc_id":              "doc1",
						"important_kwd":       []string{"k"},
						"question_kwd":        []string{"q"},
						"img_id":              "img1",
						"position_int":        []int{1, 2, 3},
						"doc_type_kwd":        "image",
						"tag_kwd":             []string{"tag"},
						"tag_feas":            map[string]any{"source": "x"},
					},
				},
				"doc": map[string]any{"id": "doc1", "chunk_count": 1},
			},
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	result, err := client.ListChunks(context.Background(), "kb1", "doc1", domain.ChunkListRequest{})
	if err != nil {
		t.Fatalf("ListChunks failed: %v", err)
	}
	got := map[string]any(result.Chunks[0])
	if got["id"] != "c1" || got["content"] != "hello" || got["document_id"] != "doc1" {
		t.Fatalf("core mapping failed: %#v", got)
	}
	if got["image_id"] != "img1" || got["doc_type"] != "image" || got["positions"] == nil || got["tag_kwd"] == nil || got["tag_feas"] == nil {
		t.Fatalf("rich mapping failed: %#v", got)
	}
}

func TestCreateChunk_PassesImageAndTagFields(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets/kb1/documents/doc1/chunks" {
			t.Errorf("path = %q, want chunks path", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"chunk": map[string]any{"id": "c1"}},
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	_, err := client.CreateChunk(context.Background(), "kb1", "doc1", domain.ChunkMutationRequest{
		"content":            "hello",
		"important_keywords": []string{"k"},
		"questions":          []string{"q"},
		"image_base64":       "abc",
		"tag_kwd":            []string{"tag"},
		"tag_feas":           map[string]any{"source": "manual"},
	})
	if err != nil {
		t.Fatalf("CreateChunk failed: %v", err)
	}
	for _, key := range []string{"content", "important_keywords", "questions", "image_base64", "tag_kwd", "tag_feas"} {
		if _, ok := gotBody[key]; !ok {
			t.Fatalf("body missing %q: %#v", key, gotBody)
		}
	}
	if gotBody["image_base64"] != "abc" {
		t.Fatalf("image_base64 = %#v, want pure base64", gotBody["image_base64"])
	}
}

func TestDownloadDocument_Stream(t *testing.T) {
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		if r.URL.Path != "/api/v1/datasets/kb1/documents/doc1" {
			t.Errorf("path = %q, want document download path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="guide.pdf"`)
		_, _ = io.WriteString(w, "pdf")
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	stream, err := client.DownloadDocument(context.Background(), "kb1", "doc1")
	if err != nil {
		t.Fatalf("DownloadDocument failed: %v", err)
	}
	defer stream.Body.Close()
	body, _ := io.ReadAll(stream.Body)
	if string(body) != "pdf" || stream.ContentType != "application/pdf" || !strings.Contains(gotAccept, "application/octet-stream") {
		t.Fatalf("unexpected stream: body=%q contentType=%q accept=%q", string(body), stream.ContentType, gotAccept)
	}
	if stream.ContentDisposition != `attachment; filename="guide.pdf"` {
		t.Fatalf("ContentDisposition = %q", stream.ContentDisposition)
	}
}

func TestDownloadDocument_RejectsJSONEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets/kb1/documents/doc1" {
			t.Errorf("path = %q, want document download path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"retcode": 102,
			"retmsg":  "This document has been deleted",
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	stream, err := client.DownloadDocument(context.Background(), "kb1", "doc1")
	if err == nil {
		if stream != nil && stream.Body != nil {
			stream.Body.Close()
		}
		t.Fatal("expected JSON stream error")
	}
	if !strings.Contains(err.Error(), "This document has been deleted") {
		t.Fatalf("error = %v, want upstream JSON message", err)
	}
}

func TestGetImage_UsesProxySourceRoute(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/document/image/img1" {
			t.Errorf("path = %q, want image route", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "png")
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	stream, err := client.GetImage(context.Background(), "img1")
	if err != nil {
		t.Fatalf("GetImage failed: %v", err)
	}
	defer stream.Body.Close()
	body, _ := io.ReadAll(stream.Body)
	if string(body) != "png" || stream.ContentType != "image/png" {
		t.Fatalf("unexpected image stream: body=%q type=%q", string(body), stream.ContentType)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestGetImage_RejectsJSONErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/document/image/img1" {
			t.Errorf("path = %q, want image route", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"retcode": 102,
			"retmsg":  "image not found",
		})
	}))
	defer server.Close()

	client := NewRemoteMultiragEngine(server.URL, "test-key", time.Second, time.Hour)
	stream, err := client.GetImage(context.Background(), "img1")
	if stream != nil {
		t.Fatalf("stream = %#v, want nil", stream)
	}
	if err == nil || !strings.Contains(err.Error(), "image not found") {
		t.Fatalf("expected upstream JSON error, got %v", err)
	}
}
