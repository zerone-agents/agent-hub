// H7.3 模板 spec 结构定义。spec 是模板版本的唯一内容体，所有段可选；
// 模板内引用一律按名称（agents[].name），安装时经 mapping（modelRef→实际
// 模型、namePrefix 前缀）解析为租户资源。模板不携带任何权限或代码语义。
package template

// Spec 是模板版本的声明式内容，所有段可选。
type Spec struct {
	Agents               []AgentSpec               `json:"agents,omitempty"`
	PersonalityTemplates []PersonalityTemplateSpec `json:"personalityTemplates,omitempty"`
	Groups               []GroupSpec               `json:"groups,omitempty"`
	Relations            []RelationSpec            `json:"relations,omitempty"`
	Workflows            []WorkflowSpec            `json:"workflows,omitempty"`
	StateSchemas         []StateSchemaSpec         `json:"stateSchemas,omitempty"`
	ExtensionDeps        []ExtensionDepSpec        `json:"extensionDeps,omitempty"`
	SampleData           []SampleDataSpec          `json:"sampleData,omitempty"`
}

// AgentSpec 声明一个 Agent 配置。ModelRef 是模板内符号引用，安装时经
// mapping 映射到租户已有模型；Tools 仅为可选的名称列表，安装不授予
// 任何工具权限（仅记录期望，实际挂载仍走平台既有流程）。
type AgentSpec struct {
	Name              string   `json:"name"`
	Title             string   `json:"title,omitempty"`
	SystemPrompt      string   `json:"systemPrompt,omitempty"`
	PersonalityPrompt string   `json:"personalityPrompt,omitempty"`
	ModelRef          string   `json:"modelRef,omitempty"`
	Tools             []string `json:"tools,omitempty"`
}

// PersonalityTemplateSpec 声明一个人格模板（name+content）。
type PersonalityTemplateSpec struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// GroupSpec 声明一个协作群组。MemberRefs 指向 agents[].name；
// Channels 为频道名称列表。
type GroupSpec struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Channels    []string `json:"channels,omitempty"`
	MemberRefs  []string `json:"memberRefs,omitempty"`
}

// RelationSpec 声明一条定向 Agent 关系（fromRef/toRef 指向 agents[].name）。
type RelationSpec struct {
	FromRef        string   `json:"fromRef"`
	ToRef          string   `json:"toRef"`
	RelationType   string   `json:"relationType"`
	AllowedActions []string `json:"allowedActions,omitempty"`
}

// WorkflowSpec 声明一个工作流（结构对齐 workflow.Definition/Version/Step）。
type WorkflowSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Steps       []WorkflowStepSpec `json:"steps"`
}

// WorkflowStepSpec 对齐 workflow.Step 的可配置子集。
type WorkflowStepSpec struct {
	Key            string         `json:"key"`
	Name           string         `json:"name,omitempty"`
	Type           string         `json:"type,omitempty"`
	ActorType      string         `json:"actorType,omitempty"`
	ActorRef       string         `json:"actorRef,omitempty"`
	DependsOn      []string       `json:"dependsOn,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	TimeoutSeconds int            `json:"timeoutSeconds,omitempty"`
	MaxRetries     int            `json:"maxRetries,omitempty"`
}

// StateSchemaSpec 声明一个命名空间状态 Schema（schema 必须是合法 JSON
// Schema 对象，写入前经编译器校验）。
type StateSchemaSpec struct {
	Namespace string         `json:"namespace"`
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Schema    map[string]any `json:"schema"`
}

// ExtensionDepSpec 声明扩展依赖（name 为 DNS 式扩展名，版本范围语法同
// 扩展依赖：精确版本/^/~/>=>= 组合）。
type ExtensionDepSpec struct {
	Name         string `json:"name"`
	VersionRange string `json:"versionRange,omitempty"`
}

// SampleDataSpec 声明模板种子状态，仅在安装时提供 target_run_id 时写入
// 该运行（kind 目前仅支持 "state"）。
type SampleDataSpec struct {
	Kind        string         `json:"kind"`
	Namespace   string         `json:"namespace"`
	SubjectType string         `json:"subjectType"`
	SubjectID   string         `json:"subjectId"`
	Data        map[string]any `json:"data"`
}

// AllSections 返回安装支持的 section 名全集（顺序即默认执行顺序）。
func AllSections() []string {
	return []string{
		"personalityTemplates", "agents", "groups", "relations",
		"workflows", "stateSchemas", "sampleData",
	}
}

// HasSection 判断 sections 是否包含指定段；sections 为 nil/空表示全选。
func HasSection(sections []string, name string) bool {
	if len(sections) == 0 {
		return true
	}
	for _, s := range sections {
		if s == name {
			return true
		}
	}
	return false
}
