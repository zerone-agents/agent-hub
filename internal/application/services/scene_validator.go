package services

import (
	"fmt"
	"strings"
)

func ValidateSceneName(name string) error {
	return validateIdentifier("场景", name)
}

func ValidateSceneTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("场景名称不能为空")
	}
	if len(title) > 128 {
		return fmt.Errorf("场景名称长度不能超过 128 个字符")
	}
	return nil
}

func ValidateScenePrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("提示词不能为空")
	}
	return nil
}
