# Configuration

Environment variable reference. All configuration is loaded via Viper (environment variables take the highest priority).

## Server

| Variable | Required | Default | Description |
|---|---|---|---|
| `SERVER_HOST` | No | `0.0.0.0` | Listen address |
| `SERVER_PORT` | No | `8081` | Listen port |
| `SERVER_CORS_ORIGINS` | No | Allow all | Comma-separated CORS allowlist; **strongly recommended to set explicitly** |

## Database

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | ✅ | — | MySQL DSN |
| `DATABASE_MAX_IDLE` | No | `10` | Connection pool idle count |
| `DATABASE_MAX_OPEN` | No | `100` | Connection pool upper limit |
| `DATABASE_MAX_LIFETIME` | No | `3600` | Maximum connection lifetime (seconds) |

## Authentication

agent-hub ships two interchangeable auth backends, selected by `AUTH_MODE`.

- **`builtin`** (default): a self-contained username/password user system. Open-source deployments work out of the box with no external identity provider. First visit shows a setup screen to create the fixed-username `admin` account; further users are added via one-time admin-issued invite links. Roles are `admin` / `maintainer` / `member`.
- **`casdoor`**: delegate authentication to an existing Casdoor SSO. Required for the hosted SaaS and enterprise private deployments that already run Casdoor.

| Variable | Required | Default | Description |
|---|---|---|---|
| `AUTH_MODE` | No | `builtin` | Auth backend: `builtin` or `casdoor` |
| `AUTH_JWT_SECRET` | ✅ (builtin) | — | JWT signing secret for builtin mode; ≥32 bytes. Generate with `openssl rand -hex 32`. Ignored in casdoor mode. |

### Casdoor (only when `AUTH_MODE=casdoor`)

| Variable | Required | Default | Description |
|---|---|---|---|
| `CASDOOR_ENDPOINT` | ✅ | — | Casdoor service address |
| `CASDOOR_CLIENT_ID` | ✅ | — | OAuth Client ID |
| `CASDOOR_CLIENT_SECRET` | ✅ | — | OAuth Client Secret |
| `CASDOOR_CERTIFICATE` | ✅ | — | JWT verification certificate |
| `CASDOOR_ORGANIZATION` | No | — | 可选。仅从旧版本升级且存量数据无法自动归属租户时，作为回填目标的显式覆盖（一次性升级逃生舱，完成迁移后可移除）。正常运行不消费该配置——租户来自登录 token 的组织（owner） |
| `CASDOOR_CALLBACK_URL` | No | — | OAuth callback URL |

> Roles are managed locally by agent-hub; the Casdoor roles claim in the JWT is ignored entirely. Casdoor only provides user identity (authentication). See the multi-tenant section below.

#### 多租户与角色管理（仅 casdoor 模式）

- **租户 = Casdoor 组织**：token 中的组织（owner）即租户 ID，业务数据按租户隔离。组织的创建与配置在 Casdoor 侧完成，agent-hub 不接管。
- **角色由 agent-hub 本地管理**：角色真实源是本地 `user_identities` 租户成员表（Role + Status pending/active），Casdoor 仅提供用户身份。JWT 中的 Casdoor roles claim 完全忽略。`CASDOOR_ROLE_MAPPING` / `CASDOOR_DEFAULT_ROLE` 环境变量已废弃（检测到仅打 warning，不影响启动）；升级后 Casdoor 侧的 `agent-hub-*` 角色可手动删除。
- **新用户待审批流程**：新用户首次登录成功后自动创建 pending 记录，前端渲染「等待审批」页（可访问 /auth/userinfo 与 logout，其余 API 返回 403 PENDING_APPROVAL）。admin 在用户管理页为其分配角色后自动转为 active。
- **admin 锚定 Casdoor 组织管理员**：本地 admin 资格与 Casdoor 组织管理员（IsAdmin）双向同步——组织管理员登录/CLI 身份核对时自动成为本地 admin；被取消组织管理员则本地 admin 撤销为待审批。admin 任命/降级采用「Casdoor 先行」双写：先成功修改 Casdoor is_admin，再写本地。
- **用户管理（admin）**：列表来自本地成员表（禁用状态实时查 Casdoor）；审批 = 给 pending 用户分配角色（自动转 active）；禁用/重置密码直通 Casdoor；创建用户引导至 Casdoor 注册页。邀请制接口仅 builtin 模式可用。
- **升级指引（breaking）**：升级到此模型后所有现有用户变为待审批；Casdoor 组织管理员登录后自动成为 admin，再逐个为其他用户分配角色。业务数据方面：升级时存量 agents / providers / AIGC 配置等自动回填——**无需任何配置**，回填租户从 `user_identities` 自动推断（恰好一个组织登录过即推断为该组织）；聊天记录按 `user_id → user_identities` 映射回填到各用户所属租户，映射不到的兜底回填推断租户。仅在存量数据无法自动归属（`user_identities` 为空或含多个组织）时，启动会报错指引**临时**配置 `CASDOOR_ORGANIZATION` 指定回填目标，完成本次一次性迁移后即可移除。
- **已知限制**：
  - JWT 无吊销通道：admin 在 Casdoor 侧被降级后，其未过期的 access token 在过期前仍有效；CLI token 最迟 5 分钟内经身份缓存纠正。
  - 用户列表只显示登录过的用户（列表数据源为本地成员表，而非直通 Casdoor）。
