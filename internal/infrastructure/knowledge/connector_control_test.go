package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectorResumeUsesDeployedPostWithoutPatch(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "resume"}[resume], func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/connectors/c1/resume" || r.Header.Get("Authorization") != "Bearer owned-key" {
					t.Errorf("unexpected connector request: %s %s", r.Method, r.URL.Path)
				}
				var body domain.Object
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body, domain.Object{"resume": resume}) {
					t.Errorf("unexpected resume payload: %v %v", body, err)
				}
				_, _ = w.Write([]byte(`{"code":0,"data":true}`))
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "owned-key", time.Second, time.Hour)
			body := domain.Object{"resume": resume, "status": "forged"}
			result, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: body})
			if err != nil || result != true || calls.Load() != 1 {
				t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
			}
			if !reflect.DeepEqual(body, domain.Object{"resume": resume, "status": "forged"}) {
				t.Fatal("adapter mutated the caller body", body)
			}
		})
	}
}

func TestConnectorResumeFallsBackOnlyAfterRetiredPostRoute(t *testing.T) {
	for _, retired := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		for _, observed := range []string{"2", "5", "1"} {
			resume := observed != "2"
			t.Run(http.StatusText(retired)+"/"+observed, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					call := calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer key" {
						t.Error("missing authorization")
					}
					var body domain.Object
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if call == 1 {
						if r.Method != http.MethodPost || r.URL.Path != "/api/v1/connectors/c1/resume" || !reflect.DeepEqual(body, domain.Object{"resume": resume}) {
							t.Errorf("unexpected legacy request: %s %s %v", r.Method, r.URL.Path, body)
						}
						w.WriteHeader(retired)
						_, _ = io.WriteString(w, "route retired")
						return
					}
					status := "2"
					if resume {
						status = "5"
					}
					if call != 2 || r.Method != http.MethodPatch || r.URL.Path != "/api/v1/connectors/c1" || !reflect.DeepEqual(body, domain.Object{"status": status}) {
						t.Errorf("unexpected fallback: call=%d %s %s %v", call, r.Method, r.URL.Path, body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"id": "c1", "status": observed}})
				}))
				defer server.Close()
				body := domain.Object{"resume": resume, "status": "forged", "config": domain.Object{"private": true}}
				client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
				result, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: body})
				if err != nil || result == nil || calls.Load() != 2 {
					t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
				}
				if !reflect.DeepEqual(body, domain.Object{"resume": resume, "status": "forged", "config": domain.Object{"private": true}}) {
					t.Fatal("fallback mutated the caller body", body)
				}
			})
		}
	}
}

func TestConnectorResumeRejectsFailuresWithoutFallback(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"business", 200, `{"code":102,"data":false}`},
		{"business-retired-code", 200, `{"code":404,"data":false}`},
		{"conflicting-codes", 200, `{"code":102,"retcode":0,"data":true}`},
		{"false", 200, `{"code":0,"data":false}`},
		{"null", 200, `{"code":0,"data":null}`},
		{"missing-data", 200, `{"code":0}`},
		{"missing-code", 200, `{"data":true}`},
		{"wrong-data", 200, `{"code":0,"data":{"id":"c1","status":"2"}}`},
		{"coerced-data", 200, `{"code":0,"data":"true"}`},
		{"non-json", 200, `not json`},
		{"bad-request", 400, `{}`},
		{"unauthorized", 401, `{}`},
		{"forbidden", 403, `{}`},
		{"conflict", 409, `{}`},
		{"rate-limit", 429, `{}`},
		{"server-error", 500, `{}`},
		{"unavailable", 503, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost {
					t.Error("failure replayed a write", r.Method)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: domain.Object{"resume": true}})
			if err == nil || result != nil || calls.Load() != 1 {
				t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func TestConnectorResumeFallbackMustConfirmConnector(t *testing.T) {
	for _, body := range []string{
		`{"code":0,"data":false}`, `{"code":0,"data":null}`, `{"code":0}`, `{"code":0,"data":true}`,
		`{"code":0,"data":{}}`, `{"code":0,"data":{"id":"foreign","status":"5"}}`,
		`{"code":0,"data":{"id":"c1","status":"2"}}`, `{"code":0,"data":{"id":"c1","status":5}}`,
		`{"code":102,"data":{"id":"c1","status":"5"}}`, `not json`,
	} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: domain.Object{"resume": true}})
			if err == nil || result != nil || calls.Load() != 2 {
				t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func TestConnectorResumeTransportFailureNeverFallsBack(t *testing.T) {
	for _, failure := range []error{errors.New("private transport failure"), context.DeadlineExceeded, context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			var calls atomic.Int32
			client := NewRemoteMultiragEngine("http://example.invalid", "key", time.Second, time.Hour)
			client.client.Transport = imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, failure
			})
			result, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: domain.Object{"resume": true}})
			if err == nil || result != nil || calls.Load() != 1 || strings.Contains(err.Error(), "private") {
				t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func TestConnectorResumeNeverRetriesFailedFallback(t *testing.T) {
	for _, status := range []int{401, 403, 404, 405, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: domain.Object{"resume": false}})
			if err == nil || result != nil || calls.Load() != 2 {
				t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func TestConnectorResumeRejectsInvalidBooleanBeforeRemoteIO(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := NewRemoteMultiragEngine(server.URL, "owned-key", time.Second, time.Hour)
	for _, body := range []domain.Object{nil, {}, {"resume": nil}, {"resume": "true"}, {"resume": 1}, {"status": "5"}} {
		if _, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: body}); err == nil || domain.StatusCode(err) != http.StatusBadRequest {
			t.Fatal("invalid resume must fail before remote IO", body, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid resume called MultiRAG", calls.Load())
	}
}
