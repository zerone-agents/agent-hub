# Agent Hub 平台扩展协议

状态：Draft v0.1

协议标识：`agenthub.extension/v1alpha1`

目标读者：Agent Hub、Agent Runtime、能力包及垂直应用开发者

## 1. 目的

本协议定义 Agent Hub 如何安装、校验、授权和运行可插拔能力包（Capability
Package）。能力包可以声明状态 Schema、事件、工具、关系类型、提示词注入和
UI 展示，但不能绕过 Agent Hub 的租户、关系、上下文和工具权限。

协议的产品边界是：

- Agent Hub Core 管理 Agent 身份、模型、知识库、工具、权限、运行、消息和审计。
- Organization Runtime 管理组织、定向关系、频道、任务和 Agent 间通信。
- 能力包扩展平台语义与运行能力，可跨应用复用。
- 垂直应用组合能力包，并拥有自己的世界模型、业务规则、流程和用户界面。
- 模板只创建配置或实例，不执行代码，也不授予权限。

因此，`stress` 可以由一个通用心理状态能力包声明，`wealth` 可以由 Speeding
世界能力包声明；Agent Hub Core 对二者都只理解为经过 Schema 校验的命名空间状态。

## 2. 规范用语

本文中的 **MUST**、**MUST NOT**、**SHOULD**、**SHOULD NOT**、**MAY** 分别表示
必须、禁止、建议、不建议和可选。

## 3. 核心概念

### 3.1 模板（Template）

模板是只包含数据的可复用配置，例如人格模板、关系模板、团队模板或游戏角色模板。
模板 MUST NOT：

- 执行代码或远程请求；
- 注册新工具；
- 增加 Agent 权限；
- 修改事件或状态处理规则。

### 3.2 能力包（Capability Package）

能力包是带版本、依赖、权限和扩展声明的安装单元。它 MAY 包含：

- JSON Schema；
- 提示词片段；
- UI Schema；
- 模板种子；
- 声明式状态转换规则；
- 本地受信处理器或远程 MCP/HTTP 处理器引用。

### 3.3 垂直应用（Vertical Application）

垂直应用拥有用户旅程和领域规则。Speeding 的回合、财富、舆论、剧情、胜负条件和
玩家复盘属于垂直应用。垂直应用 MAY 发布自己的私有或公开能力包，但这些包仍须遵守
本协议。

### 3.4 运行（Run）

运行是有生命周期的状态隔离边界，可以是一局游戏、一次企业推演或一次长任务。
动态人物状态 MUST 绑定运行，MUST NOT 直接覆盖 Agent 的长期人格模板。

### 3.5 主体和作用域

状态和事件通过以下坐标定位：

```text
tenant + namespace + scope(type,id) + subject(type,id)
```

标准主体类型为 `agent`、`relation`、`group`、`run`。能力包 MAY 声明额外类型，
但必须使用包命名空间前缀。

## 4. 包格式

每个包根目录 MUST 包含 `extension.yaml`：

```yaml
apiVersion: agenthub.extension/v1alpha1
kind: CapabilityPackage
metadata:
  name: emotion-state
  namespace: io.zerone.organization.emotion
  version: 1.2.0
  displayName: 情绪状态
  description: 为运行中的 Agent 提供可审计的短期心理状态
  license: Apache-2.0
  publisher: zerone

compatibility:
  hub: ">=0.9.0 <2.0.0"
  runtimeProtocol: ">=1.0.0 <2.0.0"

dependencies:
  - package: io.zerone.organization.relations
    version: "^1.0.0"
    optional: true

permissions:
  state:
    read: ["io.zerone.organization.emotion/*"]
    write: ["io.zerone.organization.emotion/*"]
  events:
    consume: ["agenthub.task.*", "agenthub.message.*"]
    emit: ["io.zerone.organization.emotion.state.changed.v1"]
  tools:
    expose: ["emotion.get_state", "emotion.propose_change"]
  network:
    outbound: []

contributes:
  stateSchemas:
    - id: character-state
      version: 1.0.0
      file: schemas/character-state.schema.json
  events:
    - file: events/events.yaml
  tools:
    - file: tools/tools.yaml
  relationTypes: []
  promptFragments:
    - file: prompts/current-state.yaml
  uiViews:
    - file: ui/character-state.yaml
  templates:
    - file: templates/default-state.yaml
  handlers:
    - file: handlers/state-reducer.yaml
```

