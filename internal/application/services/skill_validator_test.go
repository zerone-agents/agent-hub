package services

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// buildZip is a test helper that builds an in-memory .zip from a map of
// path → content. Deterministic order isn't required for these tests; the
// validator must accept entries in any order.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			t.Fatalf("create %q: %v", path, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// validFrontmatter is a minimal SKILL.md that satisfies every rule of
// ValidateSkillFrontmatter — used as the baseline for cases that exercise
// other code paths.
const validFrontmatter = "---\nname: my-skill\ndescription: A test skill\n---\n# body\n"

// TestValidateSkillZip is the table-driven RED test for the server-side skill
// package validator — "CLI 端预检 + 服务端再校验（双保险）".
func TestValidateSkillZip(t *testing.T) {
	const skill = "my-skill"

	cases := []struct {
		name      string
		files     map[string]string
		raw       []byte // when set, overrides files (used for "not a zip")
		skillName string // defaults to "my-skill" if empty
		wantErr   string // substring expected in error; empty means expect success
	}{
		// ── Happy paths ──────────────────────────────────────────────
		{
			name: "valid skill nested under top dir",
			files: map[string]string{
				"my-skill/SKILL.md":           validFrontmatter,
				"my-skill/assets/diagram.png": "PNG-bytes",
			},
			wantErr: "",
		},
		{
			name: "valid skill with siblings under top dir",
			files: map[string]string{
				"my-skill/SKILL.md":       validFrontmatter,
				"my-skill/scripts/run.sh": "echo hi",
			},
			wantErr: "",
		},
		{
			name: "CRLF line endings tolerated",
			files: map[string]string{
				"my-skill/SKILL.md": "---\r\nname: my-skill\r\ndescription: x\r\n---\r\n# body\r\n",
			},
			wantErr: "",
		},
		{
			name: "name and description typed as non-string still accepted if non-empty",
			files: map[string]string{
				"my-skill/SKILL.md": "---\nname: 123\ndescription: 456\n---\n",
			},
			wantErr: "",
		},
		// Flat layout: SKILL.md at the archive root with siblings in
		// subdirectories. This is how packages built outside the CLI look
		// (e.g. the pharmaceutical-care-pathway upload from the admin UI).
		{
			name: "flat layout with SKILL.md at root accepted",
			files: map[string]string{
				"SKILL.md":            validFrontmatter,
				"assets/diagram.png":  "PNG-bytes",
				"references/notes.md": "notes",
				"scripts/run.sh":      "echo hi",
			},
			wantErr: "",
		},
		// The top-level directory name is NOT required to match the skill
		// name. Any directory layout is fine as long as SKILL.md is present.
		{
			name: "top-level directory name need not match skill name",
			files: map[string]string{
				"other-name/SKILL.md": validFrontmatter,
			},
			wantErr: "",
		},

		// ── Zip-level failures ───────────────────────────────────────
		{
			name:    "not a zip",
			raw:     []byte("this is plainly not a zip stream"),
			wantErr: "zip",
		},
		{
			name:    "empty zip has no SKILL.md",
			files:   map[string]string{},
			wantErr: "SKILL.md",
		},

		// ── Path safety ──────────────────────────────────────────────
		{
			name: "parent-dir traversal rejected",
			files: map[string]string{
				"my-skill/SKILL.md": validFrontmatter,
				"my-skill/../evil":  "owned",
			},
			wantErr: "../evil",
		},
		{
			name: "mid-path parent traversal rejected",
			files: map[string]string{
				"my-skill/SKILL.md":            validFrontmatter,
				"my-skill/good/../../evil.txt": "owned",
			},
			wantErr: "../../evil.txt",
		},
		{
			name: "unix absolute path rejected",
			files: map[string]string{
				"my-skill/SKILL.md": validFrontmatter,
				"/etc/passwd":       "owned",
			},
			wantErr: "/etc/passwd",
		},
		{
			name: "windows drive-letter path rejected",
			files: map[string]string{
				"my-skill/SKILL.md": validFrontmatter,
				"C:/Windows/evil":   "owned",
			},
			wantErr: "C:/Windows/evil",
		},
		{
			name: "windows backslash traversal rejected",
			files: map[string]string{
				"my-skill/SKILL.md":     validFrontmatter,
				"my-skill/..\\evil.txt": "owned",
			},
			wantErr: "..\\evil.txt",
		},

		// ── SKILL.md structural rules ────────────────────────────────
		{
			name:    "missing SKILL.md entirely",
			files:   map[string]string{"my-skill/README.md": "no skill here"},
			wantErr: "SKILL.md",
		},
		{
			name: "SKILL.md is a directory not a file is treated as missing",
			files: map[string]string{
				"my-skill/SKILL.md/": "",
				"my-skill/README.md": "x",
			},
			wantErr: "SKILL.md",
		},
		{
			name:    "SKILL.md without frontmatter",
			files:   map[string]string{"my-skill/SKILL.md": "# just a markdown body, no frontmatter"},
			wantErr: "frontmatter",
		},
		{
			name:    "frontmatter not closed",
			files:   map[string]string{"my-skill/SKILL.md": "---\nname: x\nbody keeps going"},
			wantErr: "frontmatter",
		},
		{
			name:    "frontmatter missing name",
			files:   map[string]string{"my-skill/SKILL.md": "---\ndescription: x\n---\nbody"},
			wantErr: "name",
		},
		{
			name:    "frontmatter missing description",
			files:   map[string]string{"my-skill/SKILL.md": "---\nname: x\n---\nbody"},
			wantErr: "description",
		},
		{
			name:    "frontmatter name explicitly empty",
			files:   map[string]string{"my-skill/SKILL.md": "---\nname: \"\"\ndescription: x\n---\nbody"},
			wantErr: "name",
		},
		{
			name:    "frontmatter description explicitly empty",
			files:   map[string]string{"my-skill/SKILL.md": "---\nname: x\ndescription: \"\"\n---\nbody"},
			wantErr: "description",
		},
		{
			name:    "frontmatter is not valid YAML",
			files:   map[string]string{"my-skill/SKILL.md": "---\nname: [unclosed\n---\nbody"},
			wantErr: "YAML",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.raw
			if raw == nil {
				raw = buildZip(t, tc.files)
			}
			got, err := ValidateSkillZip(bytes.NewReader(raw))

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ValidateSkillZip expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ValidateSkillZip error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ValidateSkillZip unexpected error: %v", err)
			}
			if !bytes.Equal(got, raw) {
				t.Errorf("returned bytes differ from input (in=%d, out=%d)", len(raw), len(got))
			}
		})
	}
}

