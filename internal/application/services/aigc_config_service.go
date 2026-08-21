package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"control-panel/internal/domain/aigc"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/internal/infrastructure/deployer"

	"gorm.io/gorm"
)

// AigcConfigService manages the single-row global AIGC content-labeling
// config (GB 45438-2025). Backed by GORM directly, like CLITokenService.
type AigcConfigService struct {
	db            *gorm.DB
	encryptionKey string
	models        providerModelCodeSource
}

// providerModelCodeSource is the subset of the provider repository needed to
// build the deployer's model code map. *repository.ProviderRepository
// satisfies this via ListAllModelsUnscoped. AIGC 模型码是全局映射
// （0001=GLM-4.5 等，按 model_id 跨租户复用），此处走无租户上下文的
// 系统对账路径。
type providerModelCodeSource interface {
	ListAllModelsUnscoped() ([]providerdomain.ProviderModel, error)
}

func NewAigcConfigService(db *gorm.DB, encryptionKey string, models providerModelCodeSource) *AigcConfigService {
	return &AigcConfigService{db: db, encryptionKey: encryptionKey, models: models}
}

// ConfigDTO is the safe projection returned to the admin UI. It never
// contains the signing key (neither plaintext nor ciphertext).
type ConfigDTO struct {
	Configured           bool   `json:"configured"`
	USCC                 string `json:"uscc,omitempty"`
	CompanyName          string `json:"companyName,omitempty"`
	ContentProducer      string `json:"contentProducer,omitempty"`
	SigningKeyConfigured bool   `json:"signingKeyConfigured"`
}

var usccPattern = regexp.MustCompile(`^[0-9A-HJ-NPQRTUWXY]{18}$`)

// defaultModelSlot fills ContentProducer bits 24-27 until the runtime
// replaces them via modelCodes on a per-model basis.
const defaultModelSlot = "0000"

// deriveContentProducer builds the 27-char service-provider code:
// "00" (format) + "1" (org) + "1" (USCC binding) + USCC + "1" (service type)
// + 4-digit model slot.
func deriveContentProducer(uscc string) string {
	return "00" + "1" + "1" + uscc + "1" + defaultModelSlot
}

func generateSigningKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func aigcToDTO(rec *aigc.Config) ConfigDTO {
	return ConfigDTO{
		Configured:           true,
		USCC:                 rec.USCC,
		CompanyName:          rec.CompanyName,
		ContentProducer:      rec.ContentProducer,
		SigningKeyConfigured: rec.SigningKeyEncrypted != "",
	}
}

func (s *AigcConfigService) Get() (*ConfigDTO, error) {
	var rec aigc.Config
	err := s.db.First(&rec, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ConfigDTO{Configured: false}, nil
	}
	if err != nil {
		return nil, err
	}
	dto := aigcToDTO(&rec)
	return &dto, nil
}

// Save creates or updates the config. On create it generates the signing key
// and derives the ContentProducer; on update it keeps the existing signing key.
func (s *AigcConfigService) Save(uscc, companyName string) (*ConfigDTO, error) {
	uscc = strings.ToUpper(strings.TrimSpace(uscc))
	if !usccPattern.MatchString(uscc) {
		return nil, errors.New("统一社会信用代码须为 18 位数字与大写字母（不含 I/O/S/V/Z）")
	}
	companyName = strings.TrimSpace(companyName)
	if companyName == "" {
		return nil, errors.New("公司完整名称不能为空")
	}

	var rec aigc.Config
	err := s.db.First(&rec, 1).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		key, err := generateSigningKey()
		if err != nil {
			return nil, err
		}
		enc, err := providerdomain.Encrypt(key, s.encryptionKey)
		if err != nil {
			return nil, err
		}
		rec = aigc.Config{
			ID:                  1,
			USCC:                uscc,
			CompanyName:         companyName,
			ContentProducer:     deriveContentProducer(uscc),
			SigningKeyEncrypted: enc,
		}
		if err := s.db.Create(&rec).Error; err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		rec.USCC = uscc
		rec.CompanyName = companyName
		rec.ContentProducer = deriveContentProducer(uscc)
		if err := s.db.Save(&rec).Error; err != nil {
			return nil, err
		}
	}
	dto := aigcToDTO(&rec)
	return &dto, nil
}

func (s *AigcConfigService) RotateKey() (*ConfigDTO, error) {
	var rec aigc.Config
	if err := s.db.First(&rec, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("AIGC 标识尚未配置")
		}
		return nil, err
	}
	key, err := generateSigningKey()
	if err != nil {
		return nil, err
	}
	enc, err := providerdomain.Encrypt(key, s.encryptionKey)
	if err != nil {
		return nil, err
	}
	rec.SigningKeyEncrypted = enc
	if err := s.db.Save(&rec).Error; err != nil {
		return nil, err
	}
	dto := aigcToDTO(&rec)
	return &dto, nil
}

func (s *AigcConfigService) Delete() error {
	return s.db.Delete(&aigc.Config{}, 1).Error
}

// DeployerConfig builds the deployer payload. Returns (nil, nil) when not
// configured so callers can leave the request field unset. A decryption
// failure is an error — never silently deploy without a signature.
func (s *AigcConfigService) DeployerConfig() (*deployer.AigcConfig, error) {
	var rec aigc.Config
	err := s.db.First(&rec, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	key, err := providerdomain.Decrypt(rec.SigningKeyEncrypted, s.encryptionKey)
	if err != nil {
		return nil, err
	}
	codes, err := s.buildModelCodes()
	if err != nil {
		return nil, fmt.Errorf("load aigc model codes: %w", err)
	}
	explicitHint := true
	return &deployer.AigcConfig{
		Enabled:         true,
		ContentProducer: rec.ContentProducer,
		SigningKey:      key,
		ExplicitHint:    &explicitHint,
		ModelCodes:      codes,
	}, nil
}

// buildModelCodes scans all provider_models and builds a deduplicated
// {modelID: code} map. Rows with empty code or empty modelID are skipped.
func (s *AigcConfigService) buildModelCodes() (map[string]string, error) {
	rows, err := s.models.ListAllModelsUnscoped()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.AigcCode == "" || r.ModelID == "" {
			continue
		}
		if _, exists := out[r.ModelID]; !exists {
			out[r.ModelID] = r.AigcCode
		}
	}
	return out, nil
}