### 4.1 标识规则

- `metadata.namespace` MUST 使用反向域名或组织所有的全局唯一前缀。
- 包内资源完整标识为 `<namespace>/<resource-id>@<version>`。
- `metadata.version` 和所有资源版本 MUST 使用 SemVer。
- 包发布后内容 MUST 不可变；相同名称和版本 MUST 对应相同内容哈希。
- 安装记录 MUST 保存发布者、版本、内容哈希、签名状态和安装者。

### 4.2 安装、启用和绑定

三者 MUST 分离：

- **安装**：租户接受包版本及其权限请求，资源进入 registry；
- **启用**：允许包处理新请求，但尚未自动影响任何 Agent；
- **绑定**：把包显式绑定到 Agent、组织或 Run。

能力包启用后 MUST NOT 自动注入所有 Agent。垂直应用创建 Run 时，应提交本局使用的包、
模板和版本锁；平台据此创建不可变的 Run binding snapshot。人格等长期能力 MAY 绑定 Agent，
情绪、世界状态等动态能力 SHOULD 绑定 Run 或 RunAgent。

## 5. 状态 Schema

### 5.1 声明

状态 MUST 使用 JSON Schema 2020-12。示例：

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "io.zerone.organization.emotion/character-state@1.0.0",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "stress": { "type": "integer", "minimum": 0, "maximum": 100 },
    "confidence": { "type": "integer", "minimum": 0, "maximum": 100 }
  },
  "required": ["stress", "confidence"]
}
```

Schema 声明还 MUST 指定：

```yaml
id: character-state
version: 1.0.0
scopeTypes: [run]
subjectTypes: [agent]
initialization: template_or_default
concurrency: optimistic
retention: run_lifetime
sensitivePaths: []
```

### 5.2 持久化契约

Agent Hub 应提供通用状态记录，逻辑模型如下：

```json
{
  "namespace": "io.zerone.organization.emotion",
  "schema": "character-state@1.0.0",
  "scope": { "type": "run", "id": "run_01" },
  "subject": { "type": "agent", "id": "42" },
  "revision": 18,
  "data": { "stress": 71, "confidence": 42 },
  "updatedAt": "2026-09-12T10:00:00Z"
}
```

平台 MUST：

- 在写入前校验 Schema、租户、作用域和权限；
- 使用 revision 做乐观锁；
- 支持 `idempotencyKey`；
- 保存 append-only 变更记录，包括 before、patch、after、原因和来源；
- 在运行结束时冻结或按保留策略归档状态；
- 禁止能力包读取未声明命名空间的数据。

状态变更 SHOULD 使用 JSON Merge Patch 或受限操作，不应由模型直接提交任意最终快照。

## 6. 事件协议

所有事件 MUST 使用统一信封：

```json
{
  "specVersion": "1.0",
  "id": "evt_01J...",
  "type": "com.speeding.world.requested_coverup.v1",
  "source": "package:com.speeding.world@1.0.0",
  "tenantId": "acme",
  "scope": { "type": "run", "id": "run_01" },
  "subject": { "type": "agent", "id": "42" },
  "actor": { "type": "agent", "id": "7" },
  "time": "2026-09-12T10:00:00Z",
  "visibility": "participants",
  "correlationId": "turn_18",
  "causationId": "msg_103",
  "idempotencyKey": "run_01:turn_18:coverup:42",
  "dataSchema": "com.speeding.world/requested-coverup@1.0.0",
  "data": { "severity": 3 }
}
```

平台事件使用 `agenthub.*` 前缀；包事件 MUST 使用自身命名空间。事件类型版本不可省略。

事件处理要求：

- 默认异步、至少一次投递，消费者 MUST 幂等；
- 同一 `scope + subject` SHOULD 保序，不保证全局顺序；
- 处理器 MUST 设置最大重试、退避、超时和死信策略；
- 派生事件 MUST 继承 correlationId 并记录 causationId；
- 平台 MUST 限制每条因果链的深度、事件数和时间预算；
- 达到预算后 MUST 终止自动传播并产生可审计的 `agenthub.guard.triggered.v1`；
- 可见性和上下文权限 MUST 逐跳重新计算，不得沿消息链自动扩大。

`A -> B -> A -> B` 在业务上允许，但每次往返必须是新的、被授权的事件；相同幂等键
不会重复生效，因果链预算负责防止无界消息爆炸。

## 7. 工具声明

工具声明示例：

```yaml
tools:
  - id: emotion.get_state
    title: 读取当前人物状态
    description: 读取调用 Agent 在当前运行中的可见心理状态
    inputSchema:
      type: object
      properties:
        subjectAgentId: { type: string }
      required: [subjectAgentId]
      additionalProperties: false
    outputSchema:
      $ref: ../schemas/character-state.schema.json
    executor:
      type: builtin
      handler: state.read
    effects: read
    auth:
      subject: caller_agent
      relationActionsAnyOf: [inform, consult, review]

  - id: emotion.propose_change
    title: 提议状态变化
    executor:
      type: mcp
      serverRef: emotion-engine
      remoteTool: propose_state_change
    effects: proposal
    confirmation: policy
