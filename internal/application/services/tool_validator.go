package services

import "fmt"

// ValidateToolName enforces the common identifier rules on tool names and,
// in addition, mirrors the agent-deployer artifact-name contract
// (validateArtifactName in its internal/model/model.go): the names "." and
// ".." are rejected because they would corrupt the content-addressed OSS key
// (tools/<tenant>/<name-segment>) and every path derived from it.
func ValidateToolName(name string) error {
	if name == "." || name == ".." {
		return fmt.Errorf("Tool 标识不能为 \".\" 或 \"..\"")
	}
	return validateIdentifier("Tool", name)
}