// TestValidateSkillZip_ReturnedBytesAreValidZip guards against a sneaky
// implementation: returning a sub-slice (e.g. stripping the central
// directory) that the caller would then upload, only to fail again at the
// deployer. The returned bytes must round-trip through archive/zip.
func TestValidateSkillZip_ReturnedBytesAreValidZip(t *testing.T) {
	raw := buildZip(t, map[string]string{
		"my-skill/SKILL.md":       validFrontmatter,
		"my-skill/scripts/run.sh": "echo hi",
	})

	out, err := ValidateSkillZip(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(out, raw) {
		t.Fatalf("returned bytes are not the same as input")
	}

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("returned bytes aren't a valid zip: %v", err)
	}
	if len(zr.File) != 2 {
		t.Errorf("expected 2 entries in returned zip, got %d", len(zr.File))
	}
}

// TestValidateSkillZip_ConsumesAllBytes verifies the validator reads the
// stream to completion, so the caller's subsequent OSS upload from the
// returned []byte is complete and not a truncated prefix.
func TestValidateSkillZip_ConsumesAllBytes(t *testing.T) {
	raw := buildZip(t, map[string]string{
		"my-skill/SKILL.md": validFrontmatter,
		"my-skill/big.bin":  strings.Repeat("x", 4096),
	})

	out, err := ValidateSkillZip(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != len(raw) {
		t.Errorf("byte count mismatch: in=%d out=%d (stream not fully consumed)", len(raw), len(out))
	}
}

// TestFindAllSkillMd covers the multi-SKILL.md locator that backs bundle
// support. Each case exercises a distinct tree shape and verifies both
// the returned entries (path + content) and the sorting contract.
func TestFindAllSkillMd(t *testing.T) {
	t.Run("returns single entry for flat layout", func(t *testing.T) {
		zipBytes := buildZip(t, map[string]string{
			"SKILL.md":  validFrontmatter,
			"README.md": "x",
		})
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatal(err)
		}
		entries, err := FindAllSkillMd(zr)
		if err != nil {
			t.Fatalf("FindAllSkillMd: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d entries, want 1", len(entries))
		}
		if entries[0].Path != "SKILL.md" {
			t.Errorf("Path = %q, want %q", entries[0].Path, "SKILL.md")
		}
		if entries[0].Content != validFrontmatter {
			t.Errorf("Content mismatch")
		}
	})

	t.Run("returns multiple entries sorted by path", func(t *testing.T) {
		zipBytes := buildZip(t, map[string]string{
			"team/sub/deploy/SKILL.md": "---\nname: deploy\ndescription: d\n---\n",
			"commit/SKILL.md":          "---\nname: commit\ndescription: c\n---\n",
			"team/review/SKILL.md":     "---\nname: review\ndescription: r\n---\n",
		})
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatal(err)
		}
		entries, err := FindAllSkillMd(zr)
		if err != nil {
			t.Fatalf("FindAllSkillMd: %v", err)
		}
		wantPaths := []string{
			"commit/SKILL.md",
			"team/review/SKILL.md",
			"team/sub/deploy/SKILL.md",
		}
		if len(entries) != len(wantPaths) {
			t.Fatalf("got %d entries, want %d", len(entries), len(wantPaths))
		}
		for i, want := range wantPaths {
			if entries[i].Path != want {
				t.Errorf("entries[%d].Path = %q, want %q", i, entries[i].Path, want)
			}
		}
	})

	t.Run("excludes SKILL.md under .git / node_modules / dist / build / .zerone-uploads", func(t *testing.T) {
		zipBytes := buildZip(t, map[string]string{
			".git/SKILL.md":                    "---\nname: ghost\ndescription: g\n---\n",
			"node_modules/pkg/SKILL.md":        "---\nname: dep\ndescription: d\n---\n",
			"dist/SKILL.md":                    "---\nname: built\ndescription: b\n---\n",
			"build/artifacts/SKILL.md":         "---\nname: art\ndescription: a\n---\n",
			".zerone-uploads/scratch/SKILL.md": "---\nname: scratch\ndescription: s\n---\n",
			"real/SKILL.md":                    "---\nname: real\ndescription: r\n---\n",
		})
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatal(err)
		}
		entries, err := FindAllSkillMd(zr)
		if err != nil {
			t.Fatalf("FindAllSkillMd: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d entries, want 1 (only real/SKILL.md); got: %v",
				len(entries), entries)
		}
		if entries[0].Path != "real/SKILL.md" {
			t.Errorf("Path = %q, want real/SKILL.md", entries[0].Path)
		}
	})

	t.Run("returns empty slice when no SKILL.md anywhere", func(t *testing.T) {
		zipBytes := buildZip(t, map[string]string{
			"a/README.md": "no skill here",
			"b/c/x.txt":   "nope",
		})
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatal(err)
		}
		entries, err := FindAllSkillMd(zr)
		if err != nil {
			t.Fatalf("FindAllSkillMd: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("got %d entries, want 0", len(entries))
		}
	})

	// Note: Windows-style backslash path normalisation in FindAllSkillMd
	// (src/zip.go uses strings.ReplaceAll(f.Name, "\\", "/")) is not
	// exercised here because Go's archive/zip writer itself normalises
	// backslashes to forward slashes when storing entries. The defensive
	// normalisation targets zips produced by Windows-native tooling
	// (e.g. Windows Explorer, some 7-Zip builds) which Go's writer
	// faithfully reproduces as forward slashes regardless.
}

