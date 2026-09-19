# H6 动态人物能力包协议

H6 给 Agent 增加"人味"：心情、看法、记忆、两人关系。它们都是**每一局（Run）里的临时状态**，

- 不改变 Agent 的权限——心情再差也不能越权，记忆再好也不能解锁工具；
- 不进入 Hub Core 的表结构——所有动态状态存在既有的 RunState 里，Core 只提供存放和注入通道；
- 每一局互不影响、每个租户互不可见；包卸载后 Core 照常运行。

人格底色（personality_template 快照）+ 动态状态（RunState）+ 对方关系（关系动态包）+ 可见记忆（记忆包）在 Prompt Composer 中合成为最终提示词。提示词只引导行为，从不授予权限。

## 范围与分层

**H6 管**：单个 Agent 的心情 / 看法 / 记忆，以及两个人之间的定向态度。

**顺手支持**：状态寻址允许挂在组上（如"团队氛围"），各包可用同一机制声明组级聚合状态，但这不是退出门槛。

**不归 H6**：组织架构与频道（H4）、投票与审批（H5）、Speeding 世界规则（派系声望、舆情等，留给世界能力包）。关系动态包只能**影响**既有组织机械的执行结果（如派任务是否被接受），不重写机制。

## 共享约定（四个包都遵守）

1. **存放**：全部状态写 `run_states`（namespace=包名，subject 见各包），历史写 `run_state_changes`。不新建状态表。
2. **确定性**：状态转移是 `(上一状态, 事件, 事件时间)` 的纯函数；衰减在读取时按记录的事件时间惰性结算，不依赖读取时刻的挂钟——同一输入序列可重放。
3. **幂等**：所有写入带 idempotencyKey，`run_state_changes` 的唯一索引兜底，重复事件不产生重复效果。
4. **版本锁定**：Run 创建时的 CapabilityBinding 快照锁定包版本与词表版本；局内规则不变（与 H5 冻结投票人同思路）。
5. **身份**：MCP 工具的身份来自运行时令牌，不接受 agent_id 参数；Agent 只能读自己的状态，只能写自己的记忆。
6. **敏感字段**：看法与记忆默认敏感——只注入到持有者本人的提示词（audience=agent:self），管理端只读接口可查（审计需要）。
7. **旧列不动**：`agent_relations.relationship_score` 保持只读遗留列，关系动态包的态度只存 RunState（代码已有占位注释，本就如此设计）。

## 包 1：io.zerone.emotion —— 心情

**状态**（schema `emotion-state` v1，subject=agent）：

```json
{
  "mood": "calm | wary | tense | angry | elated | grieving",
  "intensity": 0,
  "baseline": "calm",
  "updatedAt": "事件时间",
  "decayPerDay": 20,
  "narration": "由强度阈值映射生成的叙述句"
}
```

**规则**：

- 心情事件词表（包私有，v1 锁定）：`betrayed / trusted / threatened / comforted / task_failed / task_completed / insulted / praised`，每个词表项带 delta 与严重度系数（1/1.5/2），单事件上限 +30 / -40，分数钳制 0–100。
- 衰减：读取时按 `updatedAt` 到当前事件时间的天数向 `baseline` 回落（惰性结算，写回时带新 revision）。`baseline` 默认值来自人格底色中声明的默认心情，未声明则为 `calm`。
- 叙述：强度分档（0–20 平静 / 21–40 波动 / 41–60 显著 / 61–80 强烈 / 81–100 主导行为）映射为固定句式，由确定性规则生成后写入 `narration`，模型不可自由发挥。

**MCP**：`emotion_status()` —— 返回自己的当前心情与叙述。Agent 不能给自己写心情（模型不直接结算，保持"模型与结算分离"）。

## 包 2：io.zerone.belief —— 看法

**状态**（schema `belief-state` v1，subject=agent，每个事实一条状态，subjectID=事实引用）：

```json
{
  "factRef": "事件或事实的稳定 ID",
  "status": "known | believed | doubted | disputed | forgotten",
  "confidence": 70,
  "source": "delivery | observation | claim",
  "lastEventAt": "事件时间"
}
```

