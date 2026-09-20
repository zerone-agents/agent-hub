package services

import (
	"testing"

	"control-panel/internal/domain/capability"
	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuiltinPersonaCapabilitiesSeedAndRunIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&capability.Package{}, &capability.ResourceProvenance{},
		&rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.CapabilityBinding{},
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
	))

	lifecycle := NewExtensionLifecycleService(db)
	require.NoError(t, lifecycle.EnsureBuiltinPersonaPacks())
	registry := NewCapabilityRegistryService(db)
	require.NoError(t, registry.EnsureBuiltinPersonaCapabilities())
	require.NoError(t, registry.EnsureBuiltinPersonaCapabilities(), "seed must be idempotent")

	packages, err := registry.List("default", nil)
	require.NoError(t, err)
	require.Len(t, packages, 4)
	for _, item := range packages {
		require.True(t, item.Enabled)
		require.Equal(t, "approved", item.ApprovalStatus)
		require.Equal(t, "seed", item.SourceType)
	}
	tenantPackages, err := registry.List("tenant-a", nil)
	require.NoError(t, err)
	require.Len(t, tenantPackages, 4, "platform seed packages must be selectable by every tenant")

	runs := NewRunService(db)
	bound, err := runs.Create("tenant-a", CreateRunInput{Name: "bound", Capabilities: []RunCapabilityInput{
		{Namespace: "io.zerone.emotion", PackageName: "emotion-state", Version: "1.0.0"},
		{Namespace: "io.zerone.subjective-memory", PackageName: "subjective-memory", Version: "1.0.0"},
	}})
	require.NoError(t, err)
	unbound, err := runs.Create("tenant-a", CreateRunInput{Name: "unbound"})
	require.NoError(t, err)

	gate := NewPersonaCapabilityGate(db)
	require.True(t, gate.EnabledForRun("tenant-a", bound.ID, PersonaPackEmotion))
	require.True(t, gate.EnabledForRun("tenant-a", bound.ID, PersonaPackMemory))
	require.False(t, gate.EnabledForRun("tenant-a", bound.ID, PersonaPackBelief))
	require.False(t, gate.EnabledForRun("tenant-a", unbound.ID, PersonaPackEmotion))
	require.False(t, gate.EnabledForRun("tenant-a", "", PersonaPackEmotion))
}
