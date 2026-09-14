// ExtensionLifecycleService 是 H7.1 扩展生命周期的应用服务：
// 安装（依赖解析 + 状态 Schema 注册 + 单事务）、启停、升级（JSON Patch
// 风格迁移）、回滚（按记录的旧值推导 inverse）、卸载（依赖影响检查 +
// force 级联停用）与影响范围预览。全部按租户隔离，中文错误；
// 任一步失败由数据库事务整体回滚，不产生半持久化残留。
//
// 历史数据可读性硬约束：停用/卸载/回滚均不删除 run_states、
// run_state_changes 与旧版本 extension_versions。
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/extensionmanifest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gorm.io/gorm"
)

// installLocks 按 (tenant, extension) 串行化安装/升级/回滚/卸载，
// 避免并发事务在唯一索引上互相回滚（应用层悲观锁，跨进程部署时
// 唯一索引仍是最终防线）。
var installLocks sync.Map

func lockInstall(tenantID string, extID uint64) func() {
	key := tenantID + "/" + strconv.FormatUint(extID, 10)
	mu, _ := installLocks.LoadOrStore(key, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// LifecycleError 是生命周期操作的业务错误，携带建议的 HTTP 状态码与中文说明。
type LifecycleError struct {
	Code    int
	Message string
}

func (e *LifecycleError) Error() string { return e.Message }

func lifecycleErrorf(code int, format string, args ...any) *LifecycleError {
	return &LifecycleError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsLifecycleError 判断 err 是否为生命周期业务错误并返回其状态码。
func IsLifecycleError(err error) (int, bool) {
	var le *LifecycleError
	if errors.As(err, &le) {
		return le.Code, true
	}
	return 0, false
}

type ExtensionLifecycleService struct {
	db *gorm.DB
	// authz 为 H7.4 授权同步钩子：非 nil 时安装/升级/回滚后同步
	// manifest 权限声明为授权行，启停联动 is_active，卸载保留审计
	// （purge 才删除）。nil 时行为与 H7.1 完全一致（测试基座可不接）。
	authz *ExtensionAuthzService
}

func NewExtensionLifecycleService(db *gorm.DB) *ExtensionLifecycleService {
	return &ExtensionLifecycleService{db: db}
}

// SetAuthzService 接入 H7.4 授权服务（由 cmd/server/main.go 接线时调用）。
func (s *ExtensionLifecycleService) SetAuthzService(authz *ExtensionAuthzService) {
	s.authz = authz
}

// syncGrants 把指定版本 manifest 的权限声明同步为授权行（best-effort
// 返回错误，由调用方决定失败策略；生命周期主体已提交，授权同步失败
// 不影响安装结果，但会在返回值中暴露）。
func (s *ExtensionLifecycleService) syncGrants(tenantID string, ext *extension.Extension, ver *extension.Version, grantedBy string) error {
	if s.authz == nil {
		return nil
	}
	return s.authz.SyncGrantsFromManifest(tenantID, ext.Name, ver.Manifest, ver.Version, grantedBy)
}

// InstallResult 是安装/升级/回滚的统一结果。
type InstallResult struct {
	Install            *extension.Install `json:"install"`
	Idempotent         bool               `json:"idempotent"`                   // 同版本重复安装时置 true
	DependentsDisabled []string           `json:"dependentsDisabled,omitempty"` // force 卸载时级联停用的依赖方
}

// normalizedTenant 与 ExtensionService 的空租户归一规则保持一致。
func normalizedTenant(tenantID string) string {
	t := strings.TrimSpace(tenantID)
	if t == "" {
		return "default"
	}
	return t
}

func (s *ExtensionLifecycleService) loadExtension(tenantID string, id uint64) (*extension.Extension, error) {
	var ext extension.Extension
	err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lifecycleErrorf(404, "扩展不存在")
	}
	if err != nil {
		return nil, err
	}
	return &ext, nil
}

func (s *ExtensionLifecycleService) loadVersion(extID uint64, version string) (*extension.Version, error) {
	var ver extension.Version
	err := s.db.Where("extension_id=? AND version=?", extID, version).First(&ver).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lifecycleErrorf(404, "扩展版本 %s 不存在", version)
	}
	if err != nil {
		return nil, err
	}
	return &ver, nil
}

func parseManifestOr400(raw string) (*extensionmanifest.Manifest, error) {
	manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(raw))
	if len(errs) > 0 {
		return nil, lifecycleErrorf(400, "manifest 校验失败：%s", strings.Join(errs, "；"))
	}
	return manifest, nil
}