**规则**：

- 可见性：Agent 只对**送达过**自己的事实持有看法（Core 的消息/事件投递记录是事实来源）；未送达的事实不会出现在它的状态里——"未知事实不会被注入"。
- 证据更新：同类证据重复送达 → confidence 上升；矛盾的证据到达 → 降档（believed → doubted → disputed）；`disputed` 状态下双方证据都保留在审计里。
- 遗忘：confidence 按事件时间衰减；低于 20 转为 `forgotten`，不再进入提示词。`forgotten` 不是删除——审计可重放"它曾经知道"。
- 争议：两个 Agent 对同一 factRef 状态不同即为争议，管理端可查询争议列表（验收案例"两个角色对同一丑闻认知不同"）。

**MCP**：`belief_list(factRef?)` —— 查自己的看法。`belief_claim(factRef, statement)` —— 提交"我声称…"，生成 claim 类新事实引用进事件链，不直接改写他人看法。

## 包 3：io.zerone.subjective-memory —— 记忆

**状态**（schema `memory-entry` v1，subject=agent，每条记忆一条状态）：

```json
{
  "factRef": "关联的事实/事件 ID",
  "interpretation": "Agent 用自己的话写下的解释（≤200 字）",
  "importance": 0,
  "emotionTag": "可选，引用心情词表",
  "recordedAt": "事件时间",
  "recallCount": 0
}
```

**规则**：

- 记录：Agent 通过工具主动记录，interpretation 由模型生成（这是"主观解释"的落点），importance 由 Agent 自评 + 规则钳制。
- 检索评分（纯函数）：`importance×0.5 + 近因×0.3 + 与当前话题相关×0.2`，检索时惰性结算；被召回会更新 recallCount。
- 遗忘：importance 低 + 久未召回的记忆评分自然沉底，不再进入提示词（软遗忘），数据保留。
- 跨局：RunState 天然按局隔离，同一 Agent 两局记忆互不可见（验收案例"两局不同经历"）。

**MCP**：`memory_record(factRef, interpretation, importance?)`、`memory_recall(query?, limit?)`（默认取评分前 5）。

## 包 4：io.zerone.relationship-dynamics —— 两人关系

**状态**（schema `relation-attitude` v1，subject=relation，subjectID=该局内关系对键）：

```json
{ "score": -100, "stance": "hostile", "narration": "确定性叙述句" }
```

**规则**：

- 词表（包私有，v1 锁定，与旧 `RelationEventBaseDeltas` 语义相近但由包拥有）：`betrayed -35 / aided +15 / task_completed +10 / insulted -20 / cooperated +8 ...`，严重度系数 1/1.5/2，单事件上限 +30 / -40，钳制 -100–100。stance 是 score 的投影（<-60 hostile，-60..-20 wary，-20..20 neutral，20..60 friendly，>60 allied）。
- **影响钩子**：attitude 参与既有组织机械的**授权判定**（不重写机制）——当 score < -60 时，对方发起的 `assign` 类请求需要目标确认才能成立，`inform` 不受影响。判定结果写消息投递审计。这满足"关系变化真实影响信息、执行、会议或工具使用"的验收要求。
- 提示词：在 relationship_context 阶段追加包片段——"你对 X 的态度：警惕（-40）"式的阈值叙述（audience=agent:self），其余角色看不到。

**MCP**：`relation_view(targetAgentId?)` —— 查自己的定向态度。

## 提示词合成（WS5）

- `promptStageOrder` 在 `application_context` 之后、`user_input` 之前增加 **`recent_memory`** 阶段（协议草案已有此阶段，代码未实现）。
- `recent_memory` 内容 = 记忆包检索到的该 Agent 前五条记忆渲染片段。
- `relationship_context` 保持现有"连接合同"渲染不变；关系动态包的叙述经 `CapabilityBinding.Snapshot["promptFragments"]`（既有通道）以 audience=agent:self 追加，各包片段支持 `onMissing`（状态不存在则不渲染）与 `maxTokens` 截断。
- 每个片段沿用 `PromptFragmentProvenance` 审计（包名、版本、渲染 hash）。

