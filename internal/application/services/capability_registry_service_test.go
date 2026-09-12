package services

import (
	"strings"
	"testing"

	"control-panel/internal/domain/capability"
	rundomain "control-panel/internal/domain/run"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testCapabilityManifest = `apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata:
  name: task-state
  namespace: io.example.research
  version: 1.0.0
  displayName: Task State
  description: Generic task state
  license: Apache-2.0
  publisher: example
compatibility:
  hub: ">=0.9.0 <2.0.0"
  runtimeProtocol: ">=1.0.0 <2.0.0"
dependencies: []
permissions:
  state:
    read: ["io.example.research/*"]
    write: ["io.example.research/*"]
  events:
    consume: []
    emit: []
  tools:
    expose: []
  network:
    outbound: []
contributes:
  stateSchemas: []
`

func TestCapabilityRegistryControlsRunBindingAndLocksSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capability.Package{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.CapabilityBinding{}))
	registry := NewCapabilityRegistryService(db)
	runs := NewRunService(db)
	p, err := registry.Register("tenant-a", RegisterCapabilityInput{ManifestYAML: testCapabilityManifest, ResourcesSnapshot: map[string]any{"promptFragments": []any{"original"}, "rules": map[string]any{"version": "v1"}}})
	require.NoError(t, err)
	require.False(t, p.Enabled)
	ref := []RunCapabilityInput{{Namespace: p.Namespace, PackageName: p.Name, Version: p.Version}}
	_, err = runs.Create("tenant-a", CreateRunInput{Name: "disabled", Capabilities: ref})
	require.Error(t, err)
	_, err = registry.SetEnabled("tenant-a", p.ID, true)
	require.NoError(t, err)
	_, err = runs.Create("tenant-b", CreateRunInput{Name: "wrong tenant", Capabilities: ref})
	require.Error(t, err)
	r, err := runs.Create("tenant-a", CreateRunInput{Name: "locked", Capabilities: ref})
	require.NoError(t, err)
	require.Len(t, r.Bindings, 1)
	lockedHash := r.Bindings[0].ContentHash
	lockedManifest := r.Bindings[0].ManifestYAML
	_, err = registry.SetEnabled("tenant-a", p.ID, false)
	require.NoError(t, err)
	v2, err := registry.Register("tenant-a", RegisterCapabilityInput{ManifestYAML: strings.Replace(testCapabilityManifest, "version: 1.0.0", "version: 2.0.0", 1), ResourcesSnapshot: map[string]any{"promptFragments": []any{"v2"}}})
	require.NoError(t, err)
	_, err = registry.SetEnabled("tenant-a", v2.ID, true)
	require.NoError(t, err)
	require.NoError(t, db.Model(&capability.Package{}).Where("id=?", p.ID).Updates(map[string]any{"manifest_yaml": "changed registry data", "content_hash": "changed"}).Error)
	r, err = runs.Get("tenant-a", r.ID)
	require.NoError(t, err)
	require.Equal(t, lockedHash, r.Bindings[0].ContentHash)
	require.Equal(t, lockedManifest, r.Bindings[0].ManifestYAML)
	require.Equal(t, "original", r.Bindings[0].Snapshot["promptFragments"].([]any)[0])
}

func TestCapabilityRegistryRejectsInvalidAndDuplicateManifest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capability.Package{}))
	s := NewCapabilityRegistryService(db)
	_, err = s.Register("t", RegisterCapabilityInput{ManifestYAML: "not: [valid"})
	require.Error(t, err)
	_, err = s.Register("t", RegisterCapabilityInput{ManifestYAML: testCapabilityManifest})
	require.NoError(t, err)
	_, err = s.Register("t", RegisterCapabilityInput{ManifestYAML: testCapabilityManifest})
	require.ErrorIs(t, err, rundomain.ErrDuplicate)
}