// resolveDependencies 检查 manifest 声明的依赖：依赖扩展必须在本租户或
// default 平台级租户已启用且版本满足范围。缺失时返回 409 错误，
// 列出缺什么（扩展名 + 需要的版本范围 + 实际状态）。
func (s *ExtensionLifecycleService) resolveDependencies(tx *gorm.DB, tenantID string, manifest *extensionmanifest.Manifest) error {
	var missing []string
	for _, dep := range manifest.Dependencies {
		satisfied, actual := s.dependencySatisfied(tx, tenantID, dep.Name, dep.Version)
		if satisfied {
			continue
		}
		if dep.Optional && actual == "" {
			continue // 可选依赖：完全缺失时容忍，仅未启用/版本不满足仍报错
		}
		missing = append(missing, fmt.Sprintf("%s@%s（%s）", dep.Name, dep.Version, actual))
	}
	if len(missing) > 0 {
		return lifecycleErrorf(409, "依赖未满足：%s。请先在当前租户或 default 租户安装并启用对应扩展", strings.Join(missing, "、 "))
	}
	return nil
}

// dependencySatisfied 返回依赖是否满足；actual 描述实际安装状态（空串表示未安装）。
func (s *ExtensionLifecycleService) dependencySatisfied(tx *gorm.DB, tenantID, depName, depRange string) (bool, string) {
	for _, t := range []string{tenantID, "default"} {
		var ext extension.Extension
		if err := tx.Where("tenant_id=? AND name=?", t, depName).First(&ext).Error; err != nil {
			continue
		}
		var inst extension.Install
		err := tx.Where("tenant_id=? AND extension_id=? AND status=?", t, ext.ID, extension.InstallStatusEnabled).
			First(&inst).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Sprintf("扩展 %s 已注册但未启用", depName)
		}
		if err != nil {
			return false, err.Error()
		}
		ok, err := extension.SatisfiesRange(inst.Version, depRange)
		if err != nil {
			return false, fmt.Sprintf("版本范围 %q 无法解析", depRange)
		}
		if !ok {
			return false, fmt.Sprintf("已启用版本 %s，不满足范围 %s", inst.Version, depRange)
		}
		return true, fmt.Sprintf("已启用 %s@%s", depName, inst.Version)
	}
	return false, ""
}

