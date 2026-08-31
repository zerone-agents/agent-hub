package services

import (
	"strings"
	"testing"
)

// TestValidateToolName covers the tool-specific rules layered on top of
// validateIdentifier: the deployer artifact-name contract forbids "." and ".."
// in addition to the shared charset/length rules.
func TestValidateToolName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "dot rejected", value: ".", wantErr: true},
		{name: "double dot rejected", value: "..", wantErr: true},
		{name: "normal name accepted", value: "web_search", wantErr: false},
		{name: "dotted name still accepted", value: "baoyu.tool.v1", wantErr: false},
		{name: "empty string still rejected", value: "", wantErr: true},
		{name: "charset still enforced", value: "my tool", wantErr: true},
		{name: "slash still rejected", value: "a/b", wantErr: true},
		{name: "exceeds 64 chars still rejected", value: strings.Repeat("a", 65), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolName(tt.value)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateToolName(%q) expected error, got nil", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateToolName(%q) returned unexpected error: %v", tt.value, err)
			}
		})
	}
}
