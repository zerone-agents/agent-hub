# Agent Hub 与 Speeding 拆分方案及实施计划

状态：Draft v0.1

依据：`platform-extension-protocol.md`、`speeding-world-runtime-todo.md` 与当前代码实现

目标：Agent Hub 可作为独立多 Agent 组织平台运营；Speeding 作为首个标准垂直应用验证平台能力

Speeding 的玩家动力、核心循环、生存压力、路线与内容机制见
`speeding-game-design-framework.md`。该文档中的 G0–G4 是本计划 Phase 5–6 的产品设计前置。
分版本的 P0/P1/P2 优先级、Agent Hub 主导的发布节奏和退出门槛见 `version-roadmap.md`。

## 1. 拆分结论

采用“四层产品、三类代码所有权”的结构：

```text
Agent Hub Core
  Agent、模型、工具、知识库、权限、Run、状态、事件、审计、扩展注册
                         │
Organization Runtime    │ 平台内置通用能力
  定向关系、消息、群组、频道、会议、工作流、多跳与预算
                         │
Capability Packages     │ 按协议安装和绑定
  personality / emotion / belief / memory / organization dynamics
                         │
Speeding Application    │ 独立垂直应用
  财富世界、角色、剧情、虚拟时钟、结算、导演、玩家与复盘
```

代码所有权分成：

1. **平台内核代码**：任何租户即使不安装 Speeding 也能使用。
2. **能力包代码与模板**：通过扩展协议安装，不向 Core 表增加领域字段。
3. **Speeding 应用代码**：拥有游戏 UI、世界规则与内容，不进入 Agent Hub 二进制。

MVP 阶段可以共用一个 Git 工作区和一套 Compose，但逻辑边界、数据库表前缀、API 和发布版本
必须从第一天分离。待接口稳定后再拆物理仓库，避免早期跨仓库联调成本。

## 2. 判断标准

每项需求按以下顺序归类：

1. 所有多 Agent 组织都需要的身份、授权、通信和运行保障，进入 **Hub Core/Organization Runtime**。
2. 跨多个应用可复用，但不是每个 Agent 都必须拥有的语义，做成 **Capability Package**。
3. 仅提供初始配置、不执行规则的内容，做成 **Template**。
4. 包含财富、剧情、胜负、玩家体验或 Speeding 世界语义的内容，进入 **Speeding Application/Package**。
5. MCP 是执行适配器，不是领域归属：通过 MCP 调用的游戏规则仍然属于 Speeding。

## 3. 目标模块与职责

### 3.1 Agent Hub Core

必须拥有：

- Agent 长期身份、职责、模型、工具、知识库和权限；
- 人格模板库、版本和 Agent 人格快照；
- Run 生命周期和 RunAgent 绑定；
- 扩展包 registry、安装、启用、绑定、权限审核与版本锁；
- 通用状态 Schema registry、状态快照、乐观锁和变更历史；
- 统一事件信封、幂等、因果链、重试、死信和审计；
- 工具调用的统一结果信封和副作用等级；
- 分阶段提示词合成和注入来源审计；
- 声明式 UI slot 解析，不包含 Speeding 专属页面。

Core 不得认识 `wealth`、`public_opinion`、`chapter`、`ending` 等字段。

### 3.2 Organization Runtime

作为 Agent Hub 的内置通用模块，拥有：

- 有方向的 Agent 关系与关系类型协议；
- 逐跳通信授权和上下文策略；
- 一对一、多跳、一对多与多对多通信基础；
- 群组、成员、频道、订阅、会议和消息线程；
- 任务委托、交接、审批、投票和工作流；
- 自主触发、目标 Agent 串行化、冷却、取消和并发控制；
- 每个 Run、Agent、因果链的 Token、消息跳数、工具次数和时间预算。

关系结构属于平台；某一局里 A 对 B 的信任、敌意和亏欠属于 Run 状态。

### 3.3 通用能力包

首批建议建立：

