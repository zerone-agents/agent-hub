package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func extensionVerificationRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewExtensionVerificationHandler()
	r.GET("/api/v1/admin/extensions/h0", h.Overview)
	r.POST("/api/v1/admin/extensions/validate", h.Validate)
	return r
}

func decodeResponseData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if !envelope.Success {
		t.Fatalf("response failed: %s", recorder.Body.String())
	}
	return envelope.Data
}

func TestExtensionVerificationOverview(t *testing.T) {
	r := extensionVerificationRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/h0", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeResponseData(t, w)
	if data["protocolVersion"] != "v1alpha1" || data["platformVersion"] != "0.9.0-h0" || data["status"] != "可验收" {
		t.Fatalf("protocol metadata = %+v", data)
	}
	if _, bundled := data["examples"]; bundled {
		t.Fatal("production overview must not bundle consumer fixtures")
	}
}

func TestExtensionVerificationValidate(t *testing.T) {
	r := extensionVerificationRouter()
	body, _ := json.Marshal(map[string]string{"manifest": `apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata:
  name: task-state
  namespace: io.example.research
  version: 1.0.0
  displayName: Task State
  description: Generic state package
  publisher: Example
compatibility:
  hub: ">=0.9.0"
  runtimeProtocol: ">=1.0.0"
permissions:
  state: {read: [], write: []}
  events: {consume: [], emit: []}
  tools: {expose: []}
  network: {outbound: []}
contributes:
  stateSchemas:
    - {id: task, version: 1.0.0, file: schemas/task.json}
`})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions/validate", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeResponseData(t, w)
	if data["valid"] != true || data["package"] == nil {
		t.Fatalf("validation data = %+v", data)
	}
	if got := len(data["contributions"].([]any)); got != 1 {
		t.Fatalf("contribution count = %d, data=%+v", got, data)
	}
}

func TestExtensionVerificationValidationFailureIsAResult(t *testing.T) {
	r := extensionVerificationRouter()
	body, _ := json.Marshal(map[string]string{"manifest": "kind: CapabilityPackage\n"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions/validate", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeResponseData(t, w)
	if data["valid"] != false || len(data["errors"].([]any)) == 0 {
		t.Fatalf("validation data = %+v", data)
	}
}

func TestExtensionVerificationRejectsOversizedBody(t *testing.T) {
	r := extensionVerificationRouter()
	body := `{"manifest":"` + strings.Repeat("x", maxInlineManifestBytes) + `"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions/validate", strings.NewReader(body)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}
