package agent

import (
	"encoding/json"
	"testing"
)

func TestTool_DescriptionEnField(t *testing.T) {
	tool := Tool{Name: "calc", Description: "计算", DescriptionEn: "Calculator"}
	if tool.DescriptionEn != "Calculator" {
		t.Fatalf("DescriptionEn = %q, want %q", tool.DescriptionEn, "Calculator")
	}
	// JSON 序列化使用 camelCase descriptionEn（对齐仓库惯例与 skill/model.go）
	b, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	v, ok := got["descriptionEn"]
	if !ok || v != "Calculator" {
		t.Fatalf("json key descriptionEn = %#v, want %q", v, "Calculator")
	}
}
