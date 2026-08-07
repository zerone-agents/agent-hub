package provider

import "testing"

func TestValidateAttributes_Mineru(t *testing.T) {
	// Missing required backend
	if err := ValidateAttributes("mineru", map[string]AttrValue{}); err == nil {
		t.Fatal("expected error for missing backend")
	}

	// Invalid backend enum
	if err := ValidateAttributes("mineru", map[string]AttrValue{
		"backend": {Type: AttrTypeString, Value: "nope"},
	}); err == nil {
		t.Fatal("expected error for invalid backend enum")
	}

	// Invalid bool
	if err := ValidateAttributes("mineru", map[string]AttrValue{
		"backend":       {Type: AttrTypeString, Value: "pipeline"},
		"delete_output": {Type: AttrTypeBool, Value: "yes"},
	}); err == nil {
		t.Fatal("expected error for invalid bool value")
	}

	// Valid full set
	valid := map[string]AttrValue{
		"backend":       {Type: AttrTypeString, Value: "pipeline"},
		"delete_output": {Type: AttrTypeBool, Value: "true"},
		"output_dir":    {Type: AttrTypeString, Value: "/tmp/out"},
	}
	if err := ValidateAttributes("mineru", valid); err != nil {
		t.Fatalf("unexpected error for valid attrs: %v", err)
	}
}

func TestValidateAttributes_NoRulesPass(t *testing.T) {
	// Protocols without rules always pass (no required attributes).
	for _, p := range []string{"anthropic", "openai", "unknown"} {
		if err := ValidateAttributes(p, nil); err != nil {
			t.Fatalf("protocol %s should pass with no rules: %v", p, err)
		}
	}
}
