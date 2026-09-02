// internal/application/services/version.go
package services

import "strings"

// compareSemver compares dotted numeric version strings ("2.5.0"). Missing
// segments count as 0; segments parse leading digits only (pre-release
// suffixes are ignored, e.g. "0-beta.1" → 0). Returns -1 (a<b), 0, or 1.
func compareSemver(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = atoiSafe(as[i])
		}
		if i < len(bs) {
			bv = atoiSafe(bs[i])
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// atoiSafe parses leading digits; anything else (or empty) is 0.
func atoiSafe(s string) int {
	v := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			break
		}
		v = v*10 + int(r-'0')
	}
	return v
}