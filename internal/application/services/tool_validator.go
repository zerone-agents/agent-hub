package services

// ValidateToolName enforces the common identifier rules on tool names.
func ValidateToolName(name string) error {
	return validateIdentifier("Tool", name)
}