// registerSchemas 在事务内注册 manifest 声明的状态 Schema：
// namespace=扩展名、name=声明名、version=扩展版本；JSON Schema 编译失败
// 即报错（安装/升级整体回滚）。同租户同版本重复注册视为幂等跳过
// （升级重装同版本、依赖共享 Schema 等场景）。
func registerSchemas(tx *gorm.DB, tenantID string, ext *extension.Extension, ver *extension.Version, manifest *extensionmanifest.Manifest) error {
	for _, decl := range manifest.StateSchemas {
		schema := decl.Payload
		if len(schema) == 0 {
			continue // 占位声明（无 payload）不注册状态 Schema
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("schema.json", schema); err != nil {
			return lifecycleErrorf(400, "stateSchemas[%s] 不是合法 JSON Schema：%v", decl.Name, err)
		}
		if _, err := compiler.Compile("schema.json"); err != nil {
			return lifecycleErrorf(400, "stateSchemas[%s] 不是合法 JSON Schema：%v", decl.Name, err)
		}
		raw, err := json.Marshal(schema)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		row := rundomain.StateSchema{
			TenantID:     tenantID,
			Namespace:    ext.Name,
			Name:         decl.Name,
			Version:      ver.Version,
			Schema:       schema,
			ScopeTypes:   []string{"run"},
			SubjectTypes: []string{"agent"},
			ContentHash:  hex.EncodeToString(sum[:]),
		}
		if err := tx.Create(&row).Error; err != nil {
			if isDuplicate(err) {
				continue // 幂等：同租户同版本同命名空间已注册
			}
			return fmt.Errorf("注册状态 Schema %s 失败：%w", decl.Name, err)
		}
	}
	return nil
}

// findInstall 返回租户内安装记录；未安装时返回 (nil, nil)。
func findInstall(tx *gorm.DB, tenantID string, extID uint64) (*extension.Install, error) {
	var inst extension.Install
	err := tx.Where("tenant_id=? AND extension_id=?", tenantID, extID).First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

// Install 安装扩展指定版本：manifest 严格校验 → 依赖解析 → 注册状态
// Schema → 写 installs 行，全部在一个事务内。重复安装同版本幂等返回；
// 已安装其他版本时提示走升级路径；并发安装靠唯一索引保证只有一个成功。
func (s *ExtensionLifecycleService) Install(tenantID string, extID uint64, version string, installedBy string) (*InstallResult, error) {
	tenantID = normalizedTenant(tenantID)
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, lifecycleErrorf(400, "version 必填：请指定要安装的扩展版本")
	}
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	ver, err := s.loadVersion(ext.ID, version)
	if err != nil {
		return nil, err
	}
	manifest, err := parseManifestOr400(ver.Manifest)
	if err != nil {
		return nil, err
	}

	// 幂等/升级分流：在事务外先读一次，命中唯一索引冲突后事务内再兜底。
	// 同一 (tenant, extension) 的安装走进程内互斥串行化：既避免唯一索引
	// 冲突后的额外回滚，也让"并发安装只有一个写入、其余幂等返回"的语义
	// 在 SQLite 等单写库上依然成立。
	unlock := lockInstall(tenantID, ext.ID)
	defer unlock()
	created := false
	var existing *extension.Install
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		existing, err = findInstall(tx, tenantID, ext.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Version == ver.Version {
				return nil // 幂等
			}
			return lifecycleErrorf(409, "扩展 %s 已安装版本 %s：安装不同版本请使用升级接口", ext.Name, existing.Version)
		}
		if err := s.resolveDependencies(tx, tenantID, manifest); err != nil {
			return err
		}
		if err := registerSchemas(tx, tenantID, ext, ver, manifest); err != nil {
			return err
		}
		inst := extension.Install{
			TenantID:    tenantID,
			ExtensionID: ext.ID,
			Version:     ver.Version,
			Status:      extension.InstallStatusEnabled,
			InstalledBy: strings.TrimSpace(installedBy),
			InstalledAt: time.Now(),
		}
		if err := tx.Create(&inst).Error; err != nil {
			if isDuplicate(err) {
				// 并发安装：另一个事务先写入。转为读回既有记录（幂等语义）。
				existing, err = findInstall(tx, tenantID, ext.ID)
				return err
			}
			return fmt.Errorf("写入安装记录失败：%w", err)
		}
		existing = &inst
		created = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.syncGrants(tenantID, ext, ver, existing.InstalledBy); err != nil {
		return nil, err
	}
	return &InstallResult{Install: existing, Idempotent: !created}, nil
}

// setInstallStatus 是启停的公共实现：只改 installs.status，
// 不动历史数据（run_states 等旧数据依旧可读），停用仅影响新事件生效。
func (s *ExtensionLifecycleService) setInstallStatus(tenantID string, extID uint64, status string) (*extension.Install, error) {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, lifecycleErrorf(404, "扩展 %s 尚未安装，无法%s", ext.Name, map[string]string{
			extension.InstallStatusEnabled:  "启用",
			extension.InstallStatusDisabled: "停用",
		}[status])
	}
	if inst.Status == status {
		return inst, nil // 幂等：状态未变化
	}
	inst.Status = status
	if err := s.db.Save(inst).Error; err != nil {
		return nil, err
	}
	// H7.4：启停联动授权生效状态（停用保留行与审计）
	if s.authz != nil {
		if err := s.authz.SetGrantsActive(tenantID, ext.Name, status == extension.InstallStatusEnabled); err != nil {
			return nil, err
		}
	}
	return inst, nil
}

