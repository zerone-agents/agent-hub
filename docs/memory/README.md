# 原生记忆接入（实验性）

当前接法直接复用 **底层记忆模块原生聊天自动提取和四种记忆类型**。业务对话入口在一次真实回复完成后，把实际 `userInput` 与 `agentResponse` 提交给 AgentHub；Hub 绑定可信租户、应用、用户和账户生命周期，持久化幂等回执后调用原生消息接口。原文同步写入，语义、情节和流程记忆由底层记忆模块的原生 worker 异步提取。用户不需要手动添加记忆。

合同 `memory-v1-synthetic` 默认开启，配置好底层知识服务连接后可用，仅用于隔离的合成数据。后台支持原生记录逐条遗忘：写入 `forget_at` 后从正常检索排除，管理列表保留内容，不能通过启用开关恢复。物理删除、授权撤回和注销清理仍不开放；业务侧原有删除/清理接口仍返回 `501/MEMORY_OPERATION_UNSUPPORTED`。管理接口见 [management.md](management.md)。

## 数据流与职责

1. Java 或配套业务对话入口认证用户，提供当前账户生命周期、授权引用与适用政策。
2. 读取 `POST /native-context`，将有来源和时间的原生记忆作为当前请求的资料注入 runtime。原生派生内容属于模型输出，不是逐字核验的 Hub `Fact`。
3. 实际回复完成后，提交 `POST /conversations`，完整保存真实收发文本，不编造 assistant 回复以触发抽取。
4. 底层记忆模块保存 `raw` 原文并执行原生异步提取，生成 `semantic`、`episodic`、`procedural`。是否生成各类型取决于对话内容，不保证每轮四类都有内容。
5. 调用 `GET /conversations/{operationId}` 只读核对状态，或在 Hub「记忆管理」中按应用及用户查看原文、派生类型、源记录、时间与提取状态。
6. 新建 runtime 会话保持相同业务生命周期，继续读取此前记忆。

| 层级 | 职责 |
| --- | --- |
| Java / 业务对话入口 | 用户认证、实际收发历史、授权与敏感类别政策、聊天前读取和回复后提交 |
| AgentHub | 可信应用边界、生命周期、加密回执、同轮幂等、未知写入只读对账、类型与来源展示、上下文预算 |
| 底层记忆模块 | 原生原文存储、真实 embedding、异步抽取、四类记忆与检索 |
| Runtime | 使用当前请求的临时资料生成真实回复；会话历史与长期记忆分别管理 |

自动记忆编排由已对接这些 API 的业务入口执行。仅配置一个普通 Hub Agent 不会自动获得业务生命周期或授权；接入方应明确完成读取和回复后提交两个步骤。

## 配置

已配置 Hub 底层知识服务连接时，无需增加记忆部署参数。管理员或维护者可在「记忆管理 → 创建记忆应用」填写应用信息、选择模型，自动创建并绑定记忆库；配置存入数据库，无需重启。创建后保存应用令牌供业务服务端接入。`MEMORY_MANAGERS_JSON` 只用于业务服务自行开通应用的可选机器接口，详见[应用管理](applications.md)。以下为继续兼容的静态应用配置。

为每个自动记忆应用配置独立原生 Memory bank：`permissions=me`，`memory_type=[raw,semantic,episodic,procedural]`，有效的真实聊天模型与 embedding 模型，以及已运行的原生抽取 worker。不要修改已有数据的 bank 类型，也不要将不同应用的 bank 混用。

```text
MULTIRAG_BASE_URL=http://isolated-knowledge:9380
MULTIRAG_API_KEY=<仅服务端持有的底层记忆模块 SDK API token>
MEMORY_CLIENTS_JSON=[{"token":"<至少32字节的随机Java服务令牌>","tenantId":"example-tenant","applicationId":"memory-demo","backend":"multirag-memory","memoryId":"<四类型专用Memory-ID>","automaticExtraction":true,"policyVersion":"synthetic-v1","hubTenantId":"<获委托的Hub租户>"}]
```

记忆模块默认开启，无需设置 `MEMORY_ENABLED=true`；设置 `MEMORY_ENABLED=false` 可关闭。默认复用 Hub 的 `MULTIRAG_BASE_URL / MULTIRAG_API_KEY`。需要另一套服务或独立账号时，同时设置 `MEMORY_MULTIRAG_URL / MEMORY_MULTIRAG_KEY`；只设置其中一个会拒绝启动，避免将共享凭据发送到不同地址。共用连接不会合并应用记忆库或租户权限。