| 包 | 负责 | 不负责 |
|---|---|---|
| `io.zerone.personality` | 人格提示词片段、人格模板种子、表达倾向 | 动态情绪、权限 |
| `io.zerone.emotion` | 压力、恐惧、愤怒、信心等状态 Schema 和受控变化 | 游戏结算 |
| `io.zerone.belief` | known/believed/suspected/disputed/forgotten 认知状态 | 世界真相 |
| `io.zerone.subjective-memory` | 主观记忆 Schema、可见性、检索接口 | 具体剧情内容 |
| `io.zerone.relationship-dynamics` | 按 Run 隔离的信任、敌意、依赖等状态 | 静态通信权限 |
| `io.zerone.organization-workflow` | 通用工作流、审批、群体决策模板和工具 | 游戏胜负 |

人格模板仍然以提示词为主要来源。`behavior_profile` 在过渡期只作为兼容投影，不再作为新系统
的核心数据模型；动态情绪进入 Run 状态，不写回人格模板或 Agent 主表。

### 3.4 Speeding 专属能力包

| 包 | 负责 |
|---|---|
| `com.speeding.world` | 资产、现金流、公司控制、政治与社会影响、舆情、法律风险及确定性结算 |
| `com.speeding.character-roles` | 首富、董事长、财务总监、记者等角色模板，公开/私人目标、秘密和开局状态 |
| `com.speeding.propagation` | 媒体、社交网络、监管披露、小道消息的延迟、失真、泄漏与可信度规则 |
| `com.speeding.narrative` | 章节、伏笔、冲突密度、人物弧光、路线和结局条件 |

这些包可由 Hub 安装，但只在 Speeding 创建的 Run 中绑定。它们不能修改 Agent Hub Core Schema。

### 3.5 Speeding Application

独立拥有：

- 玩家登录、开局、存档、多人比较和排行榜；
- 游戏虚拟时钟、暂停、倍速、单步与章节推进；
- 玩家操作面板、财富处置 UI 和人物状态 UI；
- 世界事实库、事件内容库和数值平衡；
- 规则结算服务、叙事导演与每日新局面；
- 游戏复盘、因果图、结局与评分。

Speeding 通过 Hub API/MCP 获取 Agent、组织和运行能力，不直接写 Hub 数据库。

## 4. 原 TODO 的逐项拆分

| TODO | 平台部分 | 能力包部分 | Speeding 部分 | 当前状态与调整 |
|---|---|---|---|---|
| 1 世界时钟 | Run 生命周期、定时触发接口、现实时间审计 | 可选 clock adapter | 游戏时间换算、暂停/倍速/跳时、稳定回合排序 | 未实现；拆开，Hub 不内置游戏时间 |
| 2 事件队列 | 统一事件信封、持久队列、幂等、重试、死信、因果链 | 包声明 consume/emit/handler | 游戏事件类型、优先级和 payload | 未实现；作为第一批底座 |
| 3 延迟结算与预算 | 因果链/Agent/Run 预算、调度和终止解释 | 包可声明默认预算 | 延迟财富、舆情、法律结算 | 未实现；预算进平台，结算留游戏 |
| 4 双层人格 | 人格库、版本、选择与快照 | personality、emotion | 角色目标、秘密、游戏初始状态 | 人格库已完成；删除“Agent 自定义覆盖”方向，动态状态按 Run 初始化 |
| 5 NPC 知情状态 | 通用状态、权限与可见性 | belief Schema、检索与注入 | Speeding 世界事实及传播结果 | 未实现；不能放 Agent 主表 |
| 6 主观长期记忆 | Run/Agent 隔离、存取权限、审计 | subjective-memory Schema 与检索 | 剧情记忆内容和评价规则 | 未实现；可复用 multirag，但需严格事实可见性过滤 |
| 7 工具执行反馈 | 标准 ToolResult、事件化副作用、状态提交协议 | 工具/handler 声明 | 成本、公开/私下影响和后续结算 | 当前工具多为文本返回；需平台化改造 |
| 8 自主回合调度器 | 触发器、队列、锁、冷却、预算、取消 | 包声明事件触发 | 世界时钟和导演触发策略 | 当前组织消息有单目标串行队列；尚无通用调度器 |
| 9 传播渠道 | 群组/频道/订阅和可见性底座 | 通用 communication-channel 可后提炼 | 媒体可信度、失真、泄漏、传播半径 | 未实现；先做组织频道，再做 Speeding 传播包 |
| 10 可控多跳消息 | 消息因果链、hop、deadline、预算、逐跳鉴权 | 人格/关系状态影响转发意愿 | 游戏中的隐瞒、越级和泄密动机 | 两个独立回合 A→B→C 已验证；同一链路仍被硬拦截，需重构 |
| 11 群组与成员 | group/member/role/visibility/audit | 组织模板 | Speeding 派系、董事会、临时联盟模板 | 未实现；属于 Hub 核心竞争力 |
| 12 频道与会议 | channel/subscription/thread/meeting/fanout/aggregation | 会议与团队模板 | 剧情会议样式与内容 | 未实现；属于 Organization Runtime |
| 13 工作流与群决策 | workflow/step/approval/vote/quorum/snapshot | organization-workflow | 董事会表决等游戏模板 | 未实现；通用引擎进 Hub，具体流程做模板 |
| 14 叙事导演 | 只提供受控事件注入和预算接口 | narrative 包声明事件与状态 | 节奏、章节、伏笔、人物弧光、结局 | 未实现；不得作为 Hub 超级 Agent |