- **部署要求**：agent-hub 使用的 Casdoor Application 需要所在组织的用户管理权限（读写用户、组织管理员标志）。CasdoorDirectory 与 CasdoorProvider 复用同一份 `CASDOOR_*` 配置。

##### 数据隔离语义

业务表（agents / cloud_sessions / cloud_messages / provider_summaries / tools / mcp_servers / skills / scenes / aigc_configs）均含 `tenant_id` 列并按租户隔离：

- **tenant_id = Casdoor 组织名**：与登录 token 的组织（owner）一致；builtin 模式恒为 `default`。
- **哨兵约定**：业务表 `tenant_id` 列的默认值为空串 `''`（共享哨兵，对齐全仓 tools/mcp/skills/scenes/providers/aigc 的约定）——写入时由代码显式盖章请求租户，不依赖数据库默认值；空串仅在共享行（内置/种子数据）合法。
- **内置/种子数据全局共享**：内置 tools、MCP servers、skills、scenes 及共享 provider 种子行的 `tenant_id` 为空串（全局共享行）——各租户均可读、不可修改；租户编辑共享 provider 时按 copy-on-write 复制为本租户行后再改动，原共享行不受影响。
- **存量数据回填**：从旧版本升级时，启动迁移把存量业务数据回填到归属租户（builtin 模式 → `default`；casdoor 模式 → 从 `user_identities` 自动推断的唯一组织，可用 `CASDOOR_ORGANIZATION` 显式覆盖）。系统形态上原生多租户：正常运行不依赖任何组织配置，`CASDOOR_ORGANIZATION` 仅在存量数据无法自动归属（0 或多个组织登录过）时作为一次性升级逃生舱存在。
- **同名资源跨租户共存**：原全局唯一约束（如 agents 的 `uk_name`、provider_summaries 的 `uk_key`）已改为 `(tenant_id, name)` / `(tenant_id, key)` 复合唯一索引——不同租户可各自持有同名 agent / 同 key provider。
- **聊天数据两级隔离**：cloud_sessions / cloud_messages 首先按租户隔离，会话列表在租户内再按用户隔离（member 仅见自己的会话，admin/maintainer 可见本租户全部会话）。
- **AIGC 配置纯 per-tenant**：每个运营主体（租户）各自配置自己的 aigc_configs 行（ContentProducer 主体编码等）；未配置的租户不注入 AIGC 标识，不存在跨租户的共享默认回退。写操作（Save / RotateKey / Delete）只作用于本租户行，且拒绝在租户身份缺失（builtin 之外的空租户上下文）时执行。升级时，旧"全局一份"时代的遗留共享行自动归属回填租户（显式指定或自动推断的唯一租户）；若该租户已保存自有配置，则丢弃遗留共享行、保留租户自有配置。
- **knowledge MCP 链（runtime token）**：以 runtime token 访问知识库时，租户取自所命中的 agents 行的 TenantID，与操作者无关。
- **Kong 对账为全局语义**：后台 Kong 路由对账任务扫描全表（agent ID 全局唯一），不受租户隔离过滤影响。

#### 角色权限矩阵