// Enable 启用已安装的扩展（幂等）。
func (s *ExtensionLifecycleService) Enable(tenantID string, extID uint64) (*extension.Install, error) {
	return s.setInstallStatus(tenantID, extID, extension.InstallStatusEnabled)
}

// Disable 停用扩展：新事件不再生效（GetEnabled 语义返回未启用），
// 历史 run_states / run_state_changes 数据保持不变、依旧可读。
func (s *ExtensionLifecycleService) Disable(tenantID string, extID uint64) (*extension.Install, error) {
	return s.setInstallStatus(tenantID, extID, extension.InstallStatusDisabled)
}

// Upgrade 把已安装扩展升级到目标版本：目标 manifest 校验 → 依赖检查 →
// 执行 from=当前版本 的迁移（JSON Patch 子集，应用到本扩展 namespace 的
// state_schemas.schema_json）→ 更新 installs 行并追加 migration_log。
// 全部在事务内，任一步失败整体回滚。
func (s *ExtensionLifecycleService) Upgrade(tenantID string, extID uint64, targetVersion string) (*InstallResult, error) {
	tenantID = normalizedTenant(tenantID)
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return nil, lifecycleErrorf(400, "target_version 必填：请指定升级目标版本")
	}
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, lifecycleErrorf(404, "扩展 %s 尚未安装，无法升级", ext.Name)
	}
	if inst.Version == targetVersion {
		return &InstallResult{Install: inst, Idempotent: true}, nil
	}
	target, err := s.loadVersion(ext.ID, targetVersion)
	if err != nil {
		return nil, err
	}
	targetManifest, err := parseManifestOr400(target.Manifest)
	if err != nil {
		return nil, err
	}
	// 迁移声明优先取当前版本 manifest（from=当前 → to=目标），
	// 当前版本未声明时回退到目标版本 manifest。
	currentVer, err := s.loadVersion(ext.ID, inst.Version)
	if err != nil {
		return nil, err
	}
	currentManifest, err := parseManifestOr400(currentVer.Manifest)
	if err != nil {
		return nil, err
	}
	migrations := selectMigrations(currentManifest, inst.Version, targetVersion)
	if len(migrations) == 0 {
		migrations = selectMigrations(targetManifest, inst.Version, targetVersion)
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.resolveDependencies(tx, tenantID, targetManifest); err != nil {
			return err
		}
		if err := registerSchemas(tx, tenantID, ext, target, targetManifest); err != nil {
			return err
		}
		entries := make([]migrationLogEntry, 0)
		for _, mig := range migrations {
			applied, err := applyMigration(tx, tenantID, ext.Name, mig)
			if err != nil {
				return lifecycleErrorf(500, "数据迁移失败（%s → %s）：%v", mig.From, mig.To, err)
			}
			entries = append(entries, applied...)
		}
		inst.Version = targetVersion
		if err := appendMigrationLog(inst, "upgrade", inst.Version, targetVersion, entries); err != nil {
			return err
		}
		return tx.Save(inst).Error
	})
	if err != nil {
		return nil, err
	}
	if err := s.syncGrants(tenantID, ext, target, inst.InstalledBy); err != nil {
		return nil, err
	}
	return &InstallResult{Install: inst}, nil
}

