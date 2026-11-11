# 应用与记忆库管理 API（实验性）

业务服务可自行创建多个应用。每个应用由 Hub 创建并绑定独立的底层记忆模块原生记忆库，并获得独立应用令牌；用户及账户生命周期继续在应用内部隔离。全部接口仍限隔离环境的合成数据，沿用 [原生聊天接入合同](README.md)。后台可逐条遗忘原生记录；物理删除、撤权和注销清理仍未开放。

## 从后台创建（推荐）

Hub 已配置底层知识服务连接时，管理员或维护者打开「记忆管理 → 创建记忆应用」，填写名称、应用标识和说明，选择可用的提取模型、向量模型即可。Hub 自动创建私有记忆库、绑定当前登录租户并生成独立应用令牌，配置存入数据库，立即生效。无需设置 `MEMORY_MANAGERS_JSON`、`MEMORY_CLIENTS_JSON` 或重启容器。

创建后在关闭弹窗前复制应用令牌，只交给可信业务服务端。列表和配置详情不返回令牌。创建结果未确认时，使用原操作核对，不另建应用来重试。记忆库模型在创建后固定；已有应用配置仍为只读。

后台接口使用已有 Hub 登录 JWT 和管理员/维护者权限，不接受请求体指定租户、委托关系或现有记忆库。普通成员无权创建。业务租户和管理租户均为当前登录租户；每个租户最多创建 32 个应用（计入静态占位）。应用隔离和合成数据合同保持不变。

| 方法 | 后台路径 | 用途 |
|---|---|---|
| GET | `/api/v1/admin/memory/application-models` | 从当前记忆服务账号读取已启用的提取和向量模型，保留模型实例引用 |
| POST | `/api/v1/admin/memory/applications` | 请求 `operationId/applicationId/name/description/chatModel/embeddingModel`，创建并返回 `application/applicationToken` |
| GET | `/api/v1/admin/memory/applications` | 返回当前后台租户可管理的应用，包括待确认状态，不返回令牌 |
| GET | `/api/v1/admin/memory/applications/{applicationId}/provisioning` | 只读核对当前租户创建的应用状态，不重发建库请求、不返回令牌 |

创建返回 Hub 标准 `{success:true,data:…}` 信封，成功为 HTTP 200，待确认为 HTTP 202。原创建请求的精确重放可读取同一回执和令牌，不重复创建库；修改同一操作的模型或其他参数返回冲突。即使模型目录后来不可用，已有回执仍可核对。页面仅在当前弹窗内存中保留令牌，不写入浏览器持久存储。

## 业务服务自动创建的可选配置

```text
MULTIRAG_BASE_URL=http://isolated-knowledge:9380
MULTIRAG_API_KEY=<底层记忆模块 SDK API token，不能使用网页登录 JWT>
MEMORY_CLIENTS_JSON=[]
MEMORY_MANAGERS_JSON=[{"token":"<至少32字节的随机租户管理令牌>","tenantId":"example-tenant","hubTenantId":"example-hub-tenant","chatModel":"<对应服务账号的聊天模型引用>","embeddingModel":"<对应服务账号的embedding模型引用>","maxApplications":32}]
```

