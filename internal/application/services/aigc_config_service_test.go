package services

import (
	"errors"
	"strings"
	"testing"

	"control-panel/internal/domain/aigc"
	providerdomain "control-panel/internal/domain/provider"
	persistence "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const aigcTestEncKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const testUSCC = "91320118MAK93FC72D"

func setupAigcSvc(t *testing.T) (*AigcConfigService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&aigc.Config{}))
	return NewAigcConfigService(db, aigcTestEncKey, fakeModelSource{}), db
}

// fakeModelSource is a stand-in for the provider repository in unit tests.
type fakeModelSource struct {
	rows []providerdomain.ProviderModel
	err  error
}

func (f fakeModelSource) ListAllModelsUnscoped() ([]providerdomain.ProviderModel, error) {
	return f.rows, f.err
}

func newAigcSvcWithModels(t *testing.T, models providerModelCodeSource) *AigcConfigService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&aigc.Config{}))
	return NewAigcConfigService(db, aigcTestEncKey, models)
}

func TestAigcSave_CreateGeneratesKeyAndProducer(t *testing.T) {
	svc, db := setupAigcSvc(t)

	dto, err := svc.Save("acme", testUSCC, "南京测试科技有限公司")
	require.NoError(t, err)
	require.True(t, dto.Configured)
	require.True(t, dto.SigningKeyConfigured)
	require.Len(t, dto.ContentProducer, 27)
	require.Equal(t, "0011"+testUSCC+"1"+"0000", dto.ContentProducer)
	// ConfigDTO 不再包含 ModelCodes 字段（编译期保证）

	var rec aigc.Config
	require.NoError(t, db.First(&rec, 1).Error)
	require.True(t, strings.HasPrefix(rec.SigningKeyEncrypted, "enc:"))
	require.NotContains(t, rec.SigningKeyEncrypted, "signing") // 无明文语义
}

func TestAigcSave_RejectsInvalidUSCC(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	_, err := svc.Save("acme", "91320118MAK93FC72DIX", "公司") // 19 位
	require.Error(t, err)
	_, err = svc.Save("acme", "91320118MAK93FC72S", "公司") // 含非法字符 S
	require.Error(t, err)
	_, err = svc.Save("acme", testUSCC, "  ")
	require.Error(t, err)
}

func TestAigcSave_UpdateKeepsSigningKey(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save("acme", testUSCC, "公司A")
	require.NoError(t, err)
	var before aigc.Config
	require.NoError(t, db.First(&before, 1).Error)

	_, err = svc.Save("acme", testUSCC, "公司B")
	require.NoError(t, err)
	var after aigc.Config
	require.NoError(t, db.First(&after, 1).Error)
	require.Equal(t, before.SigningKeyEncrypted, after.SigningKeyEncrypted)
	require.Equal(t, "公司B", after.CompanyName)
}

func TestAigcRotateKey_ReplacesKey(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save("acme", testUSCC, "公司A")
	require.NoError(t, err)
	var before aigc.Config
	require.NoError(t, db.First(&before, 1).Error)

	_, err = svc.RotateKey("acme")
	require.NoError(t, err)
	var after aigc.Config
	require.NoError(t, db.First(&after, 1).Error)
	require.NotEqual(t, before.SigningKeyEncrypted, after.SigningKeyEncrypted)
}

func TestAigcRotateKey_RequiresExistingConfig(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	_, err := svc.RotateKey("acme")
	require.Error(t, err)
}

func TestAigcDeployerConfig_NotConfiguredReturnsNil(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.Nil(t, cfg)
}

func TestAigcDeployerConfig_DecryptsKey(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save("acme", testUSCC, "公司A")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.True(t, cfg.Enabled)
	require.NotNil(t, cfg.ExplicitHint)
	require.True(t, *cfg.ExplicitHint)

	var rec aigc.Config
	require.NoError(t, db.First(&rec, 1).Error)
	plain, err := providerdomain.Decrypt(rec.SigningKeyEncrypted, aigcTestEncKey)
	require.NoError(t, err)
	require.Equal(t, plain, cfg.SigningKey)
	require.Len(t, cfg.SigningKey, 64) // 32 字节 hex
}