## 5. 当前已完成能力基线

以下能力可直接保留并纳入新架构：

- 人格模板 CRUD、内置模板、版本历史、Agent 下拉选择和服务端快照；
- Agent 编辑页不再直接编辑人格原稿和结构化滑杆；
- 有向 `agent_relations`，同一 Agent 对可存在不同 scope 和不同关系；
- 双向关系以两条可独立变化的边保存；
- 关系类型模板、版本和拓扑 UI；
- `allowedActions`、`contextPolicy`、`deliveryPolicy` 逐边授权；
- Organization MCP 的 `agent_relations`、`agent_send`、`agent_relation_signal`、
  `agent_message_status` 和 `agent_inbox`；
- 同目标异步投递串行队列；
- 关系分数的受控事件、幂等和 before/after 审计；
- 一对一关系、反向独立关系和跨两个回合的 A→B→C 测试。

这些是“已实现基线”，不应在新计划中重复重写。

## 6. 当前必须偿还的边界债务

### 6.1 Speeding 文案进入平台运行时

当前组织消息信封写死 `[SPEEDING 组织消息]`。应改为中性 `[Agent Hub 组织消息]`，应用语境
通过 `application_context` 提示词阶段或能力包注入。

### 6.2 固定关系事件类型写在 Core

`betrayed`、`credit_stolen`、`public_humiliation` 等固定事件和分值当前由 Hub 硬编码。
短期可作为内置 `relationship-dynamics` 包的兼容规则；长期 Core 只负责验证、应用和审计，
事件类型及 reducer 由能力包声明。

### 6.3 嵌套通信采用进程内一刀切

当前 `active sync.Map` 和提示词中的“本轮不要继续调用 agent_send”禁止目标 Agent 转发。
这保护了系统，但不能满足同一业务链路 A→B→C。需要改为持久化因果链、异步续投、逐跳鉴权
和预算终止；同步调用栈仍禁止相互等待。

### 6.4 动态关系状态与静态关系合同混表

`relationship_score` 和 `stance` 当前直接存于静态关系边。平台独立运营时静态边用于权限合同，
动态态度应放入 `run_relation_states`，否则不同游戏或任务运行会互相污染。迁移期可把全局分数
解释为默认基线，新 Run 初始化后不再回写静态边。

### 6.5 人格兼容投影仍在 Agent 主表

`behavior_profile` 可暂留以兼容现有部署协议，但新功能不得继续依赖它。最终人格模板只保存
原稿与可选投影，Agent 保存模板快照；运行状态由扩展状态服务管理。

### 6.6 缺少 Run 作为统一隔离边界

