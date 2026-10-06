package handler

import (
	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDocumentFiltersAllowlistAndEmptyResults(t *testing.T) {
	for _, result := range []string{
		`{"code":0,"data":{"total":0,"filter":{"suffix":{},"run_status":{},"metadata":{"empty_metadata":{"true":0}}}}}`,
		`{"code":0,"data":{"total":2,"filter":{"suffix":{"pdf":2},"run_status":{"3":1,"4":1},"metadata":{"version":{"v2":2}}}}}`,
		`{"code":109,"message":"denied","data":false}`,
		`{"code":0,"data":null}`,
		`{"data":{"total":0,"filter":{}}}`,
		`{"code":0,"data":{"total":0,"filter":{"suffix":{},"run_status":{}}}}`,
	} {
		t.Run(result, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/api/v1/datasets/kb/documents", r.URL.Path)
				query := r.URL.Query()
				require.Equal(t, "filter", query.Get("type"))
				require.Equal(t, []string{"3", "4"}, query["run"])
				require.Equal(t, "pdf", query.Get("suffix"))
				require.Equal(t, "doc", query.Get("keywords"))
				for _, key := range []string{"page", "page_size", "metadata", "id", "ids"} {
					require.NotContains(t, query, key)
				}
				_, _ = w.Write([]byte(result))
			}))
			defer upstream.Close()
			router := managementTestRouter(upstream.URL)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/knowledge/datasets/kb/documents/filters?run=3&run=4&suffix=pdf&keywords=doc&page=100&page_size=1000&metadata=bad&id=other", nil))
			if strings.Contains(result, `"run_status":{`) && strings.Contains(result, `"metadata":{`) {
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"success":true`)
			} else {
				require.Equal(t, 502, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"success":false`)
			}
		})
	}
}

func TestIndexCancelOnlyTargetsCurrentDatasetBinding(t *testing.T) {
	for _, scenario := range []string{"success", "completed-race", "failed-race", "pending-readback", "completed-old-marker", "failed-old-marker", "recorded-new-marker", "stale", "empty", "denied-read", "terminal", "missing-progress", "denied-cancel", "false-ack", "null-ack", "missing-code", "readback-changed", "readback-failed"} {
		t.Run(scenario, func(t *testing.T) {
			reads, cancels := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == "POST" {
					cancels++
					require.Equal(t, "/api/v1/tasks/current/cancel", r.URL.Path)
					body := `{"code":0,"data":true}`
					switch scenario {
					case "denied-cancel":
						body = `{"code":109,"data":false}`
					case "false-ack":
						body = `{"code":0,"data":false}`
					case "null-ack":
						body = `{"code":0,"data":null}`
					case "missing-code":
						body = `{"data":true}`
					}
					_, _ = w.Write([]byte(body))
					return
				}
				reads++
				require.Equal(t, "/api/v1/datasets/kb/index", r.URL.Path)
				require.Equal(t, "graph", r.URL.Query().Get("type"))
				if scenario == "denied-read" || (reads > 1 && scenario == "readback-failed") {
					_, _ = w.Write([]byte(`{"code":109,"data":false}`))
					return
				}
				task := map[string]any{"id": "current", "progress": 0.5}
				if scenario == "failed-old-marker" || scenario == "recorded-new-marker" {
					task["progress_msg"] = "old [cancel_requested]"
				}
				if scenario == "stale" || (reads > 1 && scenario == "readback-changed") {
					task["id"] = "another"
				}
				if scenario == "empty" {
					task = map[string]any{}
				}
				if scenario == "terminal" {
					task["progress"] = 1
				}
				if scenario == "missing-progress" {
					delete(task, "progress")
				}
				if reads > 1 {
					task["progress"] = -1
					task["progress_msg"] = "[cancel_requested]"
					switch scenario {
					case "completed-race":
						task["progress"], task["progress_msg"] = 1, "completed"
					case "failed-race":
						task["progress_msg"] = "natural failure"
					case "pending-readback":
						task["progress"], task["progress_msg"] = 0.5, "still running"
					case "completed-old-marker":
						task["progress"], task["progress_msg"] = 1, "[cancel_requested] completed"
					case "failed-old-marker":
						task["progress_msg"] = "old [cancel_requested] natural failure"
					case "recorded-new-marker":
						task["progress_msg"] = "old [cancel_requested]\nnew [cancel_requested]"
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": task})
			}))
			defer upstream.Close()
			router := managementTestRouter(upstream.URL)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/knowledge/datasets/kb/index/cancel?type=graph", strings.NewReader(`{"task_id":"current"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			switch scenario {
			case "success", "recorded-new-marker":
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"request_accepted":true`)
				require.Contains(t, w.Body.String(), `"cancel_requested":true`)
				require.Contains(t, w.Body.String(), `"progress":-1`)
			case "completed-race", "failed-race", "pending-readback", "completed-old-marker", "failed-old-marker":
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"request_accepted":true`)
				require.Contains(t, w.Body.String(), `"cancel_requested":false`)
				require.Equal(t, 1, cancels)
				var envelope struct {
					Data struct {
						Task map[string]any `json:"task"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
				if scenario == "completed-race" || scenario == "completed-old-marker" {
					require.Equal(t, float64(1), envelope.Data.Task["progress"])
				} else if scenario == "pending-readback" {
					require.Equal(t, 0.5, envelope.Data.Task["progress"])
				} else {
					require.Equal(t, float64(-1), envelope.Data.Task["progress"])
				}
			case "stale", "empty", "terminal":
				require.Equal(t, 409, w.Code, w.Body.String())
				require.Zero(t, cancels)
			default:
				require.Equal(t, 502, w.Code, w.Body.String())
			}
			if scenario == "denied-read" || scenario == "missing-progress" {
				require.Zero(t, cancels)
			}
		})
	}
}

func TestManagementValidationNeverCallsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("invalid input reached %s", r.URL)
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	router := managementTestRouter(upstream.URL)
	for _, query := range []string{"page_size=1000", "page_size=0", "page_size=bad", "page=-1", "page=0", "page=1&page=2"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/knowledge/datasets/kb/documents?"+query, nil))
		require.Equal(t, 400, w.Code, query)
	}
	for _, tc := range []struct{ query, body string }{
		{"type=graph", `{}`}, {"type=invalid", `{"task_id":"current"}`}, {"type=graph&type=raptor", `{"task_id":"current"}`},
		{"type=graph", `{"task_id":"../other"}`}, {"type=graph", `{"task_id":null}`}, {"type=graph", `{"task_id":"current","dataset_id":"other"}`},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/knowledge/datasets/kb/index/cancel?"+tc.query, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, 400, w.Code, tc.body)
	}
}

func TestChunkIDReadUsesScopedRouteAndChecksBusinessCode(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/api/v1/datasets/kb/documents/d/chunks", r.URL.Path)
			require.Equal(t, "exact", r.URL.Query().Get("id"))
			if allowed {
				_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"chunks":[{"id":"exact","doc_id":"d","content_with_weight":"match"}],"doc":{"id":"d"}}}`))
			} else {
				_, _ = w.Write([]byte(`{"code":102,"data":false}`))
			}
		}))
		router := managementTestRouter(upstream.URL)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/knowledge/datasets/kb/documents/d/chunks?id=exact", nil))
		if allowed {
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), `"document_id":"d"`)
		} else {
			require.Equal(t, 502, w.Code)
		}
		upstream.Close()
	}
}

