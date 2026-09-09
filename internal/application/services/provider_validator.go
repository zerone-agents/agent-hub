package services

import "control-panel/internal/domain/provider"

// ValidateProviderKey enforces the common identifier rules on provider keys.
// The shared validateIdentifier returns plain fmt.Errorf; wrap it into the
// domain ValidationError so the provider HTTP boundary can render it as a
// user-facing 400 (vs. internal diagnostics → 500 neutral, issue #95 P2).
func ValidateProviderKey(key string) error {
	if err := validateIdentifier("Provider key", key); err != nil {
		return provider.NewValidationErrorf("%s", err.Error())
	}
	return nil
}
