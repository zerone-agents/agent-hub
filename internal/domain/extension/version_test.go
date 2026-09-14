// 语义化版本解析与比较的领域单测。
package extension

import "testing"

func TestParseVersionValid(t *testing.T) {
	for _, raw := range []string{"0.0.1", "1.2.3", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0+build.5", "1.0.0-rc.1+exp"} {
		if _, err := ParseVersion(raw); err != nil {
			t.Errorf("ParseVersion(%q) 应当合法：%v", raw, err)
		}
	}
}

func TestParseVersionInvalid(t *testing.T) {
	for _, raw := range []string{"", "1.2", "1.2.3.4", "01.2.3", "1.2.x", "1.2.3-", "1.2.3-alpha..1", "v1.2.3"} {
		if _, err := ParseVersion(raw); err == nil {
			t.Errorf("ParseVersion(%q) 应当报错", raw)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.1.0", "2.0.9", 1},
		{"1.0.0-alpha", "1.0.0", -1},              // 正式版高于预发布版
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},      // 段数少的预发布版更低
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1}, // 数字段低于字母段
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},     // 数字段按数值比较
		{"1.0.0+build1", "1.0.0+build2", 0},       // build 元数据不参与比较
	}
	for _, tc := range cases {
		got, err := CompareVersions(tc.a, tc.b)
		if err != nil {
			t.Fatalf("CompareVersions(%q,%q) 出错：%v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Errorf("CompareVersions(%q,%q) = %d，期望 %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCompareVersionsInvalid(t *testing.T) {
	if _, err := CompareVersions("abc", "1.0.0"); err == nil {
		t.Error("非法版本应当报错")
	}
}
