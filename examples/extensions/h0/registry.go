// Package h0 contains demonstration extension manifests used by the H0
// verification screen. They are examples, not platform capabilities.
package h0

import _ "embed"

type Example struct {
	ID          string
	Name        string
	Kind        string
	Description string
	Manifest    string
}

//go:embed speeding.yaml
var speedingManifest string

//go:embed research-team.yaml
var researchManifest string

func Examples() []Example {
	return []Example{
		{ID: "speeding", Name: "Speeding 财富与权力模拟", Kind: "vertical-application", Description: "垂直互动游戏能力包示例，不属于 Agent Hub 内置功能。", Manifest: speedingManifest},
		{ID: "research-team", Name: "通用研究协作", Kind: "general-purpose", Description: "非游戏场景，证明扩展协议不依赖特定垂直应用。", Manifest: researchManifest},
	}
}
