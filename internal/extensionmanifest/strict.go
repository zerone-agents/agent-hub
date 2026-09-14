// 扩展注册中心 manifest 的严格模式校验器（H7.0）。
//
// 与面向 H6 能力包的 ValidateManifest 不同，本文件校验的是 H7 产品化扩展
// manifest（JSON 形式，POST /admin/extensions 请求体）：apiVersion 固定
// agenthub.extension/v1alpha1，名称 DNS 式，版本 semver，依赖版本范围合法，
// 权限声明受十类白名单约束，UI 插槽受七类白名单约束，声明类贡献
// （stateSchemas/events/tools/relations/promptInjections）结构合法。
// 所有错误信息使用中文。既有的 ValidateManifest/ValidatePackage 保持不变。
package extensionmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"control-panel/internal/domain/extension"
)

// 扩展 manifest 协议标识（沿用 v1alpha1 协议，见 docs/h7-decisions.md D1）。
const ExtensionAPIVersion = "agenthub.extension/v1alpha1"

// PermissionScopes 是权限声明白名单（H7.0 十类）。
var PermissionScopes = []string{
	"agent", "state", "message", "model", "tool",
	"event", "network", "storage", "ui", "group", "workflow",
}

// UISlots 是 UI 插槽白名单（H7.0 七类，H7.2 实现渲染）。
var UISlots = []string{
	"sidebar",
	"dashboard.card",
	"agent.detail.tab",
	"run.detail.tab",
	"group.detail.tab",
	"workflow.detail.tab",
	"settings.section",
}

var (
	dnsNamePattern       = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9-]*)+$`)
	identifierPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	comparatorPattern    = regexp.MustCompile(`^(>=|<=|>|<|=|\^|~)?\s*(0|[1-9][0-9]*\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)
	versionNumberPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

// Manifest 是严格模式下的 H7 扩展 manifest 结构。
type Manifest struct {
	APIVersion       string                `json:"apiVersion"`
	Name             string                `json:"name"`
	Version          string                `json:"version"`
	DisplayName      string                `json:"displayName"`
	Description      string                `json:"description"`
	Icon             string                `json:"icon,omitempty"`
	Publisher        string                `json:"publisher,omitempty"`
	Dependencies     []ManifestDependency  `json:"dependencies,omitempty"`
	Permissions      []ManifestPermission  `json:"permissions,omitempty"`
	UI               *ManifestUI           `json:"ui,omitempty"`
	StateSchemas     []ManifestDeclaration `json:"stateSchemas,omitempty"`
	Events           []ManifestDeclaration `json:"events,omitempty"`
	Tools            []ManifestDeclaration `json:"tools,omitempty"`
	Relations        []ManifestDeclaration `json:"relations,omitempty"`
	PromptInjections []ManifestDeclaration `json:"promptInjections,omitempty"`
	Migrations       []ManifestMigration   `json:"migrations,omitempty"`
}

// ManifestMigration 声明一次状态 Schema 数据变换（JSON Patch 风格），
// 如 from=1.0.0 to=2.0.0 时把 schema_json 中 /intensity/minimum 替换
// 为 -100。ops 在升级时正向执行；rollback 可选，回滚时优先于
// 按旧值推导的 inverse。每条 op 均为中文错误校验。
type ManifestMigration struct {
	From     string                `json:"from"`
	To       string                `json:"to"`
	Ops      []ManifestMigrationOp `json:"ops"`
	Rollback []ManifestMigrationOp `json:"rollback,omitempty"`
}

// ManifestMigrationOp 是单条迁移操作（JSON Patch 子集：replace/add/remove）。
type ManifestMigrationOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// ManifestDependency 声明对另一扩展的版本范围，如 ">=1.0.0 <2.0.0"。
type ManifestDependency struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Optional bool   `json:"optional,omitempty"`
}

// ManifestPermission 声明一条权限：permission 为白名单类别，
// scope 为资源命名空间，actions 为允许的操作集合。
type ManifestPermission struct {
	Permission string   `json:"permission"`
	Scope      string   `json:"scope"`
	Actions    []string `json:"actions"`
}

// ManifestUI 声明 UI 贡献插槽（H7.0 仅校验白名单，H7.2 消费）。
type ManifestUI struct {
	Slots []string `json:"slots,omitempty"`
}