func TestDeployerConfig_BuildsModelCodesFromProviderModels(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{rows: []providerdomain.ProviderModel{
		{ModelID: "glm-4.5", AigcCode: "0001"},
		{ProviderID: 2, ModelID: "glm-4.5", AigcCode: "0001"}, // 重复，去重
		{ModelID: "gpt-4o", AigcCode: "0005"},
		{ModelID: "empty", AigcCode: ""}, // 跳过空码
	}})
	_, err := svc.Save("acme", testUSCC, "公司")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"glm-4.5": "0001", "gpt-4o": "0005"}, cfg.ModelCodes)
}

func TestDeployerConfig_ModelCodesEmptyWhenNoModels(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{})
	_, err := svc.Save("acme", testUSCC, "公司")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, map[string]string{}, cfg.ModelCodes)
}

func TestDeployerConfig_NotConfiguredShortCircuitsModelQuery(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{err: errors.New("should not be called")})
	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.Nil(t, cfg) // aigc 未配置 → 不查 models
}

func TestDeployerConfig_ModelQueryErrorPropagates(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{err: errors.New("db down")})
	_, err := svc.Save("acme", testUSCC, "公司")
	require.NoError(t, err)

	_, err = svc.DeployerConfig("acme")
	require.Error(t, err)
	require.Contains(t, err.Error(), "load aigc model codes")
}

func TestAigcGet_NotConfigured(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	dto, err := svc.Get("acme")
	require.NoError(t, err)
	require.False(t, dto.Configured)
}

func TestAigcDelete_RemovesConfig(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	_, err := svc.Save("acme", testUSCC, "公司A")
	require.NoError(t, err)
	require.NoError(t, svc.Delete("acme"))
	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.Nil(t, cfg)
}

// --- per-tenant（Task 6）---

// seedSharedRow 直接落一行 tenant_id=” 的共享配置，模拟迁移后的存量全局行。
func seedSharedRow(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&aigc.Config{
		TenantID:            "",
		USCC:                testUSCC,
		CompanyName:         "共享公司",
		ContentProducer:     "0011" + testUSCC + "1" + "0000",
		SigningKeyEncrypted: "enc:shared",
	}).Error)
}

// 方案 A：无共享回退——即使存在 tenant_id=” 行，无自有行的租户也视为未配置。
func TestAigcGet_SharedRowNotUsedAsFallback(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	dto, err := svc.Get("acme")
	require.NoError(t, err)
	require.False(t, dto.Configured)
}

func TestAigcSave_TenantRowShadowsShared(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	_, err := svc.Save("acme", testUSCC, "租户A公司")
	require.NoError(t, err)

	dto, err := svc.Get("acme")
	require.NoError(t, err)
	require.Equal(t, "租户A公司", dto.CompanyName)

	// 共享行不被覆盖：USCC/公司名/密钥保持原样
	var shared aigc.Config
	require.NoError(t, db.Where("tenant_id = ''").First(&shared).Error)
	require.Equal(t, "共享公司", shared.CompanyName)
	require.Equal(t, "enc:shared", shared.SigningKeyEncrypted)
}

func TestAigcGet_OtherTenantWithoutOwnRowNotConfigured(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	_, err := svc.Save("acme", testUSCC, "租户A公司")
	require.NoError(t, err)

	dto, err := svc.Get("other")
	require.NoError(t, err)
	require.False(t, dto.Configured)
}

func TestAigcDeployerConfig_TenantRowPreferred(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	_, err := svc.Save("acme", testUSCC, "租户A公司")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig("acme")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotEqual(t, "enc:shared", cfg.SigningKey) // 不是共享行的（密文占位解不开也轮不到它）

	// 无自有行的租户不走共享行 → (nil, nil)，部署不注入 AIGC 标识
	cfg, err = svc.DeployerConfig("other")
	require.NoError(t, err)
	require.Nil(t, cfg)
}