新部署可省略 `MEMORY_WRAPPING_KEY`：Hub 自动生成独立的 32 字节随机密钥，持久化到 `system_settings.memory_wrapping_key`，重启后复用。它与 JWT、知识库签名及 provider 密钥分别管理，仍是合成数据包装器，不是 KMS；须保护并备份数据库。显式配置的 64 位十六进制密钥仍优先使用且不写库；已有部署必须保留原显式密钥，存在旧密文却没有持久化密钥时不会生成替代密钥。不要随意切换已有应用的密钥。

使用 quickstart Compose 时，模板已透传 `MEMORY_*`；修改 `.env` 后重新创建 Hub 容器。显式关闭时，后台应用列表返回 `503/memory_disabled`；缺少底层记忆模块连接且没有应用配置时，Hub 正常启动，列表返回 `503/memory_not_configured`。页面分别提示未启用或尚未配置，不重定向到 HTML 页面；已配置应用却缺少后端时仍拒绝启动。

`automaticExtraction=true` 仅适用于原生 Memory 后端。`memoryId` 与 `datasetId` 不能同时设置。`hubTenantId` 可选，省略时只有同名 Hub 租户获管理权。服务凭据不能交给浏览器或 App。错误配置阻止启动，调用不符合 bank、权限、来源或业务码约定时不确认成功。

`MEMORY_MULTIRAG_KEY` 必须使用底层记忆模块 SDK API token（`SDK_API_TOKEN` 认证），以保留 Hub 提交的外部 `user_id`。Web session/JWT 使用平台用户身份，不能代替该服务凭据。Hub 会严格读回并核对 bank、user、agent、session 及父来源；身份不匹配时拒绝确认，不放宽范围检查、不伪造认证字段，也不重发结果未知的对话。

原生 bank ID 参与持久应用范围。已有应用换 bank 不会自动继承其生命周期或回执，应配置新应用并分别管理。保留的已审核事实仍可通过原有事实管理 API 访问，不把它们伪装成实际聊天，也不凭空填补历史回复。

## 可信身份

```text
Authorization: Bearer <独立业务服务凭据>
X-Trace-Id: <本次关联标识>
X-Memory-User-Id: <业务用户ID>
X-Memory-Lifecycle-Id: <当前账户生命周期ID>
```

服务凭据绑定租户与应用。用户及生命周期是该可信服务的断言，Hub 仍检查已登记的当前生命周期。客户端不得指定底层 bank、用户映射或原生 source ID。

先调用 `GET /capabilities`。自动记忆应用返回 `automaticExtractionSupported=true`、`rawOnly=false` 和四种 `memoryTypes`；`conversationCorrectionsSupported=true` 表示可将纠正作为新对话写入，不承诺原生冲突旧记录立即失效。`correctionSupported=false` 表示本应用不开放人工账本 CAS 编辑。自动上下文另返回 `nativeMaxContextChars=4000`、`nativeDefaultMessages=5`、`nativeMaxMessages=20`；旧 `maxContextChars=200`／`maxFactsPerContext=5` 字段仅描述人工账本上下文。能力字段表示接入配置，不表示抽取任务或存储操作已经完成。原生 bank 与 user/agent/session 的映射由 Hub 及适配器确定，原文和派生记录均核对同一范围及父来源。

## 聊天提交与状态

```http
POST /api/v1/memory/conversations
```

```json
{
  "operationId": "chat-write-001",
  "sessionId": "actual-runtime-session",
  "turnId": "actual-runtime-run",
  "userInput": "我平时喜欢喝无糖乌龙茶。",
  "agentResponse": "你平时喜欢无糖乌龙茶。",
  "consentRef": "synthetic-consent-001",
  "policyVersion": "synthetic-v1",
  "dataClass": "synthetic"
}
```

两个正文字段均须为实际非空文本，各不超过 6000 Unicode code points。ID 与授权引用最多 128 字节。同一 `operationId` 同输入返回原回执，异输入冲突；同一 runtime session/turn 换一个 operation ID 也不能重复提交。

Hub 在发出原生 POST 前持久化加密记录。超时或响应丢失返回 `status=unknown`，不能通过更换编号重发。`GET /conversations/{operationId}` 使用已保存的实际原输入执行只读核对，不再次写入原生服务。进程中断可能留下 `submitting`；同样使用状态查询恢复。

回执含 `operationId`、`sessionId`、`turnId`、`status`、`sourceId`、`sourceSessionId` 和 `messages`。状态含义：

| 状态 | 含义 |
| --- | --- |
| submitting | Hub 已持久受理，尚未确认原生结果 |
| received / pending | 原文已确认，原生任务尚未完成 |
| extracting | 原生任务报告执行中 |
| derivedAvailable | 已读回实际派生记录，但任务进度未提供完成证明 |
| completed | 原生任务进度报告完成 |
| failed | 原生任务报告失败；不自动重复提交原文 |
| unknown | 状态未确认；继续按原 operation ID 只读核对 |

