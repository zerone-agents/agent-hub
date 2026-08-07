package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func BuildOSSKey(skillType string, name string) string {
	return fmt.Sprintf("%s-skills/%s.zip", skillType, name)
}

func ValidateSkillType(skillType string) error {
	if skillType != "expert" && skillType != "community" {
		return fmt.Errorf("技能类型必须是 expert 或 community")
	}
	return nil
}

func ValidateSkillName(name string) error {
	return validateIdentifier("技能", name)
}

// ValidateSkillZip is the server-side half of the "double insurance" pattern:
// the CLI pre-checks the package before upload; this function re-checks it on
// the server so that:
//
//   - direct uploads via the admin UI (which bypass the CLI) can't land
//     malformed zips in OSS,
//   - hostile packages with path-traversal / absolute-path entries can never
//     reach the deployer's extraction step (which runs inside a runtime
//     container and writes to the user-level skill directory).
//
// Without this, a bad zip only fails when the deployer tries to extract it —
// by then OSS is already polluted and the error surfaces as a confusing
// "deploy agent failed" instead of a clear "your skill package is bad".
//
// It reads the stream to completion and returns the buffered bytes so the
// caller can hand them to OSS upload without re-reading the now-consumed
// multipart stream.
//
// The package may be structured in two ways, both accepted:
//   - flat: SKILL.md at the archive root (e.g. a package built outside the CLI),
//   - nested: everything under a top-level directory (e.g. `my-skill/SKILL.md`,
//     which is how the CLI's `packDir` builds packages).
//
// The validator does NOT enforce a particular top-level layout — it only
// requires that SKILL.md exists somewhere, all paths are safe, and the
// frontmatter is valid.
func ValidateSkillZip(r io.Reader) ([]byte, error) {
	// zip's central directory lives at the END of the file, so random access
	// is required — buffer the stream first.
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取 zip 内容失败: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("不是有效的 zip 文件: %w", err)
	}

	// Reject any entry that could escape the extraction directory or point
	// at absolute system locations. The deployer extracts as root inside the
	// runtime container, so a single "../" or "/etc/..." entry is a real
	// escape, not a theoretical concern.
	for _, f := range zr.File {
		if isUnsafeZipPath(f.Name) {
			return nil, fmt.Errorf("zip 包含不安全的路径: %s (禁止 ../ 或绝对路径)", f.Name)
		}
	}

	// Locate every SKILL.md anywhere in the tree (matches SDK glob
	// semantics). Multi-SKILL.md zips (bundles) are valid; each one must
	// still pass frontmatter validation.
	entries, err := FindAllSkillMd(zr)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("zip 包中缺少 SKILL.md 文件")
	}

	for _, entry := range entries {
		if err := validateSkillFrontmatter([]byte(entry.Content), entry.Path); err != nil {
			return nil, err
		}
	}

	return buf, nil
}

// SkillMdEntry pairs a SKILL.md's zip-internal path with its content.
type SkillMdEntry struct {
	Path    string
	Content string
}

// skillExcludedDirs are directory segments that should be skipped when
// scanning for SKILL.md. Mirrors the CLI's EXCLUDED set so client and
// server agree on what counts as a "real" skill file.
var skillExcludedDirs = map[string]bool{
	".git":            true,
	"node_modules":    true,
	"dist":            true,
	"build":           true,
	".zerone-uploads": true,
}