当前关系 scope 是字符串，组织消息也没有正式 Run、correlation 和 causation。必须先建立 Run，
再扩展状态、事件、消息和游戏；不能继续用 `speeding-hq` 一类 scope 代替一局游戏。

## 7. 目标数据边界

### 7.1 Hub Core 表

```text
extension_packages
extension_installations
extension_resources
extension_bindings

runs
run_agents
run_capability_snapshots

state_schemas
run_states
run_state_changes

events
event_deliveries
causal_chain_budgets
prompt_composition_logs
```

`run_states.data` 是按 Schema 校验的 JSON；Core 不增加游戏字段列。

### 7.2 Organization Runtime 表

```text
agent_relations                 # 保留，静态授权合同
relation_type_templates         # 保留
agent_messages                  # 扩充链路字段

agent_groups
group_members
group_channels
channel_subscriptions
channel_messages
message_deliveries

meetings
meeting_participants
meeting_records

workflows
workflow_versions
workflow_runs
workflow_step_runs
approvals
approval_votes
```

### 7.3 Speeding 自有表

Speeding 的内容和玩家数据留在 Speeding 数据库：

```text
game_sessions                   # 映射 Hub run_id
world_facts
world_clock
world_event_templates
world_settlements
story_chapters
narrative_threads
endings
player_actions
leaderboards
replay_summaries
```

Speeding 状态可通过 Hub `run_states` 保存，但大体量内容库、排行榜和查询型世界事实应留在
Speeding 数据库。Hub 只持有引用和审计事件，避免成为所有垂直应用的数据仓库。

## 8. 目标运行链路

```text
玩家 / 世界时钟 / Director
          │
          ▼
Speeding 发布领域事件 ──→ Hub Event Service
                              │
                   校验租户、Run、Schema、预算
                              │
             ┌────────────────┼─────────────────┐
             ▼                ▼                 ▼
       能力包 reducer    Agent 调度/通信     Speeding 结算器
             │                │                 │
             └──────────产生后续事件────────────┘
                              │
                       Hub 提交状态变化
                              │
          ┌───────────────────┴──────────────────┐
          ▼                                      ▼
   分阶段提示词投影                         时间线与复盘 UI
```

Agent 作出的是行动提议。涉及财富、舆论、法律和关系变化时，确定性规则或受控概率处理器提交
最终结果，LLM 不直接写最终状态。

## 9. 实施阶段

### Phase 0：边界冻结与兼容清理

目标：停止继续把 Speeding 语义写进 Hub。

- [ ] 将组织消息信封改为平台中性文案；
- [ ] 为现有关系事件规则标记内置包来源与规则版本；
- [ ] 明确 `behavior_profile` 为 deprecated compatibility projection；
- [ ] 给现有消息、关系事件补充可迁移的 correlation/causation 设计；
- [ ] 建立 Architecture Decision Records：Run、状态所有权、事件投递语义、仓库边界；
- [ ] 为协议清单建立 JSON Schema 和示例目录，但暂不执行第三方代码。

验收：不安装 Speeding 时，Hub 所有用户界面、运行提示词和数据库核心字段均无 Speeding 品牌
或财富游戏语义。

### Phase 1：Run、扩展注册与通用状态

目标：实现后续所有动态系统共同依赖的最小平台底座。

- [ ] `runs`、`run_agents` 与状态机（draft/running/paused/completed/archived）；
- [ ] 包上传、校验、安装、启用、禁用和显式绑定；
- [ ] 包、Schema、Prompt、规则的 Run 版本快照；
- [ ] JSON Schema registry；
- [ ] `run_states`、revision 乐观锁、idempotency 和 append-only history；
- [ ] 状态初始化 API 和只读历史 API；
- [ ] 最小 `run.agent.detail`、`run.relation.detail` 和 `run.timeline` 声明式 UI；
- [ ] `emotion-state` 与 `speeding-world` 两个无代码示例包。

验收：同一 Agent 同时进入两个 Run，初始化不同状态，修改其中一局不会影响 Agent 本体、人格
模板或另一局。