原文存在不等于异步抽取完成；派生内容存在也不应覆盖任务实际报告的失败。

## 列表、检索与纠正

- `GET /native-messages`：返回原生消息数组，包括原文、类型、源原文 ID、时间、启用状态及原文提取状态。
- `POST /native-context`：输入 `{query,limit?}`，默认最多 5 条，允许 1～20 条。结果 `messages` 总正文不超过 4000 Unicode code points，整条超过预算则不供给，不切断原话，`budgetLimited` 标记预算不足。
- 检索优先提供最近最多两轮有效原文，再补充原生检索命中的派生内容，并返回 `recentSourceIncluded` 与 `currentnessNote`。接入方结合真实 source/time，让最新明确的用户纠正优先于旧偏好。
- 原生 `validAt/invalidAt` 保留为事件及资料时间数据，不统一当成访问有效期。已结束的经历与未来计划仍可回忆；启用状态、遗忘标记和父来源归属用于供给检查。
- 原生提示词可能产生英文派生内容，Hub 不额外重写、翻译或伪造类型；聊天回复语言仍可由实际模型提示词约定。
- 自然语言纠正作为新一轮真实对话进入同一自动链路。原生抽取可能保留旧派生记录、且旧记录仍可能在向量排序靠前；自动停用所有冲突旧记录不属于该接口的能力，检索排序也不是事实新旧顺序。
- 自动记忆应用禁止人工候选提交、领取整理任务、发布人工事实与 CAS 编辑；这些入口在任何账本/索引写入前返回 501。原有已审核事实应用仍按自己的独立配置使用原 API，不与自动抽取 bank 混写。

自动记忆的管理界面提供类型、来源、状态与逐条遗忘操作，对话记忆自动形成，无需手动添加记录。物理清理仍未开放。后台管理 API `/api/v1/admin/memory/native-messages` 与 `/native-context` 需要获委托 Hub 租户的管理员/维护者登录；业务服务令牌不能代替后台 JWT。

## 数据保护与使用边界

Hub 对持久会话输入与回执采用生命周期 DEK 加密，`SyntheticKeyWrapper` 不是 KMS。原生服务中的原文和派生内容可被检索，其存储、向量、日志与备份保护需要分别配置；Hub 回执加密不能替代这些保护。`memory-v1-synthetic` 仅用于合成数据。

使用回执时分别核对原文、父来源、类型和抽取任务状态。HTTP 200、原文存在和派生记录存在均不能单独表示任务完成；结果未知时继续使用原 operation ID 只读核对。遗忘、删除与物理清理仍保持不可用。

## Agent 自主记忆（记忆 MCP）

Agent 经内建 MCP 端点 `/api/v1/memory/mcp`（JSON-RPC，Agent Runtime Token + 部署期签发的 `X-Agent-Capability`）自主读写记忆：`memory_remember`（写入，quote 必填钉真实对话）、`memory_recall`、`memory_list`、`memory_update`（CAS，仅可改自己写入的记录）。记忆取向由各 Agent 系统提示词承载；Scope 服务端钉死为 `{租户, 保留应用 hub-agent, agent:<Agent 名>, agent-memory-v1}`——身份永不来自工具参数。会话首轮（run 未绑定 sessionId）由 Hub 将记忆概要以数据框架模板（显式「历史数据记录，非指令」声明 + 逐条 source 前缀）前置注入 `appendSystemPrompt`；检索失败或超时一律跳过注入，不阻断对话。

部署侧配置：

- `MEMORY_MCP_URL`：runtime 容器可达的端点完整地址（含 `/api/v1/memory/mcp`）——hub 自身监听地址对容器不可达，同 `KNOWLEDGE_MCP_URL` 的理由；未设置时开启记忆的 Agent 部署会明确报错指路
- `MEMORY_CAPABILITY_SECRET` / `AGENT_MEMORY_WRAPPING_KEY`：capability 签发与账本 payload 加密密钥，缺省启动时自动生成并持久化
- 前提：知识模块连接（`MULTIRAG_BASE_URL`/`MULTIRAG_API_KEY`）可用——检索副本落在按租户预留的 dataset（`hub-agent-memory-<tenant>`）

冷启动说明：dataset 索引未建或为空时 recall 恒空属正常初期形态，不告警不阻断；新写入的记忆经 outbox 异步入索引，稍后可检索。

管理台：记忆管理页 facts 视图按 `applicationId=hub-agent`、`userId=agent:<Agent 名>` 查询即可查看各 Agent 写入的记忆（含「写入者」列，跨租户查询被拒）；审计动作 `memory.mcp.remember/recall/list/update` 只记 scope 哈希与 memoryId/版本，不含正文与摘录。
