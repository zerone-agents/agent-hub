// H7 P1：四个人物能力（情绪/认知立场/主观记忆/动态关系）的门控。
//
// 四种能力以内置扩展形式注册进 H7 扩展生命周期（default 租户，
// 默认已安装 + 已启用，见 ExtensionLifecycleService.EnsureBuiltinPersonaPacks），
// 管理员停用某个内置扩展后，对应能力必须真实关闭：
//   - 启动期的状态 Schema 确保（main.go 按 gate 逐包判断）；
//   - prompt composer 的 recent_memory 阶段（provider 闭包运行时查 gate）；
//   - 工作流步骤完成钩子与组织消息投递钩子的情绪/动态关系/认知立场注入；
//   - 组织 MCP 人物工具的写路径与读路径。
//
// 停用只影响"生效"，不删除任何 schema 与 run_states 历史数据；
// persona admin 只读视图不受门控（H7 语义：停用后旧数据仍可读）。
package services

import (
	"errors"

	"control-panel/internal/domain/extension"

	"gorm.io/gorm"
)

// 人物能力包标识（gate 的 pack 参数取值）。
const (
	PersonaPackEmotion              = "emotion"
	PersonaPackBelief               = "belief"
	PersonaPackMemory               = "memory"
	PersonaPackRelationshipDynamics = "relationship-dynamics"
)

// PersonaCapabilityGate 按 H7 扩展生命周期状态决定人物能力是否生效。
// 所有查询均为运行时即时查询：停用立即生效，无需重启。
type PersonaCapabilityGate struct {
	db *gorm.DB
}

func NewPersonaCapabilityGate(db *gorm.DB) *PersonaCapabilityGate {
	return &PersonaCapabilityGate{db: db}
}

// personaPackExtensionName 把能力包标识映射到内置扩展名。
func personaPackExtensionName(pack string) (string, bool) {
	switch pack {
	case PersonaPackEmotion:
		return "io.zerone.emotion", true
	case PersonaPackBelief:
		return "io.zerone.belief", true
	case PersonaPackMemory:
		return "io.zerone.memory", true
	case PersonaPackRelationshipDynamics:
		return "io.zerone.relationship-dynamics", true
	}
	return "", false
}

// Enabled 报告某租户内指定人物能力是否生效：
// 对应内置扩展的安装记录存在且 status=enabled 才返回 true；
// 记录不存在、已停用或查询出错一律返回 false（fail-closed）。
//
// 租户解析：先在给定租户（空归一为 default）内查扩展与安装记录；
// 该租户没有对应扩展时回退到 default 平台级租户。这样运行时容器
// 携带 token 调用（tenant 可能恒为 default 之外的值）仍受平台级
// 停用控制。
func (g *PersonaCapabilityGate) Enabled(tenantID, pack string) bool {
	name, ok := personaPackExtensionName(pack)
	if !ok || g == nil || g.db == nil {
		return false
	}
	tenantID = normalizedTenant(tenantID)
	tenants := []string{tenantID}
	if tenantID != "default" {
		tenants = append(tenants, "default")
	}
	for _, t := range tenants {
		enabled, found, err := g.enabledInTenant(t, name)
		if err != nil {
			return false
		}
		if found {
			return enabled
		}
	}
	return false
}

// enabledInTenant 返回某租户内该扩展的安装是否启用；found=false 表示
// 该租户没有此扩展记录（调用方应回退到 default 租户）。
func (g *PersonaCapabilityGate) enabledInTenant(tenantID, name string) (enabled bool, found bool, err error) {
	var ext extension.Extension
	err = g.db.Where("tenant_id=? AND name=?", tenantID, name).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	var inst extension.Install
	err = g.db.Where("tenant_id=? AND extension_id=?", tenantID, ext.ID).First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, true, nil // 扩展存在但未安装：能力不生效
	}
	if err != nil {
		return false, false, err
	}
	return inst.Status == extension.InstallStatusEnabled, true, nil
}

// GateRecentMemoryProvider 包装 recent_memory 阶段 Provider：主观记忆
// 能力被停用时返回 (nil, nil)，合成器据此跳过该阶段；启用时委托内部
// Provider。闭包内每次调用都即时查 gate，停用立即生效。
func (g *PersonaCapabilityGate) GateRecentMemoryProvider(inner RecentMemoryProviderFunc) RecentMemoryProviderFunc {
	return func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		if !g.Enabled(tenantID, PersonaPackMemory) {
			return nil, nil
		}
		return inner(tenantID, runID, agentID)
	}
}
