package services

// ValidateProviderKey enforces the common identifier rules on provider keys.
func ValidateProviderKey(key string) error {
	return validateIdentifier("Provider key", key)
}
