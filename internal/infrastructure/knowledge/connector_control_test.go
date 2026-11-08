package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectorResumeUsesStatusPatch(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "resume"}[resume], func(t *testing.T) {
			status := "2"
			if resume {
				status = "5"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/connectors/c1" || r.Header.Get("Authorization") != "Bearer owned-key" {
					t.Errorf("unexpected connector request: %s %s", r.Method, r.URL.Path)
				}
				var body domain.Object
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body, domain.Object{"status": status}) {
					t.Errorf("unexpected status payload: %v %v", body, err)
				}
				_, _ = w.Write([]byte(`{"code":0,"data":true}`))
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "owned-key", time.Second, time.Hour)
			body := domain.Object{"resume": resume, "status": "forged"}
			if _, err := client.KnowledgePage(context.Background(), domain.PageRequest{Operation: "connector-resume", DatasetID: "kb", ConnectorID: "c1", Body: body}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(body, domain.Object{"resume": resume, "status": "forged"}) {
				t.Fatal("adapter mutated the caller body", body)
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
