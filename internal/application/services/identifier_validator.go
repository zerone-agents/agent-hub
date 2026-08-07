package services

import (
	"fmt"
	"regexp"
)

// validIdentifierPattern restricts unique identifiers across the system to a
// safe, URL/file-system-friendly character set: ASCII letters, digits, dot,
// underscore and hyphen. Apply this to every user-facing identifier (agent
// name, skill name, scene name, tool name, provider key, ...) so paths,
// container names and OSS keys derived from them stay predictable.
var validIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateIdentifier enforces the common rules shared by all unique
// identifiers: non-empty, ≤64 runes, and matching validIdentifierPattern.
// entity is the human-friendly label used in error messages (e.g. "Agent",
// "Provider key").
func validateIdentifier(entity, value string) error {
	if value == "" {
		return fmt.Errorf("%s 标识不能为空", entity)
	}
	if len(value) > 64 {
		return fmt.Errorf("%s 标识长度不能超过 64 个字符", entity)
	}
	if !validIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s 标识只能包含字母、数字、点、下划线和横线", entity)
	}
	return nil
}
