// internal/application/services/version_test.go
package services

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.5.0", "2.4.0", 1},
		{"2.4.0", "2.5.0", -1},
		{"2.5.0", "2.5.0", 0},
		{"2.5", "2.5.0", 0}, // 缺段按 0
		{"3.0.0", "2.9.9", 1},
		{"2.10.0", "2.9.0", 1}, // 数值比较非字典序
		{"0.0.1", "0.0.2", -1},
		{"", "0.0.1", -1},
	}
	for _, tc := range cases {
		if got := compareSemver(tc.a, tc.b); got != tc.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