### Phase 2：事件底座、预算与工具结果

目标：让变化可执行、可解释、可恢复。

- [ ] 持久化事件信封和 delivery 记录；
- [ ] 至少一次投递、幂等、重试、超时、取消和死信；
- [ ] correlation/causation/root、稳定同主体排序；
- [ ] 因果链 hop、事件数、Token、工具次数、并发和 wall-clock 预算；
- [ ] 标准 ToolResult 与 `proposal -> validate -> commit` 流程；
- [ ] 有限声明式 reducer；
- [ ] Prompt Composer 分阶段注入并记录 provenance/hash；
- [ ] 服务重启、重复投递和规则版本回放测试。

验收：一次重复投递不会重复扣状态；链路超限会生成 guard 事件；旧 Run 仍按锁定版本复盘。

### Phase 3：Organization Runtime 补全

目标：完成 Agent Hub 自身可产品化的多 Agent 协作能力。

- [ ] 将组织消息接入统一事件和预算；
- [ ] 支持异步 A→B→C、合法回报和有限循环；
- [ ] 移除进程内全局嵌套禁令，保留同步等待死锁保护；
- [ ] 群组、成员和成员角色；
- [ ] 频道、订阅、线程、fanout 和 aggregation；
- [ ] 会议、参与者、议程和会议记录；
- [ ] 工作流、步骤、审批、投票、法定人数、否决与超时升级；
- [ ] 管理 UI：组织树、关系网、频道、工作流和历史会议；
- [ ] Organization MCP 增加 group/channel/workflow/approval 工具。

验收：一对一、一对多和多对多均有真实 Agent 运行测试；每条消息知道谁收到、谁处理、为何
被拒绝或停止，群体决策可以解释结果。

### Phase 4：认知、记忆与动态人格能力包

目标：让 Agent 在不同 Run 中形成不同经历，而不污染长期身份。

- [ ] `belief` 包：事实引用、认知状态、置信度、来源、保密级别；
- [ ] `subjective-memory` 包：解释、情绪、重要度、事件来源和检索；
- [ ] `emotion` 包：动态状态、衰减、阈值叙述和 reducer；
- [ ] `relationship-dynamics` 包：定向信任、敌意、依赖等 Run 状态；
- [ ] 人格底色 + 当前状态 + 对方关系 + 可见记忆的提示词合成；
- [ ] 事实不可见、谣言冲突、跨 Run 隔离和敏感字段测试。

验收：Agent 只能引用自己获知的事实；同一长期 Agent 在不同 Run 有不同记忆、情绪和关系态度。

### Phase 5：Speeding 世界 MVP

目标：垂直应用通过标准协议运行，而不是直接侵入 Hub。

- [ ] 独立 Speeding 服务和前端壳；
- [ ] 完成 G0 游戏命题验证：玩家动力、四条生存生命线和三层核心循环；
- [ ] `com.speeding.world` Schema、事件与结算器；
- [ ] 角色模板、开局绑定和初始状态；
- [ ] 世界事实、虚拟时钟、暂停/倍速/单步；
- [ ] 压力时钟、承诺锁、自主派系议程、待办债务和信息揭示；
- [ ] 资产、舆情、法律、控制权的工具与延迟结算；
- [ ] 私聊、会议、公告、媒体和监管等传播渠道；
- [ ] 人物状态页、事件时间线和基础复盘；
- [ ] 先制作一条 30–45 分钟可完整跑通的垂直切片。
- [ ] 垂直切片至少包含系统、路线、人物私事各一个事件及一次 fail-forward；

验收：Speeding 可以在不修改 Hub Core Schema 的情况下安装、开局、推进、结算和复盘；卸载或
禁用 Speeding 包不影响 Hub 组织协作。

### Phase 6：叙事厚度与长局

目标：从技术闭环扩展到 7–8 小时的可玩内容。