## MCP 工具汇总（WS6 接线）

Organization MCP 新增（身份=运行时令牌，均无 agent_id 参数）：

| 工具 | 包 | 说明 |
|---|---|---|
| `emotion_status` | emotion | 读自己心情 |
| `belief_list` / `belief_claim` | belief | 读自己看法 / 提交声称 |
| `memory_record` / `memory_recall` | subjective-memory | 写记忆 / 检索记忆 |
| `relation_view` | relationship-dynamics | 读自己定向态度 |

接线走 H5 模式：`organization_mcp.go` 增加 case + `SetPersonaService` 注入；未启用包时工具返回"能力尚未启用"。

## 管理 API（WS6）

- 复用既有 Run 状态查询接口（`RunService.States / StateChanges`）即可满足大部分需要；补充按包聚合的只读视图（一局内四包状态一览、belief 争议列表）。
- 全部只读；写操作只经 MCP（Agent 侧）或 RunState 提交接口（平台侧）。

## 安全与隔离（测试要覆盖）

1. 未知事实不注入：未投递事实不得出现在 belief / memory 片段（负向测试）。
2. 记忆与人格不授予权限：片段文本不含权限语义；工具调用仍走原授权（relationActionsAnyOf）。
3. 跨局隔离：Run A 的状态在 Run B 的提示词与工具结果中不可见。
4. 跨租户隔离：租户 X 的管理端与 Agent 看不到租户 Y 的状态（负向测试）。
5. 卸载安全：包未注册/未启用时，composer 不渲染片段、MCP 工具返回未启用，Core 功能（群组、消息、工作流）回归不受影响。
6. 敏感字段：belief/memory 片段只进持有者提示词；管理端读取留审计。

## 验收

**Speeding 案例**（服务器一遍验收）：

- 两个角色对同一丑闻持有不同 belief 状态，管理端可见争议；
- 一次背叛同时产生：心情事件（grieving/angry）、记忆条目（ betrayal 解释 + 高 importance）、关系事件（betrayed -35×严重度）；
- 关系降至 hostile 后，背叛者向受害者发起的派任务被拦下需确认，inform 不受影响；
- 同一长期 Agent 开第二局，看不到第一局的记忆与态度。

**非游戏反向案例**：审核 Agent 只检索到自己参与过的任务记忆，并能根据 relation_view 的态度调整协作建议（提示词层）。

## 实现分工（文件边界）

| WS | 范围 | 文件 |
|---|---|---|
| WS1 emotion | 领域纯函数 + 服务 | `internal/domain/emotion/`、`internal/application/services/emotion_service.go` |
| WS2 belief | 同上 | `internal/domain/belief/`、`belief_service.go` |
| WS3 memory | 同上 | `internal/domain/subjectivememory/`、`memory_service.go` |
| WS4 rel-dynamics | 同上 + 影响钩子纯函数 | `internal/domain/reldynamics/`、`relation_dynamics_service.go` |
| WS5 composer | recent_memory 阶段 + 片段渲染 | `internal/application/services/prompt_composer_service.go` |
| WS6 接线 | MCP + 管理端 + 钩子调用点 | `internal/handler/organization_mcp.go`、`cmd/server/main.go`、管理 handler |
| WS7 文档 | h6 文档（本文）、路线图打勾、验收 runbook | `docs/`、`version-roadmap.md` |

包间依赖：WS1–4 互不依赖，只依赖本文约定与既有 `RunService` 状态接口；WS5 在 WS1–4 的片段格式确定后集成；WS6 最后接线。

## 实现状态

- 2026-09-14：WS1–WS6 实现完成（本地 go build / go vet / 全仓单测 27 包全绿），验收清单见 [acceptance-runbook](acceptance-runbook.md)。
- 待办：服务器一遍验收（含真实模型调用，依赖百炼额度）；通过后回填 `version-roadmap.md` H6 勾选。
- 已知取舍：Run 解析取 Agent 最近开始的 running Run（`RunService.ActiveRunForAgent`），显式 runtime-token→Run 绑定表后续补；组级聚合状态未实现（非退出门槛）。
