// 语义化版本（semver 2.0.0 子集）解析与比较，供扩展版本排序、
// 依赖范围校验与注册中心展示使用。仅覆盖扩展系统需要的核心规则：
// MAJOR.MINOR.PATCH + 可选 prerelease / build 后缀，不含完整 semver 规范中的
// 宽松边角（如超大整数溢出按数学比较降级处理）。
package extension

import (
	"fmt"
	"strconv"
	"strings"
)

// Version 是解析后的语义化版本。
type SemVersion struct {
	Major      int64
	Minor      int64
	Patch      int64
	Prerelease string // 不含 build 元数据；正式版为空串
	Build      string
}

// ParseVersion 解析 semver 字符串；不合法时返回中文错误。
func ParseVersion(raw string) (SemVersion, error) {
	var v SemVersion
	s := strings.TrimSpace(raw)
	if s == "" {
		return v, fmt.Errorf("版本号不能为空")
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		v.Build = s[i+1:]
		s = s[:i]
		if !validVersionIdent(v.Build, false) {
			return v, fmt.Errorf("版本号 %q 的构建元数据不合法", raw)
		}
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.Prerelease = s[i+1:]
		s = s[:i]
		if !validVersionIdent(v.Prerelease, true) {
			return v, fmt.Errorf("版本号 %q 的预发布段不合法", raw)
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("版本号 %q 必须是 MAJOR.MINOR.PATCH 三段式", raw)
	}
	nums := make([]int64, 3)
	for i, part := range parts {
		n, err := parseNumericIdent(part)
		if err != nil {
			return v, fmt.Errorf("版本号 %q 的第 %d 段 %q 不合法：%v", raw, i+1, part, err)
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, nil
}

// IsValidVersion 报告 raw 是否是合法 semver。
func IsValidVersion(raw string) bool {
	_, err := ParseVersion(raw)
	return err == nil
}

func parseNumericIdent(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("不能为空")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("不允许前导零")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("必须是数字")
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("超出整数范围")
	}
	return n, nil
}

// validVersionIdent 校验 prerelease/build 的点分段标识符。
func validVersionIdent(s string, prerelease bool) bool {
	if s == "" {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if part == "" {
			return false
		}
		numeric := true
		for _, r := range part {
			switch {
			case r >= '0' && r <= '9':
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
				numeric = false
			default:
				return false
			}
		}
		// prerelease 中纯数字段不允许前导零
		if prerelease && numeric && len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	return true
}

// CompareVersions 按 semver 规则比较 a 与 b：
// 返回 -1（a<b）、0（相等）、+1（a>b）。正式版高于同核心的任何预发布版。
func CompareVersions(a, b string) (int, error) {
	va, err := ParseVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := ParseVersion(b)
	if err != nil {
		return 0, err
	}
	return va.Compare(vb), nil
}

// Compare 比较两个已解析版本。
func (a SemVersion) Compare(b SemVersion) int {
	if c := compareInt(a.Major, b.Major); c != 0 {
		return c
	}
	if c := compareInt(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := compareInt(a.Patch, b.Patch); c != 0 {
		return c
	}
	return comparePrerelease(a.Prerelease, b.Prerelease)
}

func compareInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// comparePrerelease：正式版（空）> 预发布版；预发布段按 semver 规则逐段比较。
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		an, aErr := strconv.ParseInt(as[i], 10, 64)
		bn, bErr := strconv.ParseInt(bs[i], 10, 64)
		switch {
		case aErr == nil && bErr == nil:
			return compareInt(an, bn)
		case aErr == nil: // 数字段低于字母段
			return -1
		case bErr == nil:
			return 1
		default:
			return strings.Compare(as[i], bs[i])
		}
	}
	return compareInt(int64(len(as)), int64(len(bs)))
}