// FindAllSkillMd finds every SKILL.md anywhere in the zip tree, returning
// entries sorted by path. Matches the SDK's loadSkillsFromDir glob
// semantics (`**/SKILL.md`) so server-side validation catches exactly
// what the runtime will register. Directory entries named "SKILL.md"
// are skipped so a literal "SKILL.md/" can't masquerade as the file.
//
// Paths under skillExcludedDirs (e.g. ".git/SKILL.md",
// "team/node_modules/x/SKILL.md") are filtered out — those are not
// real skills and would never be reached by the deployer's packDir
// pipeline either.
func FindAllSkillMd(zr *zip.Reader) ([]SkillMdEntry, error) {
	var found []SkillMdEntry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Normalise backslashes for Windows-built zips before matching.
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if path.Base(name) != "SKILL.md" {
			continue
		}
		// Skip entries whose parent path passes through excluded dirs.
		if isUnderExcludedDir(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("打开 %s 失败: %w", name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("读取 %s 内容失败: %w", name, err)
		}
		found = append(found, SkillMdEntry{Path: name, Content: string(content)})
	}
	// Stable order: by path. Makes error messages deterministic and tests
	// assertion-friendly.
	sort.Slice(found, func(i, j int) bool {
		return found[i].Path < found[j].Path
	})
	return found, nil
}

// isUnderExcludedDir reports whether any segment of the parent path of
// `entry` (everything except the basename) matches a reserved dir name.
func isUnderExcludedDir(entry string) bool {
	segments := strings.Split(entry, "/")
	// Last segment is the filename ("SKILL.md"); check parents only.
	for _, seg := range segments[:len(segments)-1] {
		if skillExcludedDirs[seg] {
			return true
		}
	}
	return false
}

// isUnsafeZipPath rejects entries that could break out of the extraction
// directory. Covers Unix absolute paths, Windows drive-letter paths, and
// any ".." segment anywhere in the path. Backslashes are normalised to
// slashes first because some Windows zip tools emit them and a "..\evil"
// entry would otherwise slip past a slash-only check.
func isUnsafeZipPath(p string) bool {
	p = strings.ReplaceAll(p, "\\", "/")

	if strings.HasPrefix(p, "/") {
		return true
	}
	// Windows drive letter: "C:/...", "c:\..."
	if len(p) >= 2 && p[1] == ':' &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) {
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// validateSkillFrontmatter enforces the SKILL.md contract from spec §6:
// the file must start with a YAML frontmatter block (delimited by --- lines)
// containing non-empty `name` and `description`. Without `name` the runtime
// can't register the skill; without `description` the agent has nothing to
// match against user intent when deciding whether to load it.
//
// `ctx` is the SKILL.md's path inside the zip (e.g. "team/review/SKILL.md").
// It is included in error messages so bundle uploads with multiple
// SKILL.md files can identify which one failed validation.
func validateSkillFrontmatter(content []byte, ctx string) error {
	// Normalise CRLF so the "---" delimiter check is line-ending-agnostic.
	text := strings.ReplaceAll(string(content), "\r\n", "\n")

	if !strings.HasPrefix(text, "---\n") && text != "---" {
		return fmt.Errorf("%s: 缺少 frontmatter (必须以 --- 开头)", ctx)
	}

	body := strings.TrimPrefix(text, "---\n")
	// Find the closing "---" line. Can't just search for "\n---\n" because
	// the closing delimiter may sit at EOF without a trailing newline.
	lines := strings.Split(body, "\n")
	closeIdx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return fmt.Errorf("%s: frontmatter 未闭合 (缺少结束 ---)", ctx)
	}

	fmBytes := []byte(strings.Join(lines[:closeIdx], "\n"))

	var parsed map[string]interface{}
	if err := yaml.Unmarshal(fmBytes, &parsed); err != nil {
		return fmt.Errorf("%s: frontmatter 不是合法的 YAML: %w", ctx, err)
	}

	// yaml.v3 unmarshals unquoted scalars into their native Go types
	// (name: 123 → int), so accept any non-nil value and stringify. Empty
	// strings, empty collections, and nil all count as "missing".
	for _, key := range []string{"name", "description"} {
		v, ok := parsed[key]
		if !ok || v == nil {
			return fmt.Errorf("%s: frontmatter 缺少 %s 字段", ctx, key)
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return fmt.Errorf("%s: frontmatter %s 不能为空", ctx, key)
		}
	}
	return nil
}