管理台（`/api/v1/admin/*`）按角色开放如下（builtin 与 casdoor 模式语义一致）：

| 权限 | admin | maintainer | member |
|---|---|---|---|
| 用户管理（用户列表/审批/邀请/重置密码） | ✅ | — | — |
| 管理写操作（创建/编辑/删除 agent、tool、MCP、skill、scene、provider、知识库） | ✅ | ✅ | — |
| 敏感读（AIGC 配置与密钥、agent 运行时文件内容） | ✅ | ✅ | — |
| 非敏感 GET（agent/tool/MCP/skill/scene/provider/知识库列表与详情、部署状态 `deploy`） | ✅ | ✅ | ✅ |
| 聊天历史会话（查看/删除） | ✅ 全部会话 | ✅ 全部会话 | ✅ 仅自己的会话 |
| 普通聊天（公开 `/agents` 聊天接口） | ✅ | ✅ | ✅ |
| CLI token 自管理（`/cli`） | ✅ | ✅ | — |

要点：
- **member 只读边界**：member 可读管理台的非敏感数据（列表/详情/知识库/部署状态 `deploy`，含 runtimeUrl/apiKey——已确认不裁剪），但管理写操作（POST/PUT/PATCH/DELETE，含探活、部署启停）与敏感读（AIGC 配置含密钥、agent 运行时文件内容 `files/content`）仍返回 403——中间件是权限墙，前端按钮隐藏仅 UX。例外：member 可删除自己的聊天会话（见下）。
- **聊天会话按用户隔离**：member 仅可查看/删除自己的聊天会话；admin/maintainer 可见并管理全部会话。
- **用户管理仅 admin**：审批/角色分配/邀请/重置密码保持 admin 专属（`RequireAdmin`）。
- **路径未改名**：全部保持 `/api/v1/admin/*`，仅内部按 write（admin/maintainer）/ read（admin/maintainer/member）分组注册。
- **CLI token 收口**：`/api/v1/cli` 仅 admin/maintainer 可用（member 403），不属于 `/admin` 面但同样由角色中间件拦截；member 历史已签发的 token 不主动吊销，自然过期后无法续签。
- **member 部署按钮**：member 在 Agent 页可见部署按钮，弹窗内仅「聊天」可用，其他操作按钮置灰。

## Operations (Ops API)

运维端点（`/api/v1/ops/*`，当前用于多组织 OAuth client 登记，见下文 runbook）的鉴权开关：

| Variable | Required | Default | Description |
|---|---|---|---|
| `OPS_API_KEY` | No | — | 运维 API 鉴权密钥。**空（默认）= 运维端点不挂载**，请求 `/api/v1/ops/*` 等效 404；配置后所有请求需携带请求头 `X-Ops-Key: <OPS_API_KEY>`，匹配放行，否则拒绝。仅在需要接入新组织时配置。 |

### 多组织接入 runbook（仅 casdoor 模式）

单组织部署（一个 Casdoor 组织 = 一个租户）**无需任何操作**：租户 client 表为空时自动回落 env 全局 `CASDOOR_CLIENT_ID` / `CASDOOR_CLIENT_SECRET` / `CASDOOR_CERTIFICATE`，行为与旧版本完全一致。只有当第二个及以后的组织（租户）需要通过各自 Casdoor Application 登录时，才按以下三步操作。

#### Step 1：Casdoor 侧建组织与应用

每个组织（如 `acme`）在 Casdoor 中：

1. 创建组织 `acme`（组织即租户，业务数据按组织隔离）。
2. 在该组织下创建一个 Casdoor Application，回调（redirect URL）配置为 hub 全局回调 `CASDOOR_CALLBACK_URL`（无需 per-org 回调），记录其 Client ID / Client Secret。
3. 无需创建 `agent-hub-admin` / `agent-hub-maintainer` / `agent-hub-member` 角色——agent-hub 的角色以本地成员表（`user_identities`）为准，Casdoor 侧角色不被消费（Casdoor 仅提供用户身份，**Casdoor 组织管理员自动同步为本地 admin**）。旧版本若在 Casdoor 侧创建过这些角色，可手动删除。

#### Step 2：通过 Ops API 登记租户 client

先在 hub 侧配置 `OPS_API_KEY` 并重启，然后：