```

`executor.type` v1alpha1 支持：

- `builtin`：由 Hub/Runtime 的受信处理器执行；
- `mcp`：调用已安装且已授权的 MCP Server；
- `http`：调用经过域名白名单和凭据绑定的远程端点。

工具声明 MUST 包含输入 Schema、输出 Schema、影响级别和授权策略。模型看到工具不等于
获得调用权限；运行时 MUST 按调用者、租户、运行、关系动作、包权限和用户策略再次鉴权。

影响级别为：

- `read`：只读；
- `proposal`：产生待规则引擎或用户确认的建议；
- `write`：修改本包拥有的状态；
- `external`：产生外部副作用。

包 SHOULD 优先让模型调用 `proposal` 工具，由确定性规则引擎校验并提交最终变化。

## 8. 关系类型声明

关系类型是稳定运行协议之上的产品语义层：

```yaml
relationTypes:
  - id: dotted-line-manager
    title: 虚线主管
    baseType: advisor
    directionPolicy: paired_asymmetric
    inverse:
      relationType: dotted-line-report
      required: true
    defaultStance: neutral
    allowedActions: [inform, consult, assign, report, escalate]
    contextPolicy: summary_only
    deliveryPolicy: async
    constraints:
      maxHops: 3
      canDelegate: true
      canEscalate: policy
    ui:
      lineColor: "#7c3aed"
      lineStyle: dashed
```

`directionPolicy` 支持：

- `one_way`：只有一条定向边；
- `paired_symmetric`：创建语义相同的反向边；
- `paired_asymmetric`：创建语义不同的反向边；
- `group_membership`：关系指向群组，由群组策略决定传播。

关系类型只声明通信和授权能力。动态信任、敌意等 SHOULD 存入按运行隔离的关系状态，
而不是修改关系类型本身。现有 `relation_type_templates` 可作为此声明的本地实例，现有
两条定向 `agent_relations` 继续表示可独立变化的双向关系。

## 9. 提示词注入

提示词片段声明示例：

```yaml
promptFragments:
  - id: current-emotion
    stage: dynamic_state
    priority: 100
    template: prompts/current-state.md
    data:
      - stateRef: character-state
        required: false
    audience: agent
    maxTokens: 300
    onMissing: omit
```

标准注入阶段及顺序为：

```text
platform_safety
identity
responsibilities
personality_baseline
organization_policy
dynamic_state
relationship_context
application_context
recent_memory
user_input
```

平台安全与权限约束 MUST 不可被包覆盖。能力包只能写入被允许的阶段；同阶段按 priority、
包名和片段名确定稳定顺序。所有注入内容 MUST 记录包版本、片段版本和渲染后哈希，以便复盘。

提示词用于影响判断和表达，MUST NOT 被当作访问控制。人格模板、动态状态和关系上下文
不得自行赋予工具、通信或数据权限。

## 10. UI 展示协议

能力包 MAY 声明受限的声明式 UI Schema：

```yaml
uiViews:
  - id: character-state-panel
    slot: run.agent.detail
    title: 当前状态
    dataSource:
      type: state
      schemaRef: character-state@1.0.0
    renderer: metric-grid
    fields:
      - path: /stress
        label: 压力
        format: score-100
        visualization: bar
      - path: /confidence
        label: 信心
        format: score-100
        visualization: bar
    history:
      enabled: true
      renderer: timeline
