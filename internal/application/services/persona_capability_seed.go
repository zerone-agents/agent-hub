package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"control-panel/internal/domain/capability"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BuiltinPersonaCapabilityTenant is the tenant used by the built-in H6
// capability packages. Runtime tenants may bind these immutable packages via
// the normal Run capability snapshot mechanism.
const BuiltinPersonaCapabilityTenant = "default"

type builtinPersonaCapability struct {
	Pack, Namespace, Name, DisplayName string
}

var builtinPersonaCapabilities = []builtinPersonaCapability{
	{PersonaPackEmotion, "io.zerone.emotion", "emotion-state", "情绪状态"},
	{PersonaPackBelief, "io.zerone.belief", "belief-state", "认知立场"},
	{PersonaPackMemory, "io.zerone.subjective-memory", "subjective-memory", "主观记忆"},
	{PersonaPackRelationshipDynamics, "io.zerone.relationship-dynamics", "relationship-dynamics", "动态关系"},
}

func personaCapabilityNamespace(pack string) (string, bool) {
	for _, item := range builtinPersonaCapabilities {
		if item.Pack == pack {
			return item.Namespace, true
		}
	}
	return "", false
}

func builtinPersonaCapabilityManifest(item builtinPersonaCapability) string {
	return fmt.Sprintf(`apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata:
  name: %s
  namespace: %s
  version: 1.0.0
  displayName: %s
  description: Built-in run-scoped persona capability
  license: Apache-2.0
  publisher: zerone
compatibility:
  hub: ">=0.9.0 <2.0.0"
  runtimeProtocol: ">=1.0.0 <2.0.0"
dependencies: []
permissions:
  state:
    read: ["%s/*"]
    write: ["%s/*"]
  events:
    consume: []
    emit: []
  tools:
    expose: []
  network:
    outbound: []
contributes:
  stateSchemas: []
`, item.Name, item.Namespace, item.DisplayName, item.Namespace, item.Namespace)
}

// EnsureBuiltinPersonaCapabilities exposes the four H6 packs in the regular
// capability registry. The operation is idempotent and never overwrites an
// administrator-managed row with the same immutable identity.
func (s *CapabilityRegistryService) EnsureBuiltinPersonaCapabilities() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range builtinPersonaCapabilities {
			manifest := builtinPersonaCapabilityManifest(item)
			hash := sha256.Sum256([]byte(manifest))
			now := time.Now().UTC()
			row := capability.Package{
				TenantID: BuiltinPersonaCapabilityTenant, Namespace: item.Namespace,
				Name: item.Name, Version: builtinPersonaPackExtensionVersion,
				DisplayName: item.DisplayName, ManifestYAML: manifest,
				ResourcesSnapshot: map[string]any{"personaPack": item.Pack},
				ContentHash:       hex.EncodeToString(hash[:]), SourceType: "seed",
				SourceRef: "builtin:h6/" + item.Pack, RequestedPermissions: map[string]any{},
				ApprovedPermissions: map[string]any{}, ApprovalStatus: "approved",
				ApprovedBy: "system", ApprovedAt: &now, Enabled: true,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return fmt.Errorf("seed persona capability %s: %w", item.Pack, err)
			}
		}
		return nil
	})
}
