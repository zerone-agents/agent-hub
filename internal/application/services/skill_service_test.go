package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"control-panel/internal/domain/skill"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// mockUploader implements oss.OSSUploader for testing. It stores objects in an
// in-memory map keyed by OSS object key, so tests can stage a zip in the fake
// OSS bucket and have GetSkillMd download it.
type mockUploader struct {
	data        map[string][]byte
	downloadErr error
}

func (m *mockUploader) Upload(_ context.Context, key string, reader io.Reader, _ int64) (string, error) {
	buf, _ := io.ReadAll(reader)
	m.data[key] = buf
	return "fake-hash", nil
}

func (m *mockUploader) GetPresignedURL(_ context.Context, key string) (string, error) {
	return "https://example.com/" + key, nil
}

func (m *mockUploader) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockUploader) Download(_ context.Context, key string) (io.ReadCloser, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	data, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// setupSkillServiceTestDB returns an isolated in-memory SQLite DB with the
// skills table migrated. Tests in this package construct the SkillService
// directly so they never touch the package-level global DB.
func setupSkillServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&skill.Skill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestGetSkillMd is the RED→GREEN test for the preview endpoint's service
// method. It stages a zip containing SKILL.md in a fake OSS bucket, wires up
// the SkillService against an in-memory DB holding the matching skill record,
// and verifies the markdown body is returned verbatim.
func TestGetSkillMd(t *testing.T) {
	db := setupSkillServiceTestDB(t)

	uploader := &mockUploader{data: make(map[string][]byte)}

	// Stage a zip with SKILL.md nested under a top-level directory �?this is
	// the layout produced by the CLI's packDir.
	zipBytes := buildZip(t, map[string]string{
		"my-skill/SKILL.md": "---\nname: my-skill\ndescription: test\n---\n# My Skill\nbody text",
	})
	ossKey := "expert-skills/test-skill.zip"
	uploader.data[ossKey] = zipBytes

	// Persist the matching skill record so GetSkillMd can resolve name→url.
	sk := &skill.Skill{
		Name: "test-skill",
		Type: "expert",
		URL:  ossKey,
	}
	if err := db.Create(sk).Error; err != nil {
		t.Fatal(err)
	}

	svc := &SkillService{
		repo:     repository.NewSkillRepositoryWithDB(db),
		uploader: uploader,
		cdnHost:  "https://cdn.example.com",
	}

	entries, err := svc.GetSkillMd("test-skill")
	if err != nil {
		t.Fatalf("GetSkillMd failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if !strings.Contains(entries[0].Content, "# My Skill") {
		t.Errorf("content = %q, want to contain '# My Skill'", entries[0].Content)
	}
	if !strings.Contains(entries[0].Content, "body text") {
		t.Errorf("content = %q, want to contain 'body text'", entries[0].Content)
	}
}

// TestGetSkillMd_SkillNotFound verifies the service surfaces a not-found error
// (mapped to 404 by the handler) when the skill name doesn't exist in the DB.
func TestGetSkillMd_SkillNotFound(t *testing.T) {
	db := setupSkillServiceTestDB(t)
	svc := &SkillService{
		repo:     repository.NewSkillRepositoryWithDB(db),
		uploader: &mockUploader{data: make(map[string][]byte)},
		cdnHost:  "https://cdn.example.com",
	}

	_, err := svc.GetSkillMd("does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != skill.ErrSkillNotFound {
		t.Errorf("err = %v, want skill.ErrSkillNotFound", err)
	}
}

// TestGetSkillMd_FileNotFound verifies the service surfaces a dedicated
// file-not-found error (mapped to 404 by the handler) when the skill record
// exists but has no associated OSS file.
func TestGetSkillMd_FileNotFound(t *testing.T) {
	db := setupSkillServiceTestDB(t)
	sk := &skill.Skill{
		Name: "no-file-skill",
		Type: "expert",
		URL:  "",
	}
	if err := db.Create(sk).Error; err != nil {
		t.Fatal(err)
	}

	svc := &SkillService{
		repo:     repository.NewSkillRepositoryWithDB(db),
		uploader: &mockUploader{data: make(map[string][]byte)},
		cdnHost:  "https://cdn.example.com",
	}

	_, err := svc.GetSkillMd("no-file-skill")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != skill.ErrSkillFileNotFound {
		t.Errorf("err = %v, want skill.ErrSkillFileNotFound", err)
	}
}

// TestGetSkillMd_DownloadFailure verifies that an OSS download error is surfaced
// rather than being swallowed or misreported as a not-found error.
func TestGetSkillMd_DownloadFailure(t *testing.T) {
	db := setupSkillServiceTestDB(t)
	sk := &skill.Skill{
		Name: "broken-download-skill",
		Type: "expert",
		URL:  "expert-skills/broken-download-skill.zip",
	}
	if err := db.Create(sk).Error; err != nil {
		t.Fatal(err)
	}

	svc := &SkillService{
		repo: repository.NewSkillRepositoryWithDB(db),
		uploader: &mockUploader{
			data:        make(map[string][]byte),
			downloadErr: fmt.Errorf("oss download failed"),
		},
		cdnHost: "https://cdn.example.com",
	}

	_, err := svc.GetSkillMd("broken-download-skill")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err == skill.ErrSkillNotFound || err == skill.ErrSkillFileNotFound {
		t.Errorf("expected wrapped download error, got sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "下载技能文件失败") {
		t.Errorf("error = %q, want to contain '下载技能文件失败'", err.Error())
	}
}

// TestGetSkillMd_UnsafePath verifies that the preview endpoint re-applies the
// same path-safety check used at upload time, rejecting archives with traversal
// entries.
func TestGetSkillMd_UnsafePath(t *testing.T) {
	db := setupSkillServiceTestDB(t)

	uploader := &mockUploader{data: make(map[string][]byte)}
	zipBytes := buildZip(t, map[string]string{
		"my-skill/SKILL.md": validFrontmatter,
		"my-skill/../evil":  "owned",
	})
	ossKey := "expert-skills/unsafe-skill.zip"
	uploader.data[ossKey] = zipBytes

	sk := &skill.Skill{
		Name: "unsafe-skill",
		Type: "expert",
		URL:  ossKey,
	}
	if err := db.Create(sk).Error; err != nil {
		t.Fatal(err)
	}

	svc := &SkillService{
		repo:     repository.NewSkillRepositoryWithDB(db),
		uploader: uploader,
		cdnHost:  "https://cdn.example.com",
	}

	_, err := svc.GetSkillMd("unsafe-skill")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != skill.ErrInvalidSkillFile {
		t.Errorf("err = %v, want skill.ErrInvalidSkillFile", err)
	}
}