```

v1alpha1 标准 slot：

- `agent.detail.extension`：Agent 长期配置的扩展摘要；
- `relation.detail.extension`：静态关系配置摘要；
- `run.overview`：运行概览；
- `run.agent.detail`：本次运行的人物状态；
- `run.relation.detail`：本次运行的定向关系状态；
- `run.timeline`：事件和状态变化历史。

标准 renderer：`key-value`、`metric-grid`、`badge-list`、`table`、`timeline`、
`json-inspector`。包 MUST 默认使用声明式组件，不能注入任意 JavaScript。

如果未来允许沙箱 UI，必须使用独立扩展协议版本，并具备 CSP、跨域隔离、权限提示和
审核机制。Agent 新建/编辑页面 SHOULD 只显示能力包或模板选择，不显示运行中的动态值。

## 11. 处理器与状态转换

声明式处理器示例：

```yaml
handlers:
  - id: task-failure-reducer
    consumes: agenthub.task.failed.v1
    mode: reducer
    targetState: character-state@1.0.0
    transition:
      operations:
        - path: /stress
          op: increment
          valueFrom: "clamp(event.data.severity * 5, 0, 20)"
        - path: /confidence
          op: increment
          valueFrom: "clamp(event.data.severity * -3, -12, 0)"
      clampToSchema: true
    emits: emotion.state.changed.v1
```

v1alpha1 SHOULD 首先支持有限的声明式 reducer。表达式环境不得访问网络、文件系统、
密钥或未声明状态。复杂逻辑可通过 MCP/HTTP 处理器执行，但返回值仍须经 Hub 校验后落库。

LLM MAY 提议状态变化和原因，MUST NOT 直接覆写状态。规则引擎负责边界、权限、幂等、
并发和最终提交。

## 12. 生命周期

能力包状态为：

```text
uploaded -> validated -> installed -> enabled -> disabled -> uninstalled
```

安装流程 MUST：

1. 校验清单、Schema、内容哈希和签名；
2. 求解依赖与 Hub/Runtime 兼容性；
3. 展示请求权限；
4. 注册资源但不默认扩大已有 Agent 权限；
5. 执行显式、可回滚的数据迁移；
6. 生成安装审计记录。

禁用包 MUST 停止新事件消费和工具调用，但保留历史数据可读。卸载前 MUST 检测依赖、
已绑定 Agent、活跃运行和状态保留策略；不得静默删除审计和复盘数据。

## 13. 安全与隔离

- 所有资源、状态、事件和安装记录 MUST 具有租户边界。
- 共享内置包可以使用共享 sentinel，但租户 MUST 以复制或覆盖方式修改，不能原地写共享包。
- 包权限 MUST 使用最小权限；未声明即拒绝。
- MCP/HTTP 凭据 MUST 由平台密文存储，不能出现在清单、提示词或事件数据中。
- 敏感状态路径 MUST 支持遮蔽、字段级读取策略和审计。
- Agent 间消息仍必须经过关系的 `allowedActions`、`contextPolicy` 和 `deliveryPolicy`。
- 包不能通过提示词、UI 或事件伪造 Agent 身份。
- 外部副作用工具 SHOULD 支持人工确认、配额和可撤销补偿操作。

## 14. 版本与迁移

- 补丁版本不得改变 Schema 含义或权限。
- 增加可选字段可发布次版本。
- 删除字段、改变含义、扩大数据可见性或修改事件语义必须发布主版本。
- 每次运行 MUST 锁定能力包、Schema、提示词片段、关系模板和规则版本快照。
- 新版本不得改变旧运行的复盘结果。
- 状态迁移 MUST 显式声明 `from`、`to`、迁移器、回滚器和数据保留策略。

## 15. Speeding 作为垂直应用

建议拆为：

```text
通用能力包
  io.zerone.organization.personality
  io.zerone.organization.emotion
  io.zerone.organization.relations
  io.zerone.organization.workflow

Speeding 专属能力包
  com.speeding.world
  com.speeding.narrative
  com.speeding.character-roles

Speeding 应用
  回合控制、玩家操作、剧情编排、每日局面、胜负、排行、复盘