func managementTestRouter(baseURL string) *gin.Engine {
	router := gin.New()
	group := router.Group("")
	RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(baseURL, "key", time.Second, time.Hour), nil), nil))
	return router
}

func TestDocumentExactFilterRejectsReservedEmptyMetadataKey(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "true", r.URL.Query().Get("return_empty_metadata"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"docs":[]}}`))
	}))
	defer upstream.Close()
	router := managementTestRouter(upstream.URL)
	for _, metadata := range []string{
		`{"empty_metadata":["business-value"],"version":["v2"]}`,
		`{"empty_metadata":true}`, `{"empty_metadata":false}`, `{"empty_metadata":null}`, `{"empty_metadata":[]}`,
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents?metadata="+url.QueryEscape(metadata), nil))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"success":false`)
		require.Contains(t, w.Body.String(), "return_empty_metadata")
	}
	require.Zero(t, calls)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents?return_empty_metadata=true", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}

func TestDocumentAggregationNeverForwardsReservedMetadataMode(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "filter", r.URL.Query().Get("type"))
		for _, key := range []string{"metadata", "metadata_condition", "return_empty_metadata", "empty_metadata"} {
			require.NotContains(t, r.URL.Query(), key)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"filter":{"suffix":{},"run_status":{},"metadata":{"empty_metadata":{"true":0}}}}}`))
	}))
	defer upstream.Close()
	w := httptest.NewRecorder()
	managementTestRouter(upstream.URL).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents/filters?metadata="+url.QueryEscape(`{"empty_metadata":["business-value"]}`)+"&return_empty_metadata=true&empty_metadata=true", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
	require.Contains(t, w.Body.String(), `"empty_metadata":{"true":0}`)
}

func TestExactMetadataNullNeverReachesRemoteAndPrototypeKeyIsPreserved(t *testing.T) {
	calls := 0
	metadata := `{"__proto__":["business-value"],"constructor":["v2"],"number":[0],"bool":[false]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, metadata, r.URL.Query().Get("metadata"))
		var actual map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.URL.Query().Get("metadata")), &actual))
		require.Equal(t, []any{"business-value"}, actual["__proto__"])
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"docs":[]}}`))
	}))
	defer upstream.Close()
	router := managementTestRouter(upstream.URL)
	for _, unsafe := range []string{`{"field":[null]}`, `{"field":null}`, `{"field":[]}`, `{"field":[""]}`, `{"field":[1e309]}`} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents?metadata="+url.QueryEscape(unsafe), nil))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"success":false`)
	}
	require.Zero(t, calls)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents?metadata="+url.QueryEscape(metadata), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}