// ManifestDeclaration 是声明类贡献条目的通用结构（stateSchemas 等五类）。
type ManifestDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// ValidateExtensionManifest 对 H7 扩展 manifest 做严格校验。
// 返回解析后的结构与中文错误列表；列表为空即合法。
func ValidateExtensionManifest(raw []byte) (*Manifest, []string) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, []string{fmt.Sprintf("manifest 不是合法 JSON：%v", err)}
	}
	canonical, err := json.Marshal(decoded) // map 键排序后即规范 JSON
	if err != nil {
		return nil, []string{fmt.Sprintf("manifest 无法规范化：%v", err)}
	}
	var m Manifest
	if err := json.Unmarshal(canonical, &m); err != nil {
		return nil, []string{fmt.Sprintf("manifest 结构无法解析：%v", err)}
	}
	var errs []string
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if m.APIVersion != ExtensionAPIVersion {
		add("apiVersion 必须是 %q，当前为 %q", ExtensionAPIVersion, m.APIVersion)
	}
	if !dnsNamePattern.MatchString(m.Name) {
		add("name %q 不符合 DNS 式命名（如 io.zerone.example）", m.Name)
	}
	if !extension.IsValidVersion(m.Version) {
		add("version %q 不是合法的语义化版本", m.Version)
	}
	if strings.TrimSpace(m.DisplayName) == "" || len(m.DisplayName) > 160 {
		add("displayName 必填且不超过 160 字符")
	}
	if len(m.Description) > 2000 {
		add("description 不超过 2000 字符")
	}

	for i, dep := range m.Dependencies {
		if !dnsNamePattern.MatchString(dep.Name) {
			add("dependencies[%d].name %q 不符合 DNS 式命名", i, dep.Name)
		}
		if !IsValidVersionRange(dep.Version) {
			add("dependencies[%d].version %q 不是合法的版本范围（示例：>=1.0.0 <2.0.0、^1.2.3、1.0.0）", i, dep.Version)
		}
	}

	for i, perm := range m.Permissions {
		if !containsString(PermissionScopes, perm.Permission) {
			add("permissions[%d].permission %q 不在白名单内（允许：%s）", i, perm.Permission, strings.Join(PermissionScopes, "/"))
		}
		if strings.TrimSpace(perm.Scope) == "" || len(perm.Scope) > 160 {
			add("permissions[%d].scope 必填且不超过 160 字符", i)
		}
		if len(perm.Actions) == 0 {
			add("permissions[%d].actions 至少声明一个操作", i)
			continue
		}
		seen := map[string]bool{}
		for j, action := range perm.Actions {
			if strings.TrimSpace(action) == "" || len(action) > 64 {
				add("permissions[%d].actions[%d] 不能为空且不超过 64 字符", i, j)
				continue
			}
			if seen[action] {
				add("permissions[%d].actions 存在重复操作 %q", i, action)
			}
			seen[action] = true
		}
	}

	if m.UI != nil {
		seen := map[string]bool{}
		for i, slot := range m.UI.Slots {
			if !containsString(UISlots, slot) {
				add("ui.slots[%d] %q 不是合法插槽（允许：%s）", i, slot, strings.Join(UISlots, "/"))
			}
			if seen[slot] {
				add("ui.slots 存在重复插槽 %q", slot)
			}
			seen[slot] = true
		}
	}

	validateDeclarations(add, "stateSchemas", m.StateSchemas)
	validateDeclarations(add, "events", m.Events)
	validateDeclarations(add, "tools", m.Tools)
	validateDeclarations(add, "relations", m.Relations)
	validateDeclarations(add, "promptInjections", m.PromptInjections)
	validateMigrations(add, m.Migrations)

	if len(errs) > 0 {
		return nil, errs
	}
	return &m, nil
}

// CanonicalManifestHash 计算 manifest 规范 JSON 的 sha256（内容寻址/幂等键）。
func CanonicalManifestHash(raw []byte) (string, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("manifest 不是合法 JSON：%w", err)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("manifest 无法规范化：%w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func validateDeclarations(add func(string, ...any), kind string, decls []ManifestDeclaration) {
	seen := map[string]bool{}
	for i, d := range decls {
		if !identifierPattern.MatchString(d.Name) {
			add("%s[%d].name %q 不符合命名规范（小写字母开头，可含 . _ - 与数字）", kind, i, d.Name)
		} else if seen[d.Name] {
			add("%s 存在重复声明 %q", kind, d.Name)
		}
		seen[d.Name] = true
		if len(d.Description) > 500 {
			add("%s[%d].description 不超过 500 字符", kind, i)
		}
	}
}

// IsValidVersionRange 校验依赖版本范围表达式：
// 支持精确版本、空格/逗号分隔的比较符组合（>=、<=、>、<、=）以及 ^、~ 前缀。
func IsValidVersionRange(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" {
		return false
	}
	if versionNumberPattern.MatchString(s) {
		return true
	}
	if strings.HasPrefix(s, "^") || strings.HasPrefix(s, "~") {
		return versionNumberPattern.MatchString(strings.TrimSpace(s[1:]))
	}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		if part == "" {
			continue
		}
		m := comparatorPattern.FindStringSubmatch(part)
		if m == nil {
			return false
		}
	}
	return true
}

func validateMigrationOps(add func(string, ...any), kind string, index int, ops []ManifestMigrationOp) {
	seen := map[string]bool{}
	for j, op := range ops {
		switch op.Op {
		case "replace", "add", "remove":
		default:
			add("migrations[%d].%s[%d].op %q 必须是 replace/add/remove 之一", index, kind, j, op.Op)
			continue
		}
		if !strings.HasPrefix(op.Path, "/") || len(op.Path) == 1 {
			add("migrations[%d].%s[%d].path %q 必须是以 / 开头的 JSON Pointer 路径", index, kind, j, op.Path)
			continue
		}
		if seen[op.Op+"\x00"+op.Path] {
			add("migrations[%d].%s 存在重复路径 %q", index, kind, op.Path)
		}
		seen[op.Op+"\x00"+op.Path] = true
		if op.Op == "remove" && op.Value != nil {
			add("migrations[%d].%s[%d] remove 操作不允许携带 value", index, kind, j)
		}
	}
}

func validateMigrations(add func(string, ...any), migrations []ManifestMigration) {
	seen := map[string]bool{}
	for i, mig := range migrations {
		if !extension.IsValidVersion(mig.From) {
			add("migrations[%d].from %q 不是合法的语义化版本", i, mig.From)
		}
		if !extension.IsValidVersion(mig.To) {
			add("migrations[%d].to %q 不是合法的语义化版本", i, mig.To)
		}
		if len(mig.Ops) == 0 {
			add("migrations[%d].ops 至少声明一条迁移操作", i)
		}
		key := mig.From + "->" + mig.To
		if seen[key] {
			add("migrations 存在重复的版本区间 %q", key)
		}
		seen[key] = true
		validateMigrationOps(add, "ops", i, mig.Ops)
		validateMigrationOps(add, "rollback", i, mig.Rollback)
	}
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
