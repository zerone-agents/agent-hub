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

> Casdoor role names containing `admin` map to the `admin` role; those containing `maintainer` map to `maintainer`; everything else maps to `member`. User management (invite/roles/disable) is only available in builtin mode — in casdoor mode it stays in the Casdoor console.

#### 多租户与角色映射（仅 casdoor 模式）

- **租户 = Casdoor 组织**：token 中的组织（owner）即租户 ID，业务数据按租户隔离（Phase 3 落地）。组织的创建与配置在 Casdoor 侧完成，agent-hub 不接管。
- **角色映射**：一律严格匹配。不配 `CASDOOR_ROLE_MAPPING` 时使用内置默认映射（`admin=agent-hub-admin,maintainer=agent-hub-maintainer,member=agent-hub-member`）；需要自定义 Casdoor 角色名时显式配置：

  ```bash
  # 逗号分隔的 k=v 对：agent-hub 角色 = Casdoor 角色名
  CASDOOR_ROLE_MAPPING="admin=agent-hub-admin,maintainer=agent-hub-maintainer,member=agent-hub-member"
  CASDOOR_DEFAULT_ROLE=""   # 无匹配角色时：留空=拒绝登录；member=兜底为 member
  ```

  用户持有多个映射角色时取最高：admin > maintainer > member。启动期校验：mapping 的 key 必须是 admin/maintainer/member、值不允许重复、格式必须是 `k=v,k=v`，违反任一即拒绝启动（fail-fast）。
- **升级注意**：从 v1.1.x 升级前，必须在 Casdoor 组织内创建映射对应的角色（默认 `agent-hub-admin`/`agent-hub-maintainer`/`agent-hub-member`）并分配给用户，否则用户登录时无匹配角色会被拒绝（或落入 `CASDOOR_DEFAULT_ROLE`）。历史版本的角色名子串匹配行为已移除。
- **影子记录**：casdoor 登录成功会在 `user_identities` 表写入/刷新一条影子记录（内部 ID 映射 + 租户 + 角色快照 + last_login），不影响登录链路（失败仅记日志）。
- **用户管理（admin）**：casdoor 模式下「用户管理」页直连 Casdoor API——列表/修改角色/禁用/重置密码均在 Casdoor 侧生效；创建用户引导至 Casdoor 组织注册页（页面右上角「去 Casdoor 注册」）。邀请制接口仅 builtin 模式可用。角色修改只替换映射角色，用户在 Casdoor 侧的其他角色保持不变。
- **已知限制**：casdoor 用户被禁用后，其未过期的 access token 在过期前仍可使用（Casdoor JWT 本地校验，无吊销通道）；CLI token 路径实时查询用户状态，禁用最迟 5 分钟内生效（身份缓存 TTL）。
- **部署要求**：agent-hub 使用的 Casdoor Application 需要有所在组织的用户管理权限（读写用户、角色）。CasdoorDirectory 与 CasdoorProvider 复用同一份 `CASDOOR_*` 配置。

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
