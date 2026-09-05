package services

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/skill"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/oss"
)

// SkillService provides business logic for managing skills and their file uploads.
type SkillService struct {
	repo     *repository.SkillRepository
	uploader oss.OSSUploader
	cdnHost  string
}

// NewSkillService creates a new SkillService with the given OSS uploader and CDN host.
func NewSkillService(uploader oss.OSSUploader, cdnHost string) *SkillService {
	return &SkillService{
		repo:     repository.NewSkillRepository(),
		uploader: uploader,
		cdnHost:  cdnHost,
	}
}

// CreateSkillInput holds the parameters for creating a new skill with file upload.
type CreateSkillInput struct {
	Name          string
	Type          string
	Title         string
	TitleEn       string
	Description   string
	DescriptionEn string
	File          io.Reader
	FileSize      int64
}

// UpdateSkillInput holds the optional parameters for updating a skill.
type UpdateSkillInput struct {
	Title         string
	TitleEn       string
	Description   string
	DescriptionEn string
	File          io.Reader
	FileSize      int64
}

// SkillDTO is the skill representation returned by the API.
type SkillDTO struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	TitleEn       string `json:"titleEn"`
	Description   string `json:"description"`
	DescriptionEn string `json:"descriptionEn"`
	URL           string `json:"url"`
	FileHash      string `json:"fileHash"`
	FileSize      int64  `json:"fileSize"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// DownloadDTO contains a presigned download URL and its expiration time.
type DownloadDTO struct {
	URL       string `json:"url"`
	ExpiresIn int64  `json:"expiresIn"`
}

// ListAll returns all skills, optionally filtered by type.
func (s *SkillService) ListAll(tenantID, skillType string) ([]*SkillDTO, error) {
	var skills []*skill.Skill
	var err error

	if skillType != "" {
		skills, err = s.repo.ListByType(tenantID, skillType)
	} else {
		skills, err = s.repo.ListAll(tenantID)
	}

	if err != nil {
		return nil, fmt.Errorf("获取技能列表失败: %w", err)
	}

	result := make([]*SkillDTO, 0, len(skills))
	for _, sk := range skills {
		result = append(result, s.toDTO(sk))
	}
	return result, nil
}

// GetSkill returns a single skill by name.
func (s *SkillService) GetSkill(tenantID, name string) (*SkillDTO, error) {
	sk, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, skill.ErrSkillNotFound
	}

	return s.toDTO(sk), nil
}

// CreateSkill validates, uploads the file, and creates a new skill.
func (s *SkillService) CreateSkill(tenantID string, input *CreateSkillInput) (*SkillDTO, error) {
	if err := ValidateSkillName(input.Name); err != nil {
		return nil, err
	}

	if input.Type == "" {
		input.Type = "expert"
	}
	if err := ValidateSkillType(input.Type); err != nil {
		return nil, err
	}

	exists, err := s.repo.ExistsByName(tenantID, input.Name)
	if err != nil {
		return nil, fmt.Errorf("检查技能存在性失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("技能 '%s' 已存在", input.Name)
	}

	const maxFileSize = 50 * 1024 * 1024
	if input.FileSize > maxFileSize {
		return nil, skill.ErrFileTooLarge
	}

	// Re-validate the zip server-side before it lands in OSS. The size cap
	// above guards against OOM; this guards against malformed/hostile
	// content (missing SKILL.md, path-traversal entries, bad frontmatter).
	// Spec §7.1 step 3 + §11 risk matrix: "CLI 端预检 + 服务端再校验（双保险）".
	// Returns the buffered bytes so we can hand them to OSS upload without
	// re-reading the now-consumed multipart stream.
	validatedBytes, err := ValidateSkillZip(input.File)
	if err != nil {
		return nil, err
	}

	ossKey := BuildOSSKey(input.Type, input.Name)

	ctx := context.Background()
	fileHash, err := s.uploader.Upload(ctx, ossKey, bytes.NewReader(validatedBytes), int64(len(validatedBytes)))
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %w", err)
	}

	sk := &skill.Skill{
		Name:          input.Name,
		Type:          input.Type,
		Title:         input.Title,
		TitleEn:       input.TitleEn,
		Description:   input.Description,
		DescriptionEn: input.DescriptionEn,
		URL:           ossKey,
		FileHash:      fileHash,
		FileSize:      int64(len(validatedBytes)),
	}

	if err := s.repo.Create(tenantID, sk); err != nil {
		_ = s.uploader.Delete(ctx, ossKey)
		return nil, fmt.Errorf("创建技能失败: %w", err)
	}

	return s.toDTO(sk), nil
}

// UpdateSkill modifies an existing skill's metadata and optionally replaces its file.
func (s *SkillService) UpdateSkill(tenantID, name string, input *UpdateSkillInput) (*SkillDTO, error) {
	sk, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, skill.ErrSkillNotFound
	}

	s.updateSkillFields(sk, input)

	if input.File != nil {
		if err := s.updateSkillFile(sk, input); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Update(tenantID, sk); err != nil {
		return nil, fmt.Errorf("更新技能失败: %w", err)
	}

	return s.toDTO(sk), nil
}

// updateSkillFields applies text field updates to a skill entity.
func (s *SkillService) updateSkillFields(sk *skill.Skill, input *UpdateSkillInput) {
	if input.Title != "" {
		sk.Title = input.Title
	}
	if input.TitleEn != "" {
		sk.TitleEn = input.TitleEn
	}
	if input.Description != "" {
		sk.Description = input.Description
	}
	if input.DescriptionEn != "" {
		sk.DescriptionEn = input.DescriptionEn
	}
}

// updateSkillFile handles file upload and old file cleanup for a skill update.
func (s *SkillService) updateSkillFile(sk *skill.Skill, input *UpdateSkillInput) error {
	const maxFileSize = 50 * 1024 * 1024
	if input.FileSize > maxFileSize {
		return skill.ErrFileTooLarge
	}

	// Same server-side re-validation as CreateSkill — a replacement zip is
	// just as capable of carrying path-traversal entries or a missing
	// SKILL.md as a fresh upload.
	validatedBytes, err := ValidateSkillZip(input.File)
	if err != nil {
		return err
	}

	oldKey := sk.URL
	ossKey := BuildOSSKey(sk.Type, sk.Name)

	ctx := context.Background()
	fileHash, err := s.uploader.Upload(ctx, ossKey, bytes.NewReader(validatedBytes), int64(len(validatedBytes)))
	if err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}

	if oldKey != "" && oldKey != ossKey {
		if err := s.uploader.Delete(ctx, oldKey); err != nil {
			log.Printf("删除旧 OSS 文件失败 (skill=%s, key=%s): %v", sk.Name, oldKey, err)
		}
	}

	sk.URL = ossKey
	sk.FileHash = fileHash
	sk.FileSize = int64(len(validatedBytes))
	return nil
}

// DeleteSkill removes a skill and its associated OSS file.
func (s *SkillService) DeleteSkill(tenantID, name string) error {
	sk, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return skill.ErrSkillNotFound
	}

	own, foreign, err := s.repo.GetSkillBindingsScoped(tenantID, sk.ID)
	if err != nil {
		return fmt.Errorf("查询技能绑定失败: %w", err)
	}
	if len(own) > 0 || foreign {
		return &agent.SkillInUseError{SkillName: sk.Name, Agents: own, Foreign: foreign}
	}

	if err := s.repo.Delete(tenantID, sk.ID); err != nil {
		if repository.IsFKConstraintError(err) {
			// 并发后盾（#123 review P1）：守卫通过的瞬间绑定被并发写入 →
			// ON DELETE RESTRICT 拒绝删除 → 重查绑定给出准确 409 名单。
			own, foreign, qerr := s.repo.GetSkillBindingsScoped(tenantID, sk.ID)
			if qerr == nil {
				return &agent.SkillInUseError{SkillName: sk.Name, Agents: own, Foreign: foreign}
			}
		}
		return fmt.Errorf("删除技能失败: %w", err)
	}

	// 行已删成功：残留对象只是无引用方的孤立对象，删除失败无害，仅记日志。
	if sk.URL != "" && s.uploader != nil {
		ctx := context.Background()
		if err := s.uploader.Delete(ctx, sk.URL); err != nil {
			log.Printf("删除 OSS 文件失败 (skill=%s, key=%s): %v", sk.Name, sk.URL, err)
		}
	}

	return nil
}

// Download generates a download URL for a skill's file. When OSS_CDN_HOST is
// configured, returns a permanent public CDN URL (ExpiresIn=0); otherwise
// falls back to a short-lived presigned OSS URL (1h expiry).
func (s *SkillService) Download(tenantID, name string) (*DownloadDTO, error) {
	sk, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, skill.ErrSkillNotFound
	}

	if sk.URL == "" {
		return nil, skill.ErrSkillFileNotFound
	}

	if s.cdnHost != "" {
		return &DownloadDTO{
			URL:       buildPublicURL(s.cdnHost, sk.URL),
			ExpiresIn: 0,
		}, nil
	}

	ctx := context.Background()
	url, err := s.uploader.GetPresignedURL(ctx, sk.URL)
	if err != nil {
		return nil, fmt.Errorf("生成下载链接失败: %w", err)
	}

	return &DownloadDTO{
		URL:       url,
		ExpiresIn: 3600,
	}, nil
}

// GetSkillMd fetches every SKILL.md from a skill's zip file stored in OSS.
// It downloads the zip, runs FindAllSkillMd to enumerate all `**/SKILL.md`
// files at any depth, and returns them sorted by path. Used by the preview
// endpoint in the skill form modal — the modal's tab UI lets the user
// switch between entries when the zip is a bundle.
//
// Empty (but non-nil) slice is returned when the zip is valid but contains
// no SKILL.md anywhere; that case still surfaces as a preview "no content"
// state rather than an error, since the upload validator already rejected
// such zips at write time.
func (s *SkillService) GetSkillMd(tenantID, name string) ([]SkillMdEntry, error) {
	sk, err := s.repo.GetByName(tenantID, name)
	if err != nil {
		return nil, skill.ErrSkillNotFound
	}

	if sk.URL == "" {
		return nil, skill.ErrSkillFileNotFound
	}

	ctx := context.Background()
	rc, err := s.uploader.Download(ctx, sk.URL)
	if err != nil {
		return nil, fmt.Errorf("下载技能文件失败: %w", err)
	}
	defer rc.Close()

	buf, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("读取技能文件失败: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("解析 zip 文件失败: %w", err)
	}

	// Re-validate path safety even though the file was checked at upload time;
	// this keeps the preview endpoint consistent with the upload security model.
	for _, f := range zr.File {
		if isUnsafeZipPath(f.Name) {
			return nil, skill.ErrInvalidSkillFile
		}
	}

	return FindAllSkillMd(zr)
}

// resolveURL returns a fetchable URL for a skill's stored OSS key.
// When a CDN host is configured, returns a permanent CDN URL. Otherwise
// falls back to a short-lived presigned URL generated directly from the
// configured OSS endpoint. This makes skill storage truly optional with
// respect to CDN: if no CDN is set, the list still shows "已上传" and the
// download button remains enabled, with the actual download served by the
// /download endpoint or the presigned URL here.
func (s *SkillService) resolveURL(sk *skill.Skill) string {
	if sk.URL == "" {
		return ""
	}
	if s.cdnHost != "" {
		return buildPublicURL(s.cdnHost, sk.URL)
	}
	// Fallback to presigned URL when no CDN is configured. The file is still
	// safely private; the URL expires in 1 hour.
	ctx := context.Background()
	url, err := s.uploader.GetPresignedURL(ctx, sk.URL)
	if err != nil {
		log.Printf("生成预签名 URL 失败 (skill=%s, key=%s): %v", sk.Name, sk.URL, err)
		return ""
	}
	return url
}

// buildPublicURL turns a stored OSS object key (e.g. "expert/foo/abc.zip")
// into a public http(s) URL using the CDN host as prefix. Both sides are
// trimmed of surrounding slashes to avoid double// or empty segments.
func buildPublicURL(cdnHost, ossKey string) string {
	return strings.TrimRight(cdnHost, "/") + "/" + strings.TrimLeft(ossKey, "/")
}

// toDTO converts a Skill domain entity to a SkillDTO.
// The URL field is only populated when OSS_CDN_HOST is configured, since
// without a public prefix the stored OSS key is not a fetchable URL —
// callers must use the /download endpoint to get a presigned URL instead.
func (s *SkillService) toDTO(sk *skill.Skill) *SkillDTO {
	url := s.resolveURL(sk)
	return &SkillDTO{
		ID:            sk.ID,
		Name:          sk.Name,
		Type:          sk.Type,
		Title:         sk.Title,
		TitleEn:       sk.TitleEn,
		Description:   sk.Description,
		DescriptionEn: sk.DescriptionEn,
		URL:           url,
		FileHash:      sk.FileHash,
		FileSize:      sk.FileSize,
		CreatedAt:     sk.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     sk.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// GetAgentSkills returns the skill names associated with an agent.
func (s *SkillService) GetAgentSkills(tenantID, agentName string) ([]string, error) {
	agentRepo := repository.NewAgentRepository()
	agentCfg, err := agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return nil, fmt.Errorf("Agent '%s' 不存在", agentName)
	}
	return s.repo.GetAgentSkills(agentCfg.ID)
}

// UpdateAgentSkills replaces the skill list associated with an agent and
// automatically mounts/unmounts the built-in Skill tool accordingly.
func (s *SkillService) UpdateAgentSkills(tenantID, agentName string, skillNames []string) error {
	agentRepo := repository.NewAgentRepository()
	toolRepo := repository.NewToolRepository()
	agentCfg, err := agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return fmt.Errorf("Agent '%s' 不存在", agentName)
	}

	skillIDs := make([]uint64, 0, len(skillNames))
	for _, name := range skillNames {
		sk, err := s.repo.GetByName(tenantID, name)
		if err != nil {
			return fmt.Errorf("Skill '%s' 不存在", name)
		}
		skillIDs = append(skillIDs, sk.ID)
	}
	if err := s.repo.ReplaceAgentSkills(agentCfg.ID, skillIDs); err != nil {
		return err
	}

	skillTool, err := toolRepo.GetByName(tenantID, "Skill")
	if err != nil {
		return fmt.Errorf("内置 Skill tool 不存在: %w", err)
	}
	if len(skillNames) > 0 {
		if err := agentRepo.EnsureAgentToolBinding(agentCfg.ID, skillTool.ID); err != nil {
			return fmt.Errorf("挂载 Skill tool 失败: %w", err)
		}
	} else {
		if err := agentRepo.RemoveAgentToolBinding(agentCfg.ID, skillTool.ID); err != nil {
			return fmt.Errorf("卸载 Skill tool 失败: %w", err)
		}
	}
	return nil
}
