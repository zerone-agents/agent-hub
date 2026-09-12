package extensionmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePackage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		dir   string
		valid bool
		want  []string
	}{
		{name: "valid", dir: "valid", valid: true},
		{name: "invalid fields and traversal", dir: "invalid", want: []string{"metadata/name", "metadata/namespace", "metadata/version", "contributes/stateSchemas/0/version", "contributes/stateSchemas/0/file"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report := ValidatePackage(filepath.Join("testdata", tc.dir))
			if report.Valid != tc.valid {
				t.Fatalf("Valid = %v, errors = %v", report.Valid, report.Errors)
			}
			joined := strings.Join(report.Errors, "\n")
			for _, want := range tc.want {
				if !strings.Contains(joined, want) {
					t.Errorf("errors %q do not contain %q", joined, want)
				}
			}
		})
	}
}

func TestValidatePackageRejectsMissingReferencedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifest := `apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata: {name: test-package, namespace: io.zerone.test, version: 1.0.0, displayName: Test, description: Test package, publisher: zerone}
compatibility: {hub: "*", runtimeProtocol: "*"}
permissions:
  state: {read: [], write: []}
  events: {consume: [], emit: []}
  tools: {expose: []}
  network: {outbound: []}
contributes:
  tools: [{file: tools/missing.yaml}]
`
	if err := os.WriteFile(filepath.Join(dir, "extension.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	report := ValidatePackage(dir)
	if report.Valid || !strings.Contains(strings.Join(report.Errors, "\n"), "tools/missing.yaml") {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidatePackageRejectsReferencedSymlinkOutsidePackage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "tool.yaml")
	if err := os.WriteFile(externalFile, []byte("id: external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "tools"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalFile, filepath.Join(dir, "tools", "external.yaml")); err != nil {
		t.Fatal(err)
	}
	manifest := `apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata: {name: test-package, namespace: io.zerone.test, version: 1.0.0, displayName: Test, description: Test package, publisher: zerone}
compatibility: {hub: "*", runtimeProtocol: "*"}
permissions:
  state: {read: [], write: []}
  events: {consume: [], emit: []}
  tools: {expose: []}
  network: {outbound: []}
contributes:
  tools: [{file: tools/external.yaml}]
`
	if err := os.WriteFile(filepath.Join(dir, "extension.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	report := ValidatePackage(dir)
	if report.Valid || !strings.Contains(strings.Join(report.Errors, "\n"), "resolves outside the package") {
		t.Fatalf("unexpected report: %+v", report)
	}
}