```

`com.speeding.world` 可以声明财富、社会影响、公司控制权等 Schema 和事件；这些字段不会
进入 Agent Hub Core。若以后另一个应用也需要其中一部分，应提炼为新的通用包，而不是
让应用直接依赖 Speeding 的世界包。

一个角色在开局时的组合关系为：

```text
长期 Agent 身份
+ 人格模板快照
+ 组织关系快照
+ 本局角色模板
+ 本局能力包状态初始化
= RunAgent（仅在本局存在）
```

## 16. 与现有 Agent Hub 的映射

| 现有实现 | 协议映射 | v0.1 处理 |
|---|---|---|
| `personality_templates` / versions | 模板 + 提示词片段来源 | 保留，逐步通过包声明种子和注入阶段 |
| `relation_type_templates` / versions | 关系类型本地实例 | 保留稳定 `baseType`，增加包资源来源字段 |
| 两条定向 `agent_relations` | 非对称双向关系 | 保留，不改为一条双向记录 |
| `agent_relation_events` | 关系状态事件历史 | 适配统一事件信封，保留 before/after |
| `agent_messages` | 消息投递审计 | 增加 correlation/causation/idempotency 和链路预算 |
| `mcp_servers` | MCP 执行器注册 | 能力包只引用，绑定和凭据继续由 Hub 管理 |
| Agent deployment snapshot | 运行工件快照 | 增加能力包及资源版本和哈希 |

## 17. v0.1 实施范围

第一阶段 MUST 完成：

1. `extension.yaml` 校验器和包安装记录；
2. 状态 Schema registry；
3. 按 Run 隔离的通用状态与 append-only 变化历史；
4. 统一事件信封、幂等和因果链预算；
5. 提示词分阶段合成及来源审计；
6. 声明式 `run.agent.detail` 和 `run.timeline` UI；
7. 能力包权限审核；
8. 一个通用 `emotion-state` 示例包；
9. 一个 `speeding-world` 示例包；
10. 安装、禁用、版本锁定、跨租户和循环传播测试。

v0.1 明确不做：

- 任意前端 JavaScript 插件；
- 能力包自行访问数据库；
- 无限制脚本执行；
- 通过提示词授予权限；
- 跨运行共享动态人物状态；
- 自动把垂直应用字段提升为平台核心字段。

## 18. 验收标准

协议实现满足以下条件才可用于 Speeding：

- 删除 Speeding 包后，Agent Hub 的 Agent、人格、关系、消息和任务仍可独立工作；
- 安装 Speeding 包不需要修改 Agent Hub Core 表中的领域字段；
- 同一 Agent 可同时进入两个 Run，动态状态互不影响；
- 旧 Run 可按当时锁定的包版本完整复盘；
- Agent A 与 B 往返通信合法，但无限传播会被链路预算阻断并留痕；
- UI 根据声明展示状态，Agent 编辑页不出现游戏动态数值；
- 未获授权的包无法读取其他命名空间、其他租户或不可见关系上下文；
- 模型不能绕过规则引擎直接修改最终状态。

## 19. 参考 API 边界

以下是协议要求的逻辑 API，不限定最终 REST 路径，但语义 MUST 保持稳定：

```text
PackageRegistry
  validate(package) -> validation report
  install(package, accepted permissions) -> installation
  enable/disable(package, version)
  list/get/uninstall

Bindings
  bind(target, package, version, config)
  resolve(run, agent) -> locked capability set

State
  initialize(scope, subject, schema, template, idempotencyKey)
  get/list/query
  proposePatch(expectedRevision, patch, reason)
  commitPatch(expectedRevision, patch, event, idempotencyKey)
  history

Events
  publish(event envelope)
  subscribe(type patterns, handler)
  replay(scope, cursor)

Prompt
  compose(run, agent, stage inputs) -> text + provenance

UI
  resolveViews(slot, scope, subject) -> validated declarative views
```

建议的 Hub REST 资源为：

```text
/api/v1/extensions/packages
/api/v1/extensions/installations
/api/v1/extensions/bindings
/api/v1/runs
/api/v1/runs/:runId/states
/api/v1/runs/:runId/events
/api/v1/runs/:runId/ui-views
```

运行时工具接口可以通过内建工具或 MCP 暴露，但最终写入 MUST 回到 Hub 的状态和事件服务，
避免各能力包形成不可审计的私有状态孤岛。