// TestValidateSkillZip_Bundle covers end-to-end validation of multi-SKILL.md
// zips (the bundle upload case). Builds on TestValidateSkillZip's per-file
// frontmatter rules — these tests focus on the multi-file aggregation
// behaviour and path-prefixed error messages.
func TestValidateSkillZip_Bundle(t *testing.T) {
	t.Run("bundle of valid SKILL.md files at various depths passes", func(t *testing.T) {
		raw := buildZip(t, map[string]string{
			"commit/SKILL.md":          "---\nname: commit\ndescription: c\n---\n",
			"team/review/SKILL.md":     "---\nname: review\ndescription: r\n---\n",
			"team/sub/deploy/SKILL.md": "---\nname: deploy\ndescription: d\n---\n",
		})
		if _, err := ValidateSkillZip(bytes.NewReader(raw)); err != nil {
			t.Fatalf("ValidateSkillZip bundle (all valid) failed: %v", err)
		}
	})

	t.Run("bundle with one broken SKILL.md fails naming that file's path", func(t *testing.T) {
		raw := buildZip(t, map[string]string{
			"commit/SKILL.md":      "---\nname: commit\ndescription: c\n---\n",
			"team/review/SKILL.md": "---\nname: review\n---\n# missing description",
		})
		_, err := ValidateSkillZip(bytes.NewReader(raw))
		if err == nil {
			t.Fatal("expected error for broken bundle, got nil")
		}
		// Error must name the offending file path so the user can fix it.
		if !strings.Contains(err.Error(), "team/review/SKILL.md") {
			t.Errorf("error %q should contain 'team/review/SKILL.md'", err.Error())
		}
		// And must mention the missing field.
		if !strings.Contains(err.Error(), "description") {
			t.Errorf("error %q should mention missing 'description'", err.Error())
		}
		// The valid file should NOT be named in the error.
		if strings.Contains(err.Error(), "commit/SKILL.md") {
			t.Errorf("error %q should not mention the valid commit/SKILL.md", err.Error())
		}
	})

	t.Run("bundle with multiple broken SKILL.md fails on the first one (sorted order)", func(t *testing.T) {
		// Validation short-circuits on the first failure (sorted by path),
		// so only "a/SKILL.md" is named even though "b/SKILL.md" is also
		// broken. This is intentional — surfacing one clear error is more
		// actionable than a wall of text, and the user will see the next
		// one on retry.
		raw := buildZip(t, map[string]string{
			"a/SKILL.md": "---\ndescription: missing name\n---\n",
			"b/SKILL.md": "---\nname: b\n---\n# missing description",
		})
		_, err := ValidateSkillZip(bytes.NewReader(raw))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "a/SKILL.md") {
			t.Errorf("error %q should mention a/SKILL.md (sorted first)", err.Error())
		}
		if strings.Contains(err.Error(), "b/SKILL.md") {
			t.Errorf("error %q should NOT mention b/SKILL.md (short-circuit on first)", err.Error())
		}
	})

	t.Run("bundle where every SKILL.md lives under excluded dirs is treated as empty", func(t *testing.T) {
		raw := buildZip(t, map[string]string{
			".git/SKILL.md":           "---\nname: g\ndescription: g\n---\n",
			"node_modules/x/SKILL.md": "---\nname: x\ndescription: x\n---\n",
			"README.md":               "no real skill",
		})
		_, err := ValidateSkillZip(bytes.NewReader(raw))
		if err == nil {
			t.Fatal("expected error for empty-after-exclusion bundle, got nil")
		}
		if !strings.Contains(err.Error(), "SKILL.md") {
			t.Errorf("error %q should mention missing SKILL.md", err.Error())
		}
	})
}
