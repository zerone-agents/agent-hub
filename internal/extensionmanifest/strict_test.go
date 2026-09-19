// H7 扩展 manifest 严格校验器的单测：白名单拒绝、版本范围、
// 内容哈希稳定性（键序无关）与合法样例。
package extensionmanifest

import (
	"strings"
	"testing"
)

const validExtensionManifest = `{
  "apiVersion": "agenthub.extension/v1alpha1",
  "name": "io.zerone.example",
  "version": "1.2.3",
  "displayName": "示例扩展",
  "description": "严格校验测试用合法扩展",
  "icon": "https://example.com/icon.png",
  "publisher": "zerone",
  "dependencies": [
    {"name": "io.zerone.base", "version": ">=1.0.0 <2.0.0"}
  ],
  "permissions": [
    {"permission": "state", "scope": "io.zerone.example/*", "actions": ["read", "write"]},
    {"permission": "ui", "scope": "io.zerone.example/panel", "actions": ["render"]}
  ],
  "ui": {"slots": ["sidebar", "settings.section"]},
  "stateSchemas": [{"name": "profile", "description": "示例状态", "payload": {"type": "object"}}],
  "events": [{"name": "profile.changed"}],
  "tools": [{"name": "greet"}],
  "relations": [{"name": "acquaintance"}],
  "promptInjections": [{"name": "system-preamble"}]
}`

func TestValidateExtensionManifestValid(t *testing.T) {
	m, errs := ValidateExtensionManifest([]byte(validExtensionManifest))
	if len(errs) > 0 {
		t.Fatalf("合法 manifest 被拒：%v", errs)
	}
	if m.Name != "io.zerone.example" || m.Version != "1.2.3" {
		t.Fatalf("解析结果不正确：%+v", m)
	}
	if len(m.Permissions) != 2 || m.UI == nil || len(m.UI.Slots) != 2 {
		t.Fatalf("声明解析缺失：%+v", m)
	}
}

func TestValidateExtensionManifestAPIVersion(t *testing.T) {
	_, errs := ValidateExtensionManifest([]byte(`{"apiVersion":"v1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y"}`))
	if len(errs) == 0 || !strings.Contains(errs[0], "apiVersion") {
		t.Fatalf("期望 apiVersion 中文错误，得到 %v", errs)
	}
}

func TestValidateExtensionManifestNameAndVersion(t *testing.T) {
	_, errs := ValidateExtensionManifest([]byte(`{"apiVersion":"agenthub.extension/v1alpha1","name":"Not_DNS","version":"1.0","displayName":"x","description":"y"}`))
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "DNS") || !strings.Contains(joined, "语义化版本") {
		t.Fatalf("期望命名与版本错误，得到 %v", errs)
	}
}

func TestValidateExtensionManifestPermissionWhitelist(t *testing.T) {
	raw := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y",
	  "permissions":[{"permission":"emotion","scope":"s","actions":["read"]}]}`
	_, errs := ValidateExtensionManifest([]byte(raw))
	if len(errs) == 0 || !strings.Contains(errs[0], "白名单") {
		t.Fatalf("期望权限白名单拒绝，得到 %v", errs)
	}
}

func TestValidateExtensionManifestPermissionShape(t *testing.T) {
	raw := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y",
	  "permissions":[{"permission":"state","scope":"","actions":[]}]}`
	_, errs := ValidateExtensionManifest([]byte(raw))
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "scope") || !strings.Contains(joined, "actions") {
		t.Fatalf("期望 scope/actions 错误，得到 %v", errs)
	}
}

func TestValidateExtensionManifestSlotWhitelist(t *testing.T) {
	raw := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y",
	  "ui":{"slots":["sidebar","mind.reader","sidebar"]}}`
	_, errs := ValidateExtensionManifest([]byte(raw))
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "不是合法插槽") || !strings.Contains(joined, "重复插槽") {
		t.Fatalf("期望插槽白名单/去重拒绝，得到 %v", errs)
	}
}

func TestValidateExtensionManifestDeclarations(t *testing.T) {
	raw := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y",
	  "stateSchemas":[{"name":"bad name"},{"name":"dup"},{"name":"dup"}]}`
	_, errs := ValidateExtensionManifest([]byte(raw))
	joined := strings.Join(errs, "；")
	if !strings.Contains(joined, "命名规范") || !strings.Contains(joined, "重复声明") {
		t.Fatalf("期望声明结构错误，得到 %v", errs)
	}
}

func TestValidateExtensionManifestDependencyRange(t *testing.T) {
	base := `"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y"`
	for _, rng := range []string{">=1.0.0 <2.0.0", "^1.2.3", "~1.2.0", "1.0.0", "=1.0.0,>=1.0.0"} {
		raw := `{` + base + `,"dependencies":[{"name":"io.zerone.base","version":` + strconvQuote(rng) + `}]}`
		if _, errs := ValidateExtensionManifest([]byte(raw)); len(errs) > 0 {
			t.Errorf("版本范围 %q 应当合法：%v", rng, errs)
		}
	}
	raw := `{` + base + `,"dependencies":[{"name":"io.zerone.base","version":"latest"}]}`
	if _, errs := ValidateExtensionManifest([]byte(raw)); len(errs) == 0 {
		t.Error("非法版本范围 latest 应当被拒")
	}
}

func TestCanonicalManifestHashOrderIndependent(t *testing.T) {
	a := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example","version":"1.0.0","displayName":"x","description":"y","permissions":[{"permission":"state","scope":"s","actions":["read"]}]}`
	b := `{"description":"y","displayName":"x","version":"1.0.0","name":"io.zerone.example","apiVersion":"agenthub.extension/v1alpha1","permissions":[{"actions":["read"],"scope":"s","permission":"state"}]}`
	ha, err := CanonicalManifestHash([]byte(a))
	if err != nil {
		t.Fatal(err)
	}
	hb, err := CanonicalManifestHash([]byte(b))
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("键序不同的相同 manifest 哈希应一致：%s vs %s", ha, hb)
	}
	if ha == "" || len(ha) != 64 {
		t.Fatalf("哈希格式不正确：%q", ha)
	}
}

func TestIsValidVersionRange(t *testing.T) {
	for _, rng := range []string{"", "  ", "=>1.0.0", "1.0", ">=a.b.c"} {
		if IsValidVersionRange(rng) {
			t.Errorf("版本范围 %q 应当非法", rng)
		}
	}
}

func strconvQuote(s string) string {
	return `"` + s + `"`
}