```bash
curl -X POST https://<hub>/api/v1/ops/tenant-clients \
  -H "X-Ops-Key: <OPS_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"org":"acme","clientId":"...","clientSecret":"..."}'
```

字段说明：`org` = Casdoor 组织名（即租户 ID）；`cert` 可选，仅当该组织使用**独立于全局 `CASDOOR_CERTIFICATE` 的验签证书**时才需要传（PEM 明文）；`isDefault` 可选。

**org 命名约束**：组织名必须匹配 `^[a-z][a-z0-9]{0,62}$`（小写字母开头，仅小写字母和数字，不超过 63 字符），**不允许连字符、大写、下划线等**，不合法返回 400。原因：组织名用于拼接部署键 `<org>-<agent>` 与 runtime URL 路径段 `/<org>/<agent>`（见下文「部署键与 runtime URL」），而 agent 名本身允许连字符——若 org 也允许连字符，`(org "a", agent "b-c")` 与 `(org "a-b", agent "c")` 会拼出同一个部署键，产生跨租户覆盖/误删的歧义。已登记的存量组织（`zerone` / `ayu` / `zhengxin` 等）天然合规，无需迁移。

default tenant 语义（不变式：**有且唯一**）：

- **首行自动成为 default**：登记的第一条记录自动 `isDefault=true`。
- 登录页组织输入框**留空**（或无 `org` 参数访问）即走 default 行。
- **切换 default**：给新行显式传 `"isDefault":true`（原子切换，旧 default 自动降级）。
- **降级保护**：把当前 default 降级（新增非 default 行时）且表内还有其他行 → 409；**删除 default 行**且表内还有其他行 → 409（响应含迁移指引：先把 default 切给其他组织再删）。删 default 是最后一行时允许；删除不存在的 org 幂等返回 204。

管理查询：`GET /api/v1/ops/tenant-clients`（返回 org / clientId / isDefault / hasCert / 时间，**不返回 secret**）；删除登记：`DELETE /api/v1/ops/tenant-clients/<org>`（default 行且表内还有其他行 → 409；删除不存在的 org 幂等返回 204，语义见上文「降级保护」）。

#### Step 3：分发组织专属登录链接

给该组织成员分发：`https://<hub>/login?org=acme`

- 登录页「更多」折叠区内会出现组织输入框（仅存在多组织配置时渲染），成员也可手动填组织名。
- 组织用户首次登录自动创建 pending 记录（等待 admin 审批分配角色）；Casdoor 组织管理员登录自动成为该租户 admin。
- 登录后的 refresh token 按 token 内的 owner 自解析对应组织凭证，**无需额外配置**。

#### 解析链与边界行为

- **显式 org 未注册 → 404，绝不回落**：`?org=xxx` 指定了不存在/未登记的组织，登录预检就地报错，不会回退到 default。
- **留空 / 无参数 → default 行 → env 全局**：default 行不存在（表空）时回落 env `CASDOOR_CLIENT_ID`（存量单组织部署零改动）。

### 部署键与 runtime URL

agent 的部署标识自本版本起按**租户限定**（tenant-scoped）：

- **runtime URL** 形态为 `https://<gateway>/<org>/<agent>`（如 `https://hub.example.com/acme/general`）。builtin 模式（default 租户）同样限定，URL 为 `/default/<agent>`。
- **Kong 实体名与 deployer 容器键**使用 `<org>-<agent>` 拼接（Kong service/route 实体带 `agent-` 前缀，如 `agent-acme-general`）；容器键同理。
- **数据库仍存裸名**（`agents.name` 等），租户归属由 `tenant_id` 列表达，部署键仅在部署/网关层拼接。
- **hub 代理按限定名寻址**：hub 的聊天代理（chatbox proxy / agent 详情等）经 `/v1/agents/<org>-<name>` 限定名访问 runtime，**裸名 404**；subagent 例外——子 agent 不独立部署，名保持裸名。

**存量 agent 迁移**（升级前以裸名 `/<agent>` 部署的 agent）分三步：

