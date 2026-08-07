package provider

import "testing"

func TestMapModelTypeToMultiRAG(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"llm", "chat"},
		{"ocr", "ocr"},
		{"embedding", "embedding"},
		{"vlm", "image2text"},
		{"", ""},
		{"unknown", ""},
	}
	for _, tc := range tests {
		got := MapModelTypeToMultiRAG(tc.in)
		if got != tc.want {
			t.Errorf("MapModelTypeToMultiRAG(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
