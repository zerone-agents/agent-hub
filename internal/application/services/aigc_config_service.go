package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand"
	"regexp"
	"strings"
	"time"

	"control-panel/internal/domain/aigc"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/internal/infrastructure/deployer"
	persistence "control-panel/internal/infrastructure/persistence"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AigcConfigService manages the per-tenant AIGC content-labeling config
// (GB 45438-2025). aigc_configs is purely per-tenant: there is no shared
// row and no fallback — a tenant without its own row is simply
// unconfigured (no AIGC label injected on deploy).
// Backed by GORM directly, like CLITokenService.
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

// fetch resolves the tenant's own config row (tenant_id = ?), else
// gorm.ErrRecordNotFound. No shared fallback.
func (s *AigcConfigService) fetch(tenantID string) (*aigc.Config, error) {
	var rec aigc.Config
	err := s.db.Where("tenant_id = ?", tenantID).First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *AigcConfigService) Get(tenantID string) (*ConfigDTO, error) {
	rec, err := s.fetch(tenantID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ConfigDTO{Configured: false}, nil
	}
	if err != nil {
		return nil, err
	}
	dto := aigcToDTO(rec)
	return &dto, nil
}

// aigcSaveBackoff 是 Save 重试的退避间隔（10–50ms 抖动）；
// 包级变量便于测试置零（见 aigc_config_retry_test.go）。
var aigcSaveBackoff = func() time.Duration {
	return time.Duration(10+mrand.Intn(40)) * time.Millisecond
}

// isRetryableMySQLError：1062（唯一键冲突，重走已存在行路径）与 1213（死锁，
// 重跑完整事务）重试；1205（锁等待超时）等快速失败（spec §3.3）。
func isRetryableMySQLError(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062 || me.Number == 1213
	}
	return false
}

// withRetry 驱动有界重试：总尝试 ≤3，重试前退避抖动；
// attempt 必须是自包含的完整事务（saveOnce），非可重试错误立即返回。
func withRetry(attempt func() error) error {
	var err error
	for i := 0; i < 3; i++ { // 总尝试上限 3
		if i > 0 {
			time.Sleep(aigcSaveBackoff())
		}
		if err = attempt(); err == nil {
			return nil
		}
		if !isRetryableMySQLError(err) {
			return err
		}
	}
	return err
}

// Save creates or updates the tenant's own config row. On create it
// generates the signing key and derives the ContentProducer; on update it
// keeps the existing signing key. 成功提交的尝试返回权威变更回执
// （create=全集；update=实际值变化字段；幂等=[]）；任何失败（含 1205
// 快速失败）dto 与 receipt 均为 nil（spec §3.3）。
func (s *AigcConfigService) Save(tenantID, uscc, companyName string) (*ConfigDTO, *aigc.AigcMutationReceipt, error) {
	if tenantID == "" {
		return nil, nil, persistence.ErrTenantIDRequired
	}
	uscc = strings.ToUpper(strings.TrimSpace(uscc))
	if !usccPattern.MatchString(uscc) {
		return nil, nil, errors.New("统一社会信用代码须为 18 位数字与大写字母（不含 I/O/S/V/Z）")
	}
	companyName = strings.TrimSpace(companyName)
	if companyName == "" {
		return nil, nil, errors.New("公司完整名称不能为空")
	}
	var dto *ConfigDTO
	var rcpt *aigc.AigcMutationReceipt
	if err := withRetry(func() error {
		d, r, e := s.saveOnce(tenantID, uscc, companyName)
		if e == nil {
			dto, rcpt = d, &r
		}
		return e
	}); err != nil {
		return nil, nil, err // 失败（含 1205）→ dto/rcpt 均 nil
	}
	return dto, rcpt, nil
}

// saveOnce：一次完整事务（SELECT [FOR UPDATE] → 比较/写入 → COMMIT）。
// 每次重试从本函数重新开始（spec §3.3）。
func (s *AigcConfigService) saveOnce(tenantID, uscc, companyName string) (*ConfigDTO, aigc.AigcMutationReceipt, error) {
	var rcpt aigc.AigcMutationReceipt
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, rcpt, tx.Error
	}
	rollback := true
	defer func() {
		if rollback {
			tx.Rollback()
		}
	}()
	// 只查本租户行——upsert 永远只作用于本租户自有行。
	q := tx.Where("tenant_id = ?", tenantID)
	if s.db.Dialector.Name() == "mysql" { // FOR UPDATE 仅 MySQL 方言（SQLite 不支持该子句）
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rec aigc.Config
	err := q.First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		key, kerr := generateSigningKey()
		if kerr != nil {
			return nil, rcpt, kerr
		}
		enc, eerr := providerdomain.Encrypt(key, s.encryptionKey)
		if eerr != nil {
			return nil, rcpt, eerr
		}
		rec = aigc.Config{
			TenantID:            tenantID,
			USCC:                uscc,
			CompanyName:         companyName,
			ContentProducer:     deriveContentProducer(uscc),
			SigningKeyEncrypted: enc,
		}
		if cerr := tx.Create(&rec).Error; cerr != nil {
			return nil, rcpt, cerr
		}
		rcpt = aigc.AigcMutationReceipt{
			Created:       true,
			ChangedFields: []aigc.AigcConfigField{aigc.AigcFieldUSCC, aigc.AigcFieldCompanyName},
		}
	case err != nil:
		return nil, rcpt, err
	default:
		rcpt.ChangedFields = []aigc.AigcConfigField{}
		if rec.USCC != uscc {
			rcpt.ChangedFields = append(rcpt.ChangedFields, aigc.AigcFieldUSCC)
		}
		if rec.CompanyName != companyName {
			rcpt.ChangedFields = append(rcpt.ChangedFields, aigc.AigcFieldCompanyName)
		}
		rec.USCC = uscc
		rec.CompanyName = companyName
		rec.ContentProducer = deriveContentProducer(uscc)
		if uerr := tx.Save(&rec).Error; uerr != nil {
			return nil, rcpt, uerr
		}
	}
	if cerr := tx.Commit().Error; cerr != nil {
		return nil, rcpt, cerr
	}
	rollback = false
	dto := aigcToDTO(&rec)
	return &dto, rcpt, nil
}

// RotateKey rotates the tenant's own row. A tenant without its own row is
// unconfigured and must Save first before rotating.
func (s *AigcConfigService) RotateKey(tenantID string) (*ConfigDTO, error) {
	if tenantID == "" {
		return nil, persistence.ErrTenantIDRequired
	}
	// 只查本租户自有行：无自有行即未配置，报错而非轮换。
	var rec aigc.Config
	err := s.db.Where("tenant_id = ?", tenantID).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("本租户尚未配置 AIGC 信息，请先保存本租户配置再轮换密钥")
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

// Delete removes the tenant's own row only.
func (s *AigcConfigService) Delete(tenantID string) error {
	if tenantID == "" {
		return persistence.ErrTenantIDRequired
	}
	return s.db.Where("tenant_id = ?", tenantID).Delete(&aigc.Config{}).Error
}

// DeployerConfig builds the deployer payload for the tenant's own config.
// Returns (nil, nil) when the tenant has no own row (unconfigured) so
// callers can leave the request field unset. A decryption failure is an
// error — never silently deploy without a signature.
func (s *AigcConfigService) DeployerConfig(tenantID string) (*deployer.AigcConfig, error) {
	rec, err := s.fetch(tenantID)
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