1. **升级后逐个重新部署**：重新部署即落位新部署键 `<org>-<agent>` 与新 URL `/<org>/<agent>`。重新部署前面板显示「未部署」属预期（面板按新部署键查询，尚无对应容器），**外部旧 URL 不受影响**（升级重启后 Reconcile 已挂 `-legacy` 兼容路由），完成重新部署后面板与聊天即恢复。
2. **`-legacy` 兼容路由自动挂载**：重新部署时自动检测升级前的旧裸名 Kong 实体，或上一次部署已挂载的 `-legacy` 路由（兼容窗口从升级重启起算、持续到手动删除该路由为止，**期间含多次重新部署**：每次重新部署前探测到任一存在，清理后都会由新容器重挂兼容路由）；命中则挂载兼容路由 `agent-<org>-<name>-route-legacy`，旧 URL `/<agent>` 继续可用（指向新容器），下游调用方无感。
3. **择机下线旧路由**：确认所有调用方已切换到新 URL 后，经 Kong admin API 删除 `agent-<org>-<name>-route-legacy` 路由（或联系平台方处理）。删除后下次重新部署不再重挂（两种探测均不命中），旧 URL 正式下线。

## OSS (S3 / MinIO)

| Variable | Required | Default | Description |
|---|---|---|---|
| `OSS_ENDPOINT` | No | — | S3 / MinIO Endpoint. **留空 = 整体禁用 OSS**（`InitOSS` 返回 nil，服务正常启动，仅文件上传/下载功能降级） |
| `OSS_REGION` | No | `us-east-1` | S3 Region |
| `OSS_BUCKET` | 若启用 OSS 则必填 | — | Bucket name（`OSS_ENDPOINT` 非空时缺失会启动报错） |
| `OSS_ACCESS_KEY` | 若启用 OSS 则必填 | — | S3 Access Key |
| `OSS_SECRET_KEY` | 若启用 OSS 则必填 | — | S3 Secret Key |
| `OSS_FORCE_PATH_STYLE` | No | `false` | Must be set to `true` for MinIO |
| `OSS_CDN_HOST` | No | — | Domain prefix for skill file downloads. **When set**: the `/skills` list and `/skills/:name/download` endpoints return permanent CDN URLs; **when unset**: the list URL field is empty, and the download endpoint falls back to a 1-hour valid OSS presigned URL |

## Provider Encryption

| Variable | Required | Default | Description |
|---|---|---|---|
| `PROVIDER_ENCRYPTION_KEY` | ⚠️ | — | Encryption key for Provider API Keys (32 bytes hex). **If not set, stored unencrypted** — only for development environments |

Generate an encryption key:

```bash
openssl rand -hex 32
```

## Agent Deployer

| Variable | Required | Default | Description |
|---|---|---|---|
| `AGENT_DEPLOYER_URL` | ✅ | — | agent-deployer service address, e.g. `http://agent-deployer:8080/api/v1` |
| `AGENT_DEPLOYER_API_KEY` | No | — | agent-deployer authentication Bearer token |
| `AGENT_DEPLOYER_PUBLIC_HOST` | No | Parsed from `AGENT_DEPLOYER_URL` | Host for browser access to the runtime |
| `AGENT_RUNTIME_API_KEY` | ⚠️ | — | **Deprecated / kept for compatibility**. The Runtime Token is now generated by agent-hub at deployment time and stored encrypted in `agents.runtime_token`; this variable is currently unused, retained only as a config entry |

## Multirag (Knowledge Base)

| Variable | Required | Default | Description |
|---|---|---|---|
| `MULTIRAG_BASE_URL` | Required for KB module | — | multirag service address, e.g. `http://multirag:8000`; if missing, the service still starts, and knowledge base APIs return 503 |
| `MULTIRAG_API_KEY` | Required for KB module | — | Bearer token for the agent-hub service account to access multirag; not exposed to the browser |
| `MULTIRAG_TIMEOUT_SECONDS` | No | `30` | Timeout for agent-hub calling standard multirag APIs |
| `MULTIRAG_UPLOAD_TIMEOUT_SECONDS` | No | `3600` | Timeout (seconds) for agent-hub streaming document uploads to multirag |

> Large file uploads are streamed from the browser to multirag and are not buffered entirely within agent-hub. The reverse proxy or Kong Route in production must also permit the target file size, disable full-request-body buffering, and set upstream timeouts to no less than `MULTIRAG_UPLOAD_TIMEOUT_SECONDS`.
