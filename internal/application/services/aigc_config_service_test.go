package services

import (
	"errors"
	"strings"
	"testing"

	"control-panel/internal/domain/aigc"
	providerdomain "control-panel/internal/domain/provider"

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

	dto, err := svc.Save(testUSCC, "南京测试科技有限公司")
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
	_, err := svc.Save("91320118MAK93FC72DIX", "公司") // 19 位
	require.Error(t, err)
	_, err = svc.Save("91320118MAK93FC72S", "公司") // 含非法字符 S
	require.Error(t, err)
	_, err = svc.Save(testUSCC, "  ")
	require.Error(t, err)
}

func TestAigcSave_UpdateKeepsSigningKey(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save(testUSCC, "公司A")
	require.NoError(t, err)
	var before aigc.Config
	require.NoError(t, db.First(&before, 1).Error)

	_, err = svc.Save(testUSCC, "公司B")
	require.NoError(t, err)
	var after aigc.Config
	require.NoError(t, db.First(&after, 1).Error)
	require.Equal(t, before.SigningKeyEncrypted, after.SigningKeyEncrypted)
	require.Equal(t, "公司B", after.CompanyName)
}

func TestAigcRotateKey_ReplacesKey(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save(testUSCC, "公司A")
	require.NoError(t, err)
	var before aigc.Config
	require.NoError(t, db.First(&before, 1).Error)

	_, err = svc.RotateKey()
	require.NoError(t, err)
	var after aigc.Config
	require.NoError(t, db.First(&after, 1).Error)
	require.NotEqual(t, before.SigningKeyEncrypted, after.SigningKeyEncrypted)
}

func TestAigcRotateKey_RequiresExistingConfig(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	_, err := svc.RotateKey()
	require.Error(t, err)
}

func TestAigcDeployerConfig_NotConfiguredReturnsNil(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	cfg, err := svc.DeployerConfig()
	require.NoError(t, err)
	require.Nil(t, cfg)
}

func TestAigcDeployerConfig_DecryptsKey(t *testing.T) {
	svc, db := setupAigcSvc(t)
	_, err := svc.Save(testUSCC, "公司A")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig()
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
	_, err := svc.Save(testUSCC, "公司")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"glm-4.5": "0001", "gpt-4o": "0005"}, cfg.ModelCodes)
}

func TestDeployerConfig_ModelCodesEmptyWhenNoModels(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{})
	_, err := svc.Save(testUSCC, "公司")
	require.NoError(t, err)

	cfg, err := svc.DeployerConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, map[string]string{}, cfg.ModelCodes)
}

func TestDeployerConfig_NotConfiguredShortCircuitsModelQuery(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{err: errors.New("should not be called")})
	cfg, err := svc.DeployerConfig()
	require.NoError(t, err)
	require.Nil(t, cfg) // aigc 未配置 → 不查 models
}

func TestDeployerConfig_ModelQueryErrorPropagates(t *testing.T) {
	svc := newAigcSvcWithModels(t, fakeModelSource{err: errors.New("db down")})
	_, err := svc.Save(testUSCC, "公司")
	require.NoError(t, err)

	_, err = svc.DeployerConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "load aigc model codes")
}

func TestAigcGet_NotConfigured(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	dto, err := svc.Get()
	require.NoError(t, err)
	require.False(t, dto.Configured)
}

func TestAigcDelete_RemovesConfig(t *testing.T) {
	svc, _ := setupAigcSvc(t)
	_, err := svc.Save(testUSCC, "公司A")
	require.NoError(t, err)
	require.NoError(t, svc.Delete())
	cfg, err := svc.DeployerConfig()
	require.NoError(t, err)
	require.Nil(t, cfg)
}
