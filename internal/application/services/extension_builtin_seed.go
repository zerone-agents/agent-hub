// 内置人物能力扩展种子（H7 P1）。
//
// 四个人物能力（情绪/认知立场/主观记忆/动态关系）历史上无条件生效。
// 本文件把它们注册为 default 租户的内置扩展（source=seed），默认
// 已安装 + 已启用：现有部署行为零变化，但管理员可通过 H7 扩展
// 生命周期 API/页面真实停用某个能力（停用语义见 PersonaCapabilityGate）。
//
// 种子幂等：按 (tenant, name) / (extension_id, version) / 安装唯一索引
// 判重，重复启动不产生重复记录；已存在则原样保留（管理员的启停状态
// 不会被启动覆盖）。停用/卸载不删 schema 与 run_states 历史数据。
package services

import (
	"fmt"
	"time"

	"control-panel/internal/domain/extension"
	"control-panel/internal/extensionmanifest"

	"gorm.io/gorm"
)

// builtinPersonaPackExtensionVersion 是内置人物能力扩展的初始版本。
const builtinPersonaPackExtensionVersion = "1.0.0"

// builtinPersonaPack 描述一个待种子的内置人物能力扩展。
type builtinPersonaPack struct {
	Pack        string // 能力包标识（PersonaCapabilityGate 的 pack 参数）
	Name        string // 内置扩展名（DNS 式）
	DisplayName string // 通用展示名（不得含游戏垂直词）
	Description string
}

// builtinPersonaPacks 是四个人物能力的内置扩展清单（顺序稳定，便于测试）。
var builtinPersonaPacks = []builtinPersonaPack{
	{PersonaPackEmotion, "io.zerone.emotion", "情绪状态", "内置人物能力：记录并叙述 Agent 的情绪状态，供提示词与工作流钩子使用。"},
	{PersonaPackBelief, "io.zerone.belief", "认知立场", "内置人物能力：管理 Agent 对事实的认知立场与看法声明。"},
	{PersonaPackMemory, "io.zerone.memory", "主观记忆", "内置人物能力：按 Agent 主观解释记录与检索近期记忆，注入提示词。"},
	{PersonaPackRelationshipDynamics, "io.zerone.relationship-dynamics", "动态关系", "内置人物能力：维护 Agent 之间的定向态度与动态关系影响。"},
}

// builtinPersonaPackManifest 生成能通过 extensionmanifest 严格校验的最小
// manifest：不贡献 UI 插槽与 apiRoutes，仅声明 state 读写权限（白名单内）。
func builtinPersonaPackManifest(pack builtinPersonaPack) string {
	return fmt.Sprintf(`{
  "apiVersion": "agenthub.extension/v1alpha1",
  "name": %q,
  "version": %q,
  "displayName": %q,
  "description": %q,
  "publisher": "zerone",
  "permissions": [{"permission": "state", "scope": "persona", "actions": ["read", "write"]}],
  "ui": {"slots": []},
  "apiRoutes": []
}`, pack.Name, builtinPersonaPackExtensionVersion, pack.DisplayName, pack.Description)
}

// EnsureBuiltinPersonaPacks 在 default 平台级租户内为四个人物能力各确保
// 一条扩展记录 + 一个 1.0.0 版本 + 一条 enabled 安装记录（尚不存在时）。
// 幂等，可重复调用；authz 接入后授权同步与种子同事务。
func (s *ExtensionLifecycleService) EnsureBuiltinPersonaPacks() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, pack := range builtinPersonaPacks {
			if err := s.ensureBuiltinPersonaPack(tx, pack); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ExtensionLifecycleService) ensureBuiltinPersonaPack(tx *gorm.DB, pack builtinPersonaPack) error {
	tenantID := "default"
	raw := builtinPersonaPackManifest(pack)
	manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(raw))
	if len(errs) > 0 {
		return fmt.Errorf("内置扩展 %s manifest 校验失败：%v", pack.Name, errs)
	}

	var ext extension.Extension
	err := tx.Where("tenant_id=? AND name=?", tenantID, pack.Name).First(&ext).Error
	if err == gorm.ErrRecordNotFound {
		ext = extension.Extension{
			TenantID:    tenantID,
			Name:        pack.Name,
			DisplayName: manifest.DisplayName,
			Description: manifest.Description,
			Source:      extension.SourceSeed,
			Status:      extension.StatusActive,
		}
		if err := tx.Create(&ext).Error; err != nil {
			if !isDuplicate(err) {
				return fmt.Errorf("创建内置扩展 %s 失败：%w", pack.Name, err)
			}
			if err := tx.Where("tenant_id=? AND name=?", tenantID, pack.Name).First(&ext).Error; err != nil {
				return fmt.Errorf("读取内置扩展 %s 失败：%w", pack.Name, err)
			}
		}
	} else if err != nil {
		return fmt.Errorf("查询内置扩展 %s 失败：%w", pack.Name, err)
	}

	var ver extension.Version
	err = tx.Where("extension_id=? AND version=?", ext.ID, builtinPersonaPackExtensionVersion).First(&ver).Error
	if err == gorm.ErrRecordNotFound {
		hash, hashErr := extensionmanifest.CanonicalManifestHash([]byte(raw))
		if hashErr != nil {
			return hashErr
		}
		ver = extension.Version{
			ExtensionID: ext.ID,
			Version:     builtinPersonaPackExtensionVersion,
			Manifest:    raw,
			ContentHash: hash,
			Changelog:   "内置人物能力扩展初始版本",
			CreatedBy:   "system",
		}
		if err := tx.Create(&ver).Error; err != nil {
			if !isDuplicate(err) {
				return fmt.Errorf("创建内置扩展 %s 版本失败：%w", pack.Name, err)
			}
			if err := tx.Where("extension_id=? AND version=?", ext.ID, builtinPersonaPackExtensionVersion).First(&ver).Error; err != nil {
				return fmt.Errorf("读取内置扩展 %s 版本失败：%w", pack.Name, err)
			}
		}
	} else if err != nil {
		return fmt.Errorf("查询内置扩展 %s 版本失败：%w", pack.Name, err)
	}

	inst, err := findInstall(tx, tenantID, ext.ID)
	if err != nil {
		return err
	}
	if inst == nil {
		newInst := extension.Install{
			TenantID:    tenantID,
			ExtensionID: ext.ID,
			Version:     ver.Version,
			Status:      extension.InstallStatusEnabled,
			InstalledBy: "system",
			InstalledAt: time.Now(),
		}
		if err := tx.Create(&newInst).Error; err != nil {
			if !isDuplicate(err) {
				return fmt.Errorf("创建内置扩展 %s 安装记录失败：%w", pack.Name, err)
			}
		}
	}

	// H7.4：接入授权服务时，权限声明同步与种子同事务（幂等，失败回滚）。
	return s.syncGrantsWithTx(tx, tenantID, &ext, &ver, "system")
}
