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
| `CASDOOR_ORGANIZATION` | ✅ | — | Casdoor organization name |
| `CASDOOR_CALLBACK_URL` | No | — | OAuth callback URL |

> Roles are managed locally by agent-hub; the Casdoor roles claim in the JWT is ignored entirely. Casdoor only provides user identity (authentication). See the multi-tenant section below.

#### 多租户与角色管理（仅 casdoor 模式）

- **租户 = Casdoor 组织**：token 中的组织（owner）即租户 ID，业务数据按租户隔离。组织的创建与配置在 Casdoor 侧完成，agent-hub 不接管。
- **角色由 agent-hub 本地管理**：角色真实源是本地 `user_identities` 租户成员表（Role + Status pending/active），Casdoor 仅提供用户身份。JWT 中的 Casdoor roles claim 完全忽略。`CASDOOR_ROLE_MAPPING` / `CASDOOR_DEFAULT_ROLE` 环境变量已废弃（检测到仅打 warning，不影响启动）；升级后 Casdoor 侧的 `agent-hub-*` 角色可手动删除。
- **新用户待审批流程**：新用户首次登录成功后自动创建 pending 记录，前端渲染「等待审批」页（可访问 /auth/userinfo 与 logout，其余 API 返回 403 PENDING_APPROVAL）。admin 在用户管理页为其分配角色后自动转为 active。
- **admin 锚定 Casdoor 组织管理员**：本地 admin 资格与 Casdoor 组织管理员（IsAdmin）双向同步——组织管理员登录/CLI 身份核对时自动成为本地 admin；被取消组织管理员则本地 admin 撤销为待审批。admin 任命/降级采用「Casdoor 先行」双写：先成功修改 Casdoor is_admin，再写本地。
- **用户管理（admin）**：列表来自本地成员表（禁用状态实时查 Casdoor）；审批 = 给 pending 用户分配角色（自动转 active）；禁用/重置密码直通 Casdoor；创建用户引导至 Casdoor 注册页。邀请制接口仅 builtin 模式可用。
- **升级指引（breaking）**：升级到此模型后所有现有用户变为待审批；Casdoor 组织管理员登录后自动成为 admin，再逐个为其他用户分配角色。业务数据方面：升级时存量 agents / providers / AIGC 配置等自动回填到 `CASDOOR_ORGANIZATION` 指定的租户；聊天记录按 `user_id → user_identities` 映射回填到各用户所属租户，映射不到的兜底回填启动租户。**casdoor 模式未配置 `CASDOOR_ORGANIZATION` 会拒绝启动**（避免回填到错误租户），请先补齐配置再升级。
- **已知限制**：
  - JWT 无吊销通道：admin 在 Casdoor 侧被降级后，其未过期的 access token 在过期前仍有效；CLI token 最迟 5 分钟内经身份缓存纠正。
  - 用户列表只显示登录过的用户（列表数据源为本地成员表，而非直通 Casdoor）。
- **部署要求**：agent-hub 使用的 Casdoor Application 需要所在组织的用户管理权限（读写用户、组织管理员标志）。CasdoorDirectory 与 CasdoorProvider 复用同一份 `CASDOOR_*` 配置。

##### 数据隔离语义

业务表（agents / cloud_sessions / cloud_messages / provider_summaries / tools / mcp_servers / skills / scenes / aigc_configs）均含 `tenant_id` 列并按租户隔离：

- **tenant_id = Casdoor 组织名**：与登录 token 的组织（owner）一致；builtin 模式恒为 `default`。
- **哨兵约定**：业务表 `tenant_id` 列的默认值为空串 `''`（共享哨兵，对齐全仓 tools/mcp/skills/scenes/providers/aigc 的约定）——写入时由代码显式盖章请求租户，不依赖数据库默认值；空串仅在共享行（内置/种子数据）合法。
- **内置/种子数据全局共享**：内置 tools、MCP servers、skills、scenes 及共享 provider 种子行的 `tenant_id` 为空串（全局共享行）——各租户均可读、不可修改；租户编辑共享 provider 时按 copy-on-write 复制为本租户行后再改动，原共享行不受影响。
- **存量数据回填**：从旧版本升级时，启动迁移会把存量业务数据回填到启动租户（builtin 模式 → `default`；casdoor 模式 → `CASDOOR_ORGANIZATION` 指定的组织）。casdoor 模式未配置 `CASDOOR_ORGANIZATION` 时启动直接失败（fail fast），以避免数据回填到错误租户。
- **同名资源跨租户共存**：原全局唯一约束（如 agents 的 `uk_name`、provider_summaries 的 `uk_key`）已改为 `(tenant_id, name)` / `(tenant_id, key)` 复合唯一索引——不同租户可各自持有同名 agent / 同 key provider。
- **聊天数据两级隔离**：cloud_sessions / cloud_messages 首先按租户隔离，会话列表在租户内再按用户隔离（member 仅见自己的会话，admin/maintainer 可见本租户全部会话）。
- **AIGC 配置 per-tenant + 共享默认回退**：读取时本租户的 aigc_configs 行优先；本租户未配置则回退到共享默认行（ContentProducer 主体编码等全局默认值）。写操作（Save / RotateKey / Delete）只作用于本租户行，且拒绝在租户身份缺失（builtin 之外的空租户上下文）时执行。
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

## OSS (S3 / MinIO)

| Variable | Required | Default | Description |
|---|---|---|---|
| `OSS_ENDPOINT` | ✅ | — | S3 / MinIO Endpoint |
| `OSS_REGION` | No | `us-east-1` | S3 Region |
| `OSS_BUCKET` | ✅ | — | Bucket name |
| `OSS_ACCESS_KEY` | ✅ | — | S3 Access Key |
| `OSS_SECRET_KEY` | ✅ | — | S3 Secret Key |
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