// Rollback 把已安装扩展回滚到目标版本（空则自动取当前版本的前一版本）：
// 优先执行迁移声明里的 rollback 段，否则按 migration_log 中记录的
// 旧值对迁移逆序执行 inverse replace，恢复 schema_json；然后 installs
// 指向目标版本并追加回滚记录。事务内执行，失败整体回滚。
func (s *ExtensionLifecycleService) Rollback(tenantID string, extID uint64, targetVersion string) (*InstallResult, error) {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, lifecycleErrorf(404, "扩展 %s 尚未安装，无法回滚", ext.Name)
	}
	if strings.TrimSpace(targetVersion) == "" {
		targetVersion, err = s.previousVersion(ext.ID, inst.Version)
		if err != nil {
			return nil, err
		}
	}
	if inst.Version == targetVersion {
		return &InstallResult{Install: inst, Idempotent: true}, nil
	}
	// 只允许回滚到低于当前版本的已注册版本
	cmp, err := extension.CompareVersions(targetVersion, inst.Version)
	if err != nil || cmp >= 0 {
		return nil, lifecycleErrorf(400, "回滚目标版本必须是低于当前版本 %s 的已注册版本", inst.Version)
	}
	target, err := s.loadVersion(ext.ID, targetVersion)
	if err != nil {
		return nil, err
	}
	targetManifest, err := parseManifestOr400(target.Manifest)
	if err != nil {
		return nil, err
	}

	currentManifest, mErr := parseManifestOr400(func() string {
		v, e := s.loadVersion(ext.ID, inst.Version)
		if e != nil {
			return ""
		}
		return v.Manifest
	}())
	if mErr != nil {
		currentManifest = nil
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := registerSchemas(tx, tenantID, ext, target, targetManifest); err != nil {
			return err
		}
		var entries []migrationLogEntry
		if currentManifest != nil {
			for _, mig := range selectMigrations(currentManifest, targetVersion, inst.Version) {
				// 声明了 rollback 段：直接按声明逆序执行
				if len(mig.Rollback) > 0 {
					applied, err := applyMigrationOps(tx, tenantID, ext.Name, reverseOps(mig.Rollback))
					if err != nil {
						return lifecycleErrorf(500, "回滚迁移失败（%s → %s）：%v", inst.Version, targetVersion, err)
					}
					entries = append(entries, applied...)
					continue
				}
			}
		}
		if len(entries) == 0 && len(inst.MigrationLog) > 0 {
			// 按记录的旧值推导 inverse：对本区间内的 upgrade 记录逆序执行
			applied, err := applyInverseFromLog(tx, tenantID, ext.Name, inst.MigrationLog, targetVersion, inst.Version)
			if err != nil {
				return lifecycleErrorf(500, "回滚迁移失败（%s → %s）：%v", inst.Version, targetVersion, err)
			}
			entries = append(entries, applied...)
		}
		oldVersion := inst.Version
		inst.Version = targetVersion
		if err := appendMigrationLog(inst, "rollback", oldVersion, targetVersion, entries); err != nil {
			return err
		}
		return tx.Save(inst).Error
	})
	if err != nil {
		return nil, err
	}
	if err := s.syncGrants(tenantID, ext, target, inst.InstalledBy); err != nil {
		return nil, err
	}
	return &InstallResult{Install: inst}, nil
}

// previousVersion 返回低于 current 的最高已注册版本（回滚默认目标）。
func (s *ExtensionLifecycleService) previousVersion(extID uint64, current string) (string, error) {
	var versions []extension.Version
	if err := s.db.Where("extension_id=?", extID).Find(&versions).Error; err != nil {
		return "", err
	}
	best := ""
	for _, v := range versions {
		if cmp, err := extension.CompareVersions(v.Version, current); err != nil || cmp >= 0 {
			continue
		}
		if best == "" {
			best = v.Version
			continue
		}
		if cmp, err := extension.CompareVersions(v.Version, best); err == nil && cmp > 0 {
			best = v.Version
		}
	}
	if best == "" {
		return "", lifecycleErrorf(404, "没有可回滚到的更低版本")
	}
	return best, nil
}