只有业务服务需要脱离后台登录、自行批量开通应用时，才配置上面的租户管理凭据。记忆默认开启，连接默认复用；独立服务/账号需要同时配置 `MEMORY_MULTIRAG_URL` 和 `MEMORY_MULTIRAG_KEY`。新部署包装密钥默认自动生成并持久化，旧部署保留原显式密钥；详细规则见[接入配置](README.md#配置)。动态应用、记忆库绑定及 Hub 生成的应用令牌继续存入数据库，无需逐个增加环境变量；管理令牌仅用于以下机器接口的租户级启动授权，不是页面创建的前置条件。

租户管理令牌仅交给可信业务服务端，不能交给浏览器或移动端。每个管理令牌固定绑定一个业务租户，`hubTenantId` 固定委托管理后台租户，省略时与业务租户相同。模型来自该底层记忆模块 SDK 账号可用的配置，推荐使用数值模型 ID；Hub 不替调用方创建模型或启动抽取 worker。`maxApplications` 默认为 32，可配置 1～1000，包括该租户保留的静态应用占位。

配置管理令牌后重启 Hub 一次。之后新建、修改或停用应用均不要求重启 Hub。旧 `MEMORY_CLIENTS_JSON` 应用可继续使用，但不能由这些接口覆盖、迁移或删除，列表只包含 API 创建的应用。保留包装密钥及 Hub 数据库；更换包装密钥将影响已有生命周期和创建回执解密。凭据用途及库占位会持久保留，静态凭据轮换不会释放旧令牌供其他应用或管理用途复用。

## 接口

请求头：`Authorization: Bearer <租户管理令牌>`、`X-Trace-Id: <关联ID>`；写请求使用 `Content-Type: application/json`。接口不需要用户或生命周期头，管理令牌不能直接调用聊天接口，应用令牌也不能调用应用管理接口。

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/v1/memory/applications` | 创建应用、建原生库并绑定，返回独立应用令牌 |
| GET | `/api/v1/memory/applications?offset=0&limit=50` | 查询本租户创建的应用；limit 为 1～100 |
| GET | `/api/v1/memory/applications/{applicationId}` | 查看本租户应用详情，不访问原生服务 |
| PATCH | `/api/v1/memory/applications/{applicationId}` | 以版本条件修改名称、描述或启停状态 |
| GET | `/api/v1/memory/applications/{applicationId}/provisioning` | 只读核对原生建库结果，确认后完成 Hub 绑定 |

创建请求：

```json
{
  "operationId": "create-assistant-001",
  "applicationId": "assistant-a",
  "name": "学习助手",
  "description": "隔离合成数据联调",
  "policyVersion": "synthetic-v1"
}
```

成功使用现有业务接口信封 `{ "code": 200, "data": ..., "traceId": ... }`。创建的 `data` 为 `{ "application": { ... }, "applicationToken": "..." }`，其他接口不返回令牌、令牌摘要或密文。调用方应安全保存应用令牌；原创建请求的精确重放可取回相同令牌。没有额外的“取密钥”接口。

应用详情包括租户、应用标识、名称、描述、政策、后台委托、原生库 ID 与不可变库名、实际核对的模型引用、`status`、`version` 和时间。库具有 `permissions=me`、`storage_type=table` 及 `raw/semantic/episodic/procedural` 四种类型。原生库名是随机的 `hub-...` 标记，用于可靠对账；Hub 展示名称可独立修改。创建成功只证明库配置已读回，不能证明模型调用或抽取 worker 已就绪。

将返回的 `applicationToken` 用于现有 `/api/v1/memory/capabilities`、`/lifecycle/activate`、`/native-context` 和 `/conversations` 等接口；用户与生命周期仍通过可信请求头传递。创建的应用会立即进入获委托 Hub 租户的记忆管理应用列表。各应用使用不同库，同名用户在不同应用间也不共享记忆。

## 幂等与结果未知

`applicationId` 在租户内唯一。`operationId` 在租户内唯一；同应用、同操作、同请求精确重放返回原结果，不再发建库 POST。重复应用换操作、同操作换输入或静态应用冲突返回 `409`。名称为 1～64 个 Unicode 字符，描述最多 1024 个字符，不含控制字符；标识最多 128 字节。

Hub 在调用底层记忆模块前持久保存不可预测库名和模型配置。原生建库接口没有幂等键，重名时还可能自动更名，因此 Hub 对一个创建操作只派发一次 POST。超时、响应丢失、进程中断或无法核对配置时，状态为 `provisioning` 或 `unknown`；创建可能返回 `202` 及原令牌，但此时令牌不能进入聊天链路。调用原创建请求或 `/provisioning` 只执行原生列表及配置读取，核对唯一精确库名、私有权限、类型及模型后才转为 `active`。

查询不到库、分页不完整、同名歧义或配置不符均不会重建。即使进程在持久受理后、实际发送前退出，也需要运维核实；本期不提供强制重发、重绑或自动补偿删除，以免误建第二个库。未知状态不表示已失败，也不表示库不存在。

## 修改与停用

```json
{
  "expectedVersion": 2,
  "name": "新的展示名称",
  "description": "更新后的描述",
  "enabled": false
}
```

至少提供一个修改字段。版本过期或库尚未确认绑定返回 `409`。不能修改应用 ID、租户、后台委托、政策、模型或原生库；需要不同配置时创建新应用。每次成功修改增加 `version`。

`disabled` 会拒绝之后的应用令牌请求，后台和租户管理接口仍可查看，使用最新版本和 `enabled=true` 可恢复。获委托 Hub 管理员仍保留原有诊断和记忆状态管理权限。不取消已经受理的请求或底层记忆模块后台任务，不改变用户生命周期，也不代表遗忘、删除或副本清理完成。

错误沿用业务接口的 `error.reason`，包含 `MEMORY_MANAGER_UNAUTHORIZED`（401）、`MEMORY_APPLICATION_INVALID`（400）、`MEMORY_APPLICATION_NOT_FOUND`（404）、`MEMORY_APPLICATION_CONFLICT`／`MEMORY_APPLICATION_QUOTA`（409）及 `MEMORY_APPLICATION_UNAVAILABLE`（503）。所有响应为 `Cache-Control: no-store`；不得记录创建返回的令牌或包含凭据的配置。

## 可复现验证

常规检查：`go test ./internal/memoryapp ./internal/config ./internal/handler ./cmd/server ./internal/infrastructure/nativememory`，以及对应包的 `go test -race`。原生适配的 HTTP 故障测试覆盖响应丢失、业务码失败、完整分页、重名与配置不匹配，注册表覆盖并发、重启恢复、租户隔离、历史凭据用途和版本冲突。

真实 MySQL 检查使用 `MEMORY_APPLICATION_TEST_MYSQL_DSN`，仅接受 loopback TCP 及空的 `hub_memory_app_test_<8～32位hex>` 专用 schema；创建和清理由调用方负责。CI 会准备并移除此测试 schema。

真实底层记忆模块建库验收需预先登记独立资源目录，在目录中保存 `resources.json`，设置 `HUB_MEMORY_LIVE_DIRECTORY`、唯一的 `HUB_MEMORY_LIVE_RUN`、`MEMORY_MULTIRAG_URL`、`MEMORY_MULTIRAG_KEY`、`MEMORY_WRAPPING_KEY`、`HUB_MEMORY_LIVE_CHAT_MODEL`、`HUB_MEMORY_LIVE_EMBEDDING_MODEL`，可选 `HUB_MEMORY_LIVE_OWNER`，再运行 `go test ./cmd/server -run '^TestMemoryApplicationsLive$' -count=1 -timeout=120s`。凭据使用环境变量或受限文件，避免放入 shell 历史。该检查创建两个空库及四个合成生命周期，保存私有创建回执，验证独立读回和新注册表实例的准入，最后停用应用并保留空库；不写入对话，不证明模型执行或抽取效果。已受理后不得更换 run 编号盲目重跑；按原持久记录核对。