func TestAigcDelete_OnlyOwnRow(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)
	_, err := svc.Save("acme", testUSCC, "租户A公司")
	require.NoError(t, err)

	require.NoError(t, svc.Delete("acme"))
	var count int64
	db.Model(&aigc.Config{}).Where("tenant_id = ''").Count(&count)
	require.EqualValues(t, 1, count) // 共享行仍在
	db.Model(&aigc.Config{}).Where("tenant_id = ?", "acme").Count(&count)
	require.EqualValues(t, 0, count)
}

func TestAigcRotateKey_NoOwnRowRejected(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db) // 租户只有共享行可读，无自有行

	_, err := svc.RotateKey("acme")
	require.Error(t, err)
	require.Contains(t, err.Error(), "本租户尚未配置 AIGC 信息，请先保存本租户配置再轮换密钥")

	// 共享行密钥保持不变
	var shared aigc.Config
	require.NoError(t, db.Where("tenant_id = ''").First(&shared).Error)
	require.Equal(t, "enc:shared", shared.SigningKeyEncrypted)
}

func TestAigcRotateKey_OwnRowRotatedSharedUntouched(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)
	_, err := svc.Save("acme", testUSCC, "租户A公司")
	require.NoError(t, err)
	var before aigc.Config
	require.NoError(t, db.Where("tenant_id = ?", "acme").First(&before).Error)

	_, err = svc.RotateKey("acme")
	require.NoError(t, err)
	var after aigc.Config
	require.NoError(t, db.Where("tenant_id = ?", "acme").First(&after).Error)
	require.NotEqual(t, before.SigningKeyEncrypted, after.SigningKeyEncrypted)

	var shared aigc.Config
	require.NoError(t, db.Where("tenant_id = ''").First(&shared).Error)
	require.Equal(t, "enc:shared", shared.SigningKeyEncrypted)
}

// --- 空租户守卫（I-2）：三个写方法必须拒绝 tenantID=""，且数据无变化 ---

func TestAigcSave_EmptyTenantIDRejected(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	_, err := svc.Save("", testUSCC, "越权公司")
	require.True(t, errors.Is(err, persistence.ErrTenantIDRequired), "空租户 Save 必须返回 ErrTenantIDRequired, got %v", err)

	var shared aigc.Config
	require.NoError(t, db.Where("tenant_id = ''").First(&shared).Error)
	require.Equal(t, "共享公司", shared.CompanyName) // 共享行未被覆写
	var count int64
	db.Model(&aigc.Config{}).Count(&count)
	require.EqualValues(t, 1, count) // 也没有新建行
}

func TestAigcRotateKey_EmptyTenantIDRejected(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	_, err := svc.RotateKey("")
	require.True(t, errors.Is(err, persistence.ErrTenantIDRequired), "空租户 RotateKey 必须返回 ErrTenantIDRequired, got %v", err)

	var shared aigc.Config
	require.NoError(t, db.Where("tenant_id = ''").First(&shared).Error)
	require.Equal(t, "enc:shared", shared.SigningKeyEncrypted)
}

func TestAigcDelete_EmptyTenantIDRejected(t *testing.T) {
	svc, db := setupAigcSvc(t)
	seedSharedRow(t, db)

	err := svc.Delete("")
	require.True(t, errors.Is(err, persistence.ErrTenantIDRequired), "空租户 Delete 必须返回 ErrTenantIDRequired, got %v", err)

	var count int64
	db.Model(&aigc.Config{}).Where("tenant_id = ''").Count(&count)
	require.EqualValues(t, 1, count) // 共享行未被删除
}

func TestAigcRotateKey_RotatesResolvedRow(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save("acme", testUSCC, "公司A") // 本租户行
	require.NoError(t, err)
	var before aigc.Config
	require.NoError(t, db.Where("tenant_id = ?", "acme").First(&before).Error)

	_, err = svc.RotateKey("acme")
	require.NoError(t, err)
	var after aigc.Config
	require.NoError(t, db.Where("tenant_id = ?", "acme").First(&after).Error)
	require.NotEqual(t, before.SigningKeyEncrypted, after.SigningKeyEncrypted)
}
