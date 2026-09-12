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
	examples, ok := data["examples"].([]any)
	if !ok || len(examples) != 2 {
		t.Fatalf("examples = %#v", data["examples"])
	}
	for _, raw := range examples {
		example := raw.(map[string]any)
		if example["id"] == "" || example["manifest"] == "" {
			t.Fatalf("incomplete example: %+v", example)
		}
	}
}

func TestExtensionVerificationValidate(t *testing.T) {
	r := extensionVerificationRouter()
	example := extensionExamples()[0]
	body, _ := json.Marshal(map[string]string{"manifest": example.Manifest})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions/validate", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	data := decodeResponseData(t, w)
	if data["valid"] != true || data["package"] == nil {
		t.Fatalf("validation data = %+v", data)
	}
	if got := len(data["contributions"].([]any)); got != 6 {
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
