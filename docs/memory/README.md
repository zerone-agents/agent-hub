# 原生记忆接入（实验性）

当前接法直接复用 **MultiRAG 原生聊天自动提取和四种记忆类型**。业务对话入口在一次真实回复完成后，把实际 `userInput` 与 `agentResponse` 提交给 AgentHub；Hub 绑定可信租户、应用、用户和账户生命周期，持久化幂等回执后调用原生消息接口。原文同步写入，语义、情节和流程记忆由 MultiRAG 的原生 worker 异步提取。用户不需要手动添加记忆。

合同 `memory-v1-synthetic` 默认关闭，仅用于隔离的合成数据。遗忘、物理删除、授权撤回和注销清理不开放；原生应用对应操作返回 `501/MEMORY_OPERATION_UNSUPPORTED`，不产生删除成功回执。管理接口见 [management.md](management.md)。

## 数据流与职责

1. Java 或配套业务对话入口认证用户，提供当前账户生命周期、授权引用与适用政策。
2. 读取 `POST /native-context`，将有来源和时间的原生记忆作为当前请求的资料注入 runtime。原生派生内容属于模型输出，不是逐字核验的 Hub `Fact`。
3. 实际回复完成后，提交 `POST /conversations`，完整保存真实收发文本，不编造 assistant 回复以触发抽取。
4. MultiRAG 保存 `raw` 原文并执行原生异步提取，生成 `semantic`、`episodic`、`procedural`。是否生成各类型取决于对话内容，不保证每轮四类都有内容。
5. 调用 `GET /conversations/{operationId}` 只读核对状态，或在 Hub「记忆管理」中按应用及用户查看原文、派生类型、源记录、时间与提取状态。
6. 新建 runtime 会话保持相同业务生命周期，继续读取此前记忆。

| 层级 | 职责 |
| --- | --- |
| Java / 业务对话入口 | 用户认证、实际收发历史、授权与敏感类别政策、聊天前读取和回复后提交 |
| AgentHub | 可信应用边界、生命周期、加密回执、同轮幂等、未知写入只读对账、类型与来源展示、上下文预算 |
| MultiRAG | 原生原文存储、真实 embedding、异步抽取、四类记忆与检索 |
| Runtime | 使用当前请求的临时资料生成真实回复；会话历史与长期记忆分别管理 |

自动记忆编排由已对接这些 API 的业务入口执行。仅配置一个普通 Hub Agent 不会自动获得业务生命周期或授权；接入方应明确完成读取和回复后提交两个步骤。

## 配置

为每个自动记忆应用配置独立原生 Memory bank：`permissions=me`，`memory_type=[raw,semantic,episodic,procedural]`，有效的真实聊天模型与 embedding 模型，以及已运行的原生抽取 worker。不要修改已有数据的 bank 类型，也不要将不同应用的 bank 混用。

```text
MEMORY_ENABLED=true
MEMORY_MULTIRAG_URL=http://isolated-multirag:9380
MEMORY_MULTIRAG_KEY=<仅服务端持有的MultiRAG SDK API token>
MEMORY_WRAPPING_KEY=<32随机字节的64位十六进制包装密钥>
MEMORY_CLIENTS_JSON=[{"token":"<至少32字节的随机Java服务令牌>","tenantId":"example-tenant","applicationId":"memory-demo","backend":"multirag-memory","memoryId":"<四类型专用Memory-ID>","automaticExtraction":true,"policyVersion":"synthetic-v1","hubTenantId":"<获委托的Hub租户>"}]
```

`automaticExtraction=true` 仅适用于原生 Memory 后端。`memoryId` 与 `datasetId` 不能同时设置。`hubTenantId` 可选，省略时只有同名 Hub 租户获管理权。服务凭据不能交给浏览器或 App。错误配置阻止启动，调用不符合 bank、权限、来源或业务码约定时不确认成功。

`MEMORY_MULTIRAG_KEY` 必须使用 MultiRAG SDK API token（`SDK_API_TOKEN` 认证），以保留 Hub 提交的外部 `user_id`。Web session/JWT 使用平台用户身份，不能代替该服务凭据。Hub 会严格读回并核对 bank、user、agent、session 及父来源；身份不匹配时拒绝确认，不放宽范围检查、不伪造认证字段，也不重发结果未知的对话。

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

自动记忆的管理界面是类型、来源和状态视图，没有手动添加的前置步骤，也没有遗忘、删除或物理清理入口。后台管理 API `/api/v1/admin/memory/native-messages` 与 `/native-context` 需要获委托 Hub 租户的管理员/维护者登录；业务服务令牌不能代替后台 JWT。

## 数据保护与使用边界

Hub 对持久会话输入与回执采用生命周期 DEK 加密，`SyntheticKeyWrapper` 不是 KMS。原生服务中的原文和派生内容可被检索，其存储、向量、日志与备份保护需要分别配置；Hub 回执加密不能替代这些保护。`memory-v1-synthetic` 仅用于合成数据。

使用回执时分别核对原文、父来源、类型和抽取任务状态。HTTP 200、原文存在和派生记录存在均不能单独表示任务完成；结果未知时继续使用原 operation ID 只读核对。遗忘、删除与物理清理仍保持不可用。
