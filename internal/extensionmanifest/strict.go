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
	"net/url"
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
	APIRoutes        []ManifestAPIRoute    `json:"apiRoutes,omitempty"` // H7.2 授权 API 代理
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

// ManifestUI 声明 UI 贡献插槽（H7.0 校验白名单；H7.2 起支持富声明）。
//
// Slots 的每个元素兼容两种形态：
//   - 字符串（H7.0 形态）：仅声明占用某个插槽；
//   - 对象（H7.2 形态）：声明式组件挂载 {slot, component, title, order,
//     visible, data, dataSource}。两种形态可在同一 manifest 混用。
type ManifestUI struct {
	Slots []ManifestUISlot `json:"slots,omitempty"`
}

// UIComponentTypes 是声明式渲染协议允许的组件类型枚举（H7.2 安全边界：
// 扩展不执行任意代码，前端只实现这四种静态组件）。
var UIComponentTypes = []string{"stat-card", "link-list", "key-value", "markdown"}

// ManifestUISlot 是一条插槽挂载声明。
type ManifestUISlot struct {
	Slot     string                 `json:"slot"`           // 插槽名，必须在 UISlots 白名单内
	Component string                `json:"component"`      // 组件类型，必须在 UIComponentTypes 内
	Title    string                 `json:"title,omitempty"`
	Order    int                    `json:"order,omitempty"`  // 同插槽内排序，小的在前
	Visible  *bool                  `json:"visible,omitempty"` // 默认 true；false 即默认隐藏
	Data     map[string]any         `json:"data,omitempty"`    // 组件静态数据
	// DataSource 声明动态数据：只允许 GET 扩展自己的 admin 授权 API 端点。
	DataSource *ManifestUIDataSource `json:"dataSource,omitempty"`
}

// ManifestUIDataSource 声明组件的数据来源。
type ManifestUIDataSource struct {
	Path string `json:"path"` // 必须是 manifest apiRoutes 中声明过的路径
}

// UnmarshalJSON 兼容字符串形态（H7.0）与对象形态（H7.2）。
func (s *ManifestUISlot) UnmarshalJSON(raw []byte) error {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var slot string
		if err := json.Unmarshal(raw, &slot); err != nil {
			return err
		}
		*s = ManifestUISlot{Slot: slot}
		return nil
	}
	type alias ManifestUISlot
	var a alias
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	*s = ManifestUISlot(a)
	return nil
}

// MarshalJSON 保持序列化兼容（字符串形态输出字符串）。
func (s ManifestUISlot) MarshalJSON() ([]byte, error) {
	if s.Component == "" && s.Title == "" && s.Data == nil && s.DataSource == nil && s.Order == 0 && s.Visible == nil {
		return json.Marshal(s.Slot)
	}
	type alias ManifestUISlot
	return json.Marshal(alias(s))
}

// SlotNames 提取纯插槽名列表（H7.0 摘要展示兼容）。
func (ui *ManifestUI) SlotNames() []string {
	if ui == nil {
		return nil
	}
	names := make([]string, 0, len(ui.Slots))
	for _, s := range ui.Slots {
		names = append(names, s.Slot)
	}
	return names
}

// ManifestAPIRoute 声明扩展的一条授权 API 端点（H7.2 代理目标）。
// method 仅允许 GET；path 必须以 /api/v1/extensions/{扩展名}/ 开头；
// upstream 仅允许 http(s) 且在声明时完成连通性校验（接线时由主 Agent
// 在注册流程中调用 ValidateAPIRouteUpstream）。
type ManifestAPIRoute struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Upstream string `json:"upstream"`
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
			if !containsString(UISlots, slot.Slot) {
				add("ui.slots[%d].slot %q 不是合法插槽（允许：%s）", i, slot.Slot, strings.Join(UISlots, "/"))
			}
			if slot.Slot != "" && seen[slot.Slot+"\x00"+slot.Component+"\x00"+slot.Title] {
				if slot.Component == "" {
					add("ui.slots 存在重复插槽 %q", slot.Slot)
				} else {
					add("ui.slots 存在重复挂载声明（slot=%q component=%q title=%q）", slot.Slot, slot.Component, slot.Title)
				}
			}
			seen[slot.Slot+"\x00"+slot.Component+"\x00"+slot.Title] = true
			if slot.Component == "" {
				continue // 纯字符串形态：只声明占用插槽
			}
			// ---- H7.2 富声明校验 ----
			if !containsString(UIComponentTypes, slot.Component) {
				add("ui.slots[%d].component %q 不是合法组件类型（允许：%s）", i, slot.Component, strings.Join(UIComponentTypes, "/"))
			}
			if strings.TrimSpace(slot.Title) == "" || len(slot.Title) > 160 {
				add("ui.slots[%d].title 必填且不超过 160 字符", i)
			}
			if slot.Data != nil && slot.DataSource != nil {
				add("ui.slots[%d] data 与 dataSource 只能二选一", i)
			}
			if slot.DataSource != nil {
				dsPath := strings.TrimSpace(slot.DataSource.Path)
				if !m.apiRouteDeclared(dsPath) {
					add("ui.slots[%d].dataSource.path %q 必须在 manifest apiRoutes 中声明", i, dsPath)
				}
			}
		}
	}

	for i, route := range m.APIRoutes {
		if strings.ToUpper(strings.TrimSpace(route.Method)) != "GET" {
			add("apiRoutes[%d].method %q 不合法：扩展授权 API 仅允许 GET", i, route.Method)
		}
		if !strings.HasPrefix(route.Path, "/api/v1/extensions/"+m.Name+"/") {
			add("apiRoutes[%d].path %q 必须以 /api/v1/extensions/%s/ 开头", i, route.Path, m.Name)
		}
		up, err := url.ParseRequestURI(strings.TrimSpace(route.Upstream))
		if err != nil || (up.Scheme != "http" && up.Scheme != "https") || up.Host == "" {
			add("apiRoutes[%d].upstream %q 必须是合法的 http(s) URL", i, route.Upstream)
		} else if err := ValidateUpstreamIPLiteral(up.Hostname()); err != nil {
			// IP 字面量当场拒绝内网/环回/链路本地；域名在代理转发前重新解析校验
			add("apiRoutes[%d].upstream %q 被拒绝：%v", i, route.Upstream, err)
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

// apiRouteDeclared 报告 path 是否已在本 manifest 的 apiRoutes 中声明（GET）。
func (m *Manifest) apiRouteDeclared(path string) bool {
	for _, r := range m.APIRoutes {
		if r.Path == path && strings.ToUpper(strings.TrimSpace(r.Method)) == "GET" {
			return true
		}
	}
	return false
}
