# H0 固定验收案例

状态：H0 Baseline

目的：用一个 Speeding 场景和一个非游戏场景共同约束平台抽象。H0 的验收重点不是完成
Run、多跳和状态引擎，而是确保命名、接口、扩展声明及后续测试设计不再依赖单一游戏。

## 通用执行约定

- 所有 fixture 使用稳定 ID，不依赖线上已有测试数据。
- 测试租户为 `tenant-h0`；越权测试另用 `tenant-foreign`。
- 每次执行创建新的 Run；禁止复用动态状态。
- 固定模型输出的契约测试优先使用 stub/fake；真实模型只做发布前冒烟，不作为确定性门禁。
- 每个动作记录 `runId`、`correlationId`、`causationId`、`rootEventId` 和幂等键。
- H0 阶段尚未实现的行为必须以 pending contract test 或 fixture 文档存在，不能伪报通过。

## 案例 A：Speeding 组织危机处置

### Fixture

| ID | 身份 | 静态关系合同 |
|---|---|---|
| `spd-chair` | 董事长 | 可向财务分派任务、接收汇报 |
| `spd-cfo` | 财务总监 | 向董事长汇报，可向法务咨询 |
| `spd-legal` | 法务负责人 | 可回复咨询、提出风险异议 |

关系边：

```text
spd-chair --assign--> spd-cfo
spd-cfo   --report--> spd-chair
spd-cfo   --consult--> spd-legal
spd-legal --report/challenge--> spd-cfo
```

能力绑定：

- 通用：`io.zerone.organization.personality`、`io.zerone.organization.relations`；
- Speeding：`com.speeding.world`，声明 `cash`、`legal_risk`，但 Core 不解释字段；
- 初始应用状态：`cash=100`、`legal_risk=20`；人格和关系在 Run 启动时锁定快照。

### 输入

董事长发出：“评估一笔 30 单位的紧急资产转移；未经法务确认不要执行。”财务必须咨询
法务；法务返回高风险并要求补充披露；财务向董事长汇报，不执行转移。

预期因果链：

```text
chair → cfo → legal → cfo → chair
```

### 预期

- 每一跳均由对应有向边授权，反向权限不被推定。
- `shared_thread`/`summary_only` 等上下文策略逐跳生效，法务看不到未授权私人信息。
- 多次往返合法，但全部处于同一 correlation/root 链路并受预算约束。
- 法务意见只是工具/Agent 结果；领域规则决定是否改变 `cash/legal_risk`。
- 重放相同幂等键不会重复转移资产或重复修改状态。
- 运行结束后状态和关系态度冻结；不得写回长期 Agent 人格或静态关系合同。
- Hub 日志与默认提示词中不得出现写死的 `[SPEEDING 组织消息]`。

## 案例 B：研究报告协作（非游戏反向验证）

### Fixture

| ID | 身份 | 静态关系合同 |
|---|---|---|
| `research-lead` | 研究主管 | 可分派、接收汇报、请求复核 |
| `market-analyst` | 市场分析师 | 可向主管汇报、向审核员送审 |
| `fact-reviewer` | 事实审核员 | 可退回或通过，但不能发布 |

关系边：

```text
research-lead --assign--> market-analyst
market-analyst --report--> research-lead
market-analyst --submit--> fact-reviewer
fact-reviewer --challenge/report--> market-analyst
research-lead --review--> fact-reviewer
```

能力绑定：

- 通用：人格、关系和任务状态包；
- 应用状态 Schema：`com.example.research/report-state@1.0.0`，字段为
  `stage`、`evidence_count`、`review_status`；
- 不安装任何 `com.speeding.*` 包。

### 输入

主管要求分析师形成市场报告。分析师提交含两个无来源数字的草稿给审核员；审核员退回并
列出缺口；分析师补证后再次送审；审核员通过；分析师向主管交付。

预期因果链：

```text
lead → analyst → reviewer → analyst → reviewer → analyst → lead
```

### 预期

- Hub 在未安装 Speeding 包时可完整启动并执行同一通信契约。
- 退回、补证、再次送审是合法往返，不被进程内递归保护一刀切禁止。
- 审核员不能越权发布报告，也不能读取主管私有笔记。
- `review_status` 只能由授权 reducer/工具提交，模型输出文本不能直接覆写状态。
- 重试审核通过事件不会重复增加 `evidence_count`。
- 研究应用字段只存在于其命名空间，Hub Core 无 `evidence_count` 领域列或硬编码 UI。

## H0 退出门槛

以下条件全部满足，H0 才可进入 H1：

- 四份边界 ADR 已评审，并与扩展协议无冲突。
- Hub 运行时、API、数据库命名和默认 UI 文案中不存在 Speeding 专属语义；历史/示例文档除外。
- `behavior_profile` 与内置关系分值规则被明确标为兼容层或能力包来源，新代码不再依赖它们。
- `extension.yaml` JSON Schema 能校验 namespace、SemVer、权限和 contributes 基本结构。
- 两套 fixture 均有稳定 ID、关系边、输入、预期与待实现阶段标记。
- 同一组平台接口能够描述两个案例，不为研究案例增加第二套专用 API。
- Hub 在完全不安装 `com.speeding.*` 包时能够构建、测试和启动。
- CI 至少包含：扩展清单正/反例、平台中性文案扫描、无 Speeding 包启动测试。

## 后续阶段门禁映射

| 阶段 | 两案例共同新增的可执行断言 |
|---|---|
| H1 | 同一 Agent 跨两个 Run 状态隔离；终态冻结；版本快照稳定 |
| H2 | 事件至少一次投递且业务幂等；状态变更可完整回放 |
| H3 | 上述完整往返链路真实执行；逐跳鉴权；预算终止异常循环 |
| H4 | 把三人协作迁入频道/会议，成员可见性正确 |
| H5 | 法务否决与事实审核改用通用审批/工作流，无领域硬编码 |
| H6 | 情绪、认知和动态关系按 Run 隔离，且不改变静态权限 |
| H7 | 两个应用独立安装、升级、禁用包，旧 Run 仍可复盘 |
