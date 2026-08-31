package services

import (
	"fmt"

	"control-panel/internal/domain/agent"
)

// ValidateToolName enforces the common identifier rules on tool names and,
// in addition, mirrors the agent-deployer artifact-name contract
// (validateArtifactName in its internal/model/model.go): the names "." and
// ".." are rejected because they would corrupt the content-addressed OSS key
// (tools/<tenant>/<name-segment>) and every path derived from it.
// 所有拒绝路径都包装 agent.ErrInvalidToolName（expert review round 3），
// handler 用 errors.Is 映射 400；共享的 validateIdentifier 保持不动。
func ValidateToolName(name string) error {
	if name == "." || name == ".." {
		return fmt.Errorf("%w：不能为 \".\" 或 \"..\"", agent.ErrInvalidToolName)
	}
	if err := validateIdentifier("Tool", name); err != nil {
		return fmt.Errorf("%w：%v", agent.ErrInvalidToolName, err)
	}
	return nil
}
