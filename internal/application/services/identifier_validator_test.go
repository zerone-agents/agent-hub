package services

import (
	"strings"
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		entity  string
		value   string
		wantErr bool
	}{
		{name: "plain ascii", entity: "Agent", value: "coder", wantErr: false},
		{name: "with digits", entity: "Agent", value: "agent-01", wantErr: false},
		{name: "with dot", entity: "Skill", value: "baoyu.skill.v1", wantErr: false},
		{name: "with underscore", entity: "Tool", value: "web_search", wantErr: false},
		{name: "with hyphen", entity: "Scene", value: "default-scene", wantErr: false},
		{name: "all allowed chars", entity: "Skill", value: "Aa0._-z", wantErr: false},
		{name: "single char", entity: "Tool", value: "x", wantErr: false},
		{name: "empty string", entity: "Agent", value: "", wantErr: true},
		{name: "whitespace only is literal space", entity: "Skill", value: " ", wantErr: true},
		{name: "contains space", entity: "Agent", value: "my agent", wantErr: true},
		{name: "chinese chars", entity: "Scene", value: "场景", wantErr: true},
		{name: "slash", entity: "Provider key", value: "a/b", wantErr: true},
		{name: "colon for port-like", entity: "Provider key", value: "host:8080", wantErr: true},
		{name: "at sign", entity: "Agent", value: "user@x", wantErr: true},
		{name: "exceeds 64 chars", entity: "Agent", value: strings.Repeat("a", 65), wantErr: true},
		{name: "exactly 64 chars", entity: "Agent", value: strings.Repeat("a", 64), wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIdentifier(tt.entity, tt.value)
			if tt.wantErr && err == nil {
				t.Errorf("validateIdentifier(%q, %q) expected error, got nil", tt.entity, tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateIdentifier(%q, %q) returned unexpected error: %v", tt.entity, tt.value, err)
			}
		})
	}
}

// TestValidateIdentifier_EntityInMessage ensures the entity label surfaces in
// error messages so users can tell which field failed validation.
func TestValidateIdentifier_EntityInMessage(t *testing.T) {
	if err := validateIdentifier("Agent", ""); err == nil || !strings.Contains(err.Error(), "Agent") {
		t.Errorf("expected error to contain entity label, got %v", err)
	}
	if err := validateIdentifier("Provider key", "a b"); err == nil || !strings.Contains(err.Error(), "Provider key") {
		t.Errorf("expected error to contain entity label, got %v", err)
	}
}