func TestBatchMetadataExclusionsNeverSelectWriters(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// The pinned MR incorrectly matches d1=[Alice,Bob],d2=[Bob]
		// for not in [Alice]. Only the positive control reaches this writer.
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		selector := body["selector"].(map[string]any)
		require.Equal(t, []any{"d1", "d2"}, selector["document_ids"])
		condition := selector["metadata_condition"].(map[string]any)["conditions"].([]any)[0].(map[string]any)
		require.Equal(t, "in", condition["comparison_operator"])
		_, _ = w.Write([]byte(`{"code":0,"data":{"updated":1,"matched_docs":1}}`))
	}))
	defer upstream.Close()
	router := managementTestRouter(upstream.URL)
	for _, op := range []string{"not in", "not contains", "!=", "≠", "not is"} {
		for _, value := range []string{`["Alice"]`, `"Alice"`, `[]`} {
			body := `{"selector":{"document_ids":["d1","d2"],"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"` + op + `","value":` + value + `}]}},"updates":[{"key":"category","value":"changed"}],"deletes":[]}`
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/knowledge/datasets/kb/documents/metadatas", strings.NewReader(body)))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"success":false`)
			require.Contains(t, w.Body.String(), op)
		}
	}
	require.Zero(t, calls)
	w := httptest.NewRecorder()
	body := `{"selector":{"document_ids":["d1","d2"],"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"in","value":["Alice"]}]}},"updates":[{"key":"category","value":"changed"}],"deletes":[]}`
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/knowledge/datasets/kb/documents/metadatas", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}

func TestBatchMetadataUnknownLogicNeverReachesWriter(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		selector := body["selector"].(map[string]any)
		require.Equal(t, []any{"d1", "d2"}, selector["document_ids"])
		condition := selector["metadata_condition"].(map[string]any)
		logic, present := condition["logic"]
		if present {
			require.Contains(t, []any{"and", "or"}, logic)
		}
		matched := 1
		if logic == "or" {
			matched = 2
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"updated": matched, "matched_docs": matched}})
	}))
	defer upstream.Close()
	router := managementTestRouter(upstream.URL)
	conditions := []any{map[string]any{"name": "x", "comparison_operator": "=", "value": "yes"}, map[string]any{"name": "y", "comparison_operator": "=", "value": "yes"}}
	run := func(condition map[string]any) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{"selector": map[string]any{"document_ids": []any{"d1", "d2"}, "metadata_condition": condition}, "updates": []any{map[string]any{"key": "category", "value": "changed"}}, "deletes": []any{}})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/knowledge/datasets/kb/documents/metadatas", strings.NewReader(string(body))))
		return w
	}
	for _, logic := range []any{"xor", "AND", "OR", "and ", "", nil, true, 1, []any{}, map[string]any{}} {
		w := run(map[string]any{"logic": logic, "conditions": conditions})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"success":false`)
	}
	require.Zero(t, calls)
	for _, condition := range []map[string]any{{"logic": "and", "conditions": conditions}, {"logic": "or", "conditions": conditions}, {"conditions": conditions}} {
		w := run(condition)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		matched := 1
		if condition["logic"] == "or" {
			matched = 2
		}
		require.Contains(t, w.Body.String(), fmt.Sprintf(`"matched_docs":%d`, matched))
	}
	require.Equal(t, 3, calls)
}

func TestExactMetadataPythonSeparatorsNeverDropRequestedConditions(t *testing.T) {
	calls := 0
	nonblank := `{"__proto__":["\u001cAlice\u001f"]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, nonblank, r.URL.Query().Get("metadata"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"docs":[]}}`))
	}))
	defer upstream.Close()
	router := managementTestRouter(upstream.URL)
	for _, r := range []rune{'\u001c', '\u001d', '\u001e', '\u001f'} {
		for _, value := range []any{string(r), []any{string(r)}, []any{"v2", " \t" + string(r) + "\n"}} {
			data, err := json.Marshal(map[string]any{"field": value})
			require.NoError(t, err)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents?metadata="+url.QueryEscape(string(data)), nil))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		}
	}
	require.Zero(t, calls)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge/datasets/kb/documents?metadata="+url.QueryEscape(nonblank), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}