// dependentsOf 返回当前租户内依赖指定扩展名、且处于启用状态的安装记录
// 对应的扩展名列表（卸载影响范围检查用）。
func (s *ExtensionLifecycleService) dependentsOf(tenantID, depName string) ([]string, error) {
	var installs []extension.Install
	if err := s.db.Where("tenant_id=? AND status=?", tenantID, extension.InstallStatusEnabled).Find(&installs).Error; err != nil {
		return nil, err
	}
	var names []string
	for _, inst := range installs {
		var ext extension.Extension
		if err := s.db.Where("id=?", inst.ExtensionID).First(&ext).Error; err != nil {
			return nil, err
		}
		manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(func() string {
			var ver extension.Version
			if err := s.db.Where("extension_id=? AND version=?", ext.ID, inst.Version).First(&ver).Error; err != nil {
				return ""
			}
			return ver.Manifest
		}()))
		if len(errs) > 0 || manifest == nil {
			continue
		}
		for _, dep := range manifest.Dependencies {
			if dep.Name == depName {
				names = append(names, ext.Name)
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

// Uninstall 卸载扩展：先检查依赖影响范围——有其他已安装扩展依赖它时
// 默认 409 列出依赖方；force=true 时先自动停用依赖方再卸载。
// 卸载只删除 installs 行（purge=true 时额外删除本租户版本数据），
// run_states / run_state_changes 历史数据一律保留、依旧可读。
func (s *ExtensionLifecycleService) Uninstall(tenantID string, extID uint64, force, purge bool) (*InstallResult, error) {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, lifecycleErrorf(404, "扩展 %s 尚未安装，无需卸载", ext.Name)
	}
	dependents, err := s.dependentsOf(tenantID, ext.Name)
	if err != nil {
		return nil, err
	}
	result := &InstallResult{}
	if len(dependents) > 0 && !force {
		return nil, lifecycleErrorf(409, "无法卸载：以下扩展依赖 %s：%s。确认后可使用 force=true 自动停用依赖方后卸载",
			ext.Name, strings.Join(dependents, "、"))
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if len(dependents) > 0 {
			// force：级联停用依赖方（只停用新事件生效，不动历史数据）
			var depInstalls []extension.Install
			if err := tx.Where("tenant_id=? AND status=?", tenantID, extension.InstallStatusEnabled).Find(&depInstalls).Error; err != nil {
				return err
			}
			for _, di := range depInstalls {
				var depExt extension.Extension
				if err := tx.Where("id=?", di.ExtensionID).First(&depExt).Error; err != nil {
					return err
				}
				skip := false
				for _, name := range dependents {
					if depExt.Name == name {
						skip = true
						break
					}
				}
				if !skip {
					continue
				}
				if err := tx.Model(&extension.Install{}).Where("id=?", di.ID).Update("status", extension.InstallStatusDisabled).Error; err != nil {
					return err
				}
				result.DependentsDisabled = append(result.DependentsDisabled, depExt.Name)
			}
		}
		if err := tx.Where("tenant_id=? AND extension_id=?", tenantID, ext.ID).Delete(&extension.Install{}).Error; err != nil {
			return err
		}
		if purge {
			// purge 只清本租户本扩展的版本数据；扩展行一并删除，
			// 其他租户与 run_states 历史数据不受影响。
			if err := tx.Where("extension_id=?", ext.ID).Delete(&extension.Version{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id=?", ext.ID).Delete(&extension.Extension{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// H7.4：卸载（非 purge）保留授权行与审计，仅置失效；purge 才物理删除。
	if s.authz != nil {
		if purge {
			if err := s.authz.DeleteGrantsForExtension(tenantID, ext.Name); err != nil {
				return nil, err
			}
		} else {
			if err := s.authz.SetGrantsActive(tenantID, ext.Name, false); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

// DependencyImpact 描述影响范围预览中"它依赖谁"的一项。
type DependencyImpact struct {
	Name      string `json:"name"`
	Range     string `json:"range"`
	Optional  bool   `json:"optional"`
	Satisfied bool   `json:"satisfied"`
	Actual    string `json:"actual,omitempty"` // 实际安装状态描述
}

// ExtensionImpact 是卸载/安装前的依赖影响范围预览：
// 谁依赖它、它依赖谁、当前安装版本会新增哪些 stateSchemas 与权限。
type ExtensionImpact struct {
	ExtensionID     uint64              `json:"extensionId"`
	Name            string              `json:"name"`
	Version         string              `json:"version,omitempty"` // 当前已安装版本（未安装为空）
	Installed       bool                `json:"installed"`
	Status          string              `json:"status,omitempty"`
	Dependents      []string            `json:"dependents"`   // 谁依赖它（本租户已启用）
	Dependencies    []DependencyImpact  `json:"dependencies"` // 它依赖谁
	NewStateSchemas []string            `json:"newStateSchemas"`
	Permissions     []PermissionSummary `json:"permissions"`
}

// Impact 计算扩展的依赖影响范围：卸载前必显的预览数据。
func (s *ExtensionLifecycleService) Impact(tenantID string, extID uint64) (*ExtensionImpact, error) {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	inst, err := findInstall(s.db, tenantID, ext.ID)
	if err != nil {
		return nil, err
	}
	impact := &ExtensionImpact{ExtensionID: ext.ID, Name: ext.Name}
	if inst != nil {
		impact.Installed = true
		impact.Version = inst.Version
		impact.Status = inst.Status
	}
	dependents, err := s.dependentsOf(tenantID, ext.Name)
	if err != nil {
		return nil, err
	}
	impact.Dependents = dependents
	if len(impact.Dependents) == 0 {
		impact.Dependents = []string{}
	}

	// 用已安装版本（未安装则用最新版本）的 manifest 计算"它依赖谁"与新增贡献
	version := impact.Version
	if version == "" {
		var latest extension.Version
		if err := s.db.Where("extension_id=?", ext.ID).Order("created_at DESC").First(&latest).Error; err == nil {
			version = latest.Version
		}
	}
	if version != "" {
		ver, err := s.loadVersion(ext.ID, version)
		if err != nil {
			return nil, err
		}
		manifest, mErr := parseManifestOr400(ver.Manifest)
		if mErr == nil && manifest != nil {
			for _, dep := range manifest.Dependencies {
				satisfied, actual := s.dependencySatisfied(s.db, tenantID, dep.Name, dep.Version)
				impact.Dependencies = append(impact.Dependencies, DependencyImpact{
					Name: dep.Name, Range: dep.Version, Optional: dep.Optional,
					Satisfied: satisfied, Actual: actual,
				})
			}
			registered := map[string]bool{}
			var schemas []rundomain.StateSchema
			_ = s.db.Where("tenant_id=? AND namespace=?", tenantID, ext.Name).Find(&schemas).Error
			for _, sch := range schemas {
				registered[sch.Name+":"+sch.Version] = true
			}
			for _, decl := range manifest.StateSchemas {
				if !registered[decl.Name+":"+version] {
					impact.NewStateSchemas = append(impact.NewStateSchemas, decl.Name+"@"+version)
				}
			}
			impact.Permissions = summarizePermissions(ver.Manifest)
		}
	}
	if impact.Dependencies == nil {
		impact.Dependencies = []DependencyImpact{}
	}
	if impact.NewStateSchemas == nil {
		impact.NewStateSchemas = []string{}
	}
	return impact, nil
}