- [ ] Narrative Director 的章节、节奏、伏笔和冲突密度；
- [ ] 连续性、程序、去权力三条治理路线及交叉的真相调查轴；
- [ ] 约 40 个路线事件、人物私事事件及更多系统事件；
- [ ] 系统、路线、人物私事、环境四类事件使用统一内容 Schema；
- [ ] 私人目标、秘密、联盟、背叛和人物弧光；
- [ ] 每日新局面、多人比较、评分和排行榜；
- [ ] 多结局、失败恢复、长局预算与摘要；
- [ ] 内容版本和旧存档兼容。

验收：先完成 2 小时内容密度测试，再扩展到 7–8 小时；导演只能注入已授权事件模板，不能
代替 NPC 决策或直接篡改结算结果。

## 10. 优先级调整

原 TODO 将世界时钟排在最前，但新架构下优先级应调整为：

```text
Run 隔离
  → 扩展注册与版本锁
  → 通用状态
  → 事件与预算
  → 多跳/群组/工作流
  → 认知、记忆、情绪
  → Speeding 虚拟时钟与世界结算
  → 叙事导演与长内容
```

原因是虚拟时钟如果先于 Run 和事件协议实现，会形成只能服务 Speeding 的第二套调度系统。

## 11. 仓库与部署建议

### 11.1 当前阶段

```text
agent-hub/
  internal/domain/run
  internal/domain/extension
  internal/domain/state
  internal/domain/event
  internal/domain/organization
  examples/extensions/

speeding/                    # 独立应用目录或独立仓库
  apps/web
  services/world
  services/narrative
  packages/com.speeding.*
```

通用能力包初期可以放在 Agent Hub 的 `examples/extensions` 验证协议，但稳定后应拥有独立版本
和发布制品。Speeding 专属包从一开始归 Speeding 所有。

### 11.2 部署

开发环境可用同一台服务器和 Compose：

```text
agent-hub + agent-runtime + event worker + speeding-api + speeding-web
```

但数据库凭据和 Schema 必须分开。Speeding 只能通过 Hub API、事件入口或 MCP 访问 Hub；
禁止 Speeding 服务直接连接并写 Agent Hub 数据库。

## 12. 测试矩阵

### 平台独立性

- 未安装任何能力包时 Agent、关系和消息可用；
- 安装 Speeding 后 Hub UI 不出现强制游戏字段；
- 禁用/卸载 Speeding 后历史可读、平台继续运行；
- 能力包不能跨租户、跨命名空间或绕过关系权限。

### Run 与状态

- 同 Agent 多 Run 隔离；
- 并发 revision 冲突明确返回；
- 相同 idempotency key 只应用一次；
- Run 完成后写入被拒绝；
- 包升级不改变旧 Run。

### 事件与通信

- 重启不丢事件；
- A→B、B→A、A→B→C、A→B→A 均按授权工作；
- 无限循环由预算停止，不靠禁止现实行为；
- 同步互等被拒绝，异步回报可继续；
- 上下文可见性逐跳收窄，不能扩大。

### Speeding 垂直应用

- LLM 不能直接写余额、舆情、法律和关系最终值；
- 未知事实不会进入 NPC 上下文；
- 私聊和公告的传播范围及影响不同；
- 相同种子、锁定规则和输入可重放确定性结算；
- 叙事导演受事件模板和预算约束。

## 13. 最近三个可执行里程碑

### M1：边界清理

只修改 Hub：中性化组织消息、标记关系 reducer 来源、补 ADR 和扩展清单 Schema。

### M2：Run + State 最小闭环

创建两个 Run，把同一个 Agent 绑定进去；加载人格快照与不同初始 emotion 状态；通过 API 修改
其中一局并在声明式人物状态页看到历史。

### M3：Event + A→B→C 最小闭环

将 Agent 消息迁入统一事件链，实现异步 A→B→C；每一跳重新鉴权、消耗预算并记录因果关系；
用 A→B→A→B 测试预算终止，而不是直接禁止回传。

完成 M1–M3 后，再开始群组和 Speeding 世界逻辑。否则继续堆游戏内容会产生第二轮拆分返工。
