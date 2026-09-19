# API Reference

All `/api/v1/*` endpoints require `Authorization: Bearer <JWT>` (a CLI token works as a bearer token too).

Roles are managed locally by agent-hub (`admin` / `maintainer` / `member`; see configuration.md). The `/api/v1/admin/*` paths are registered as two groups on the same paths:

- **adminWrite** (`RequireManager` = admin + maintainer): all mutating methods (POST/PUT/PATCH/DELETE) and sensitive reads (AIGC config, agent runtime file content `files/content`, provider key reveal).
- **adminRead** (admin + maintainer + member): non-sensitive GETs (lists, details, tools/skills/mcps/knowledge bindings, files listing, deploy status). member is read-only.

User management (`/api/v1/admin/users`, `/api/v1/admin/invites`) is admin-only (`RequireAdmin`).

## Authentication

Two interchangeable backends selected by `AUTH_MODE` (default `builtin`):

| Method | Path | Description |
|---|---|---|
| GET | `/auth/mode` | Report the active auth mode (`builtin` / `casdoor`) |
| GET | `/auth/login?org=<org>` | (casdoor) Redirect to the Casdoor login of the given organization. `org` omitted/blank → default org → env-global `CASDOOR_CLIENT_ID`. Unknown registered-less org → 404, never falls back |
| GET | `/auth/org-check?org=<org>` | (casdoor) Login precheck: validates the org before redirect (unregistered org → 404) |
| GET | `/auth/callback` | (casdoor) OAuth callback, returns token |
| POST | `/auth/login` | (builtin) Username/password login |
| POST | `/auth/setup` | (builtin) First-run setup, creates the initial `admin` account |
| POST | `/auth/register` | (builtin) Register via one-time invite token |
| GET | `/auth/invite/:token` | (builtin) Invite precheck |
| POST | `/auth/change-password` | (builtin) Change own password |
| GET | `/auth/userinfo` | Current user info. `tenant_id` is the authoritative field (casdoor mode = Casdoor org name; builtin mode = `default`); `org_id` is a backward-compatible same-source value |
| POST | `/auth/refresh` | Refresh access_token (rotation: old refresh token revoked) |
| POST | `/auth/logout` | Revoke token |

## Agent (Public)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/agents/manifest` | Agent manifest (includes contentHash for client-side cache invalidation) |
| GET | `/api/v1/agents` | Full configuration of enabled Agents |
| GET | `/api/v1/agents/:name` | Single Agent details |
| GET | `/api/v1/agents/:name/chat/sessions` | List chat sessions of an agent |
| POST | `/api/v1/agents/:name/chat/sessions` | Create chat session |
| GET | `/api/v1/agents/:name/chat/sessions/:id/messages` | List session messages |
| POST | `/api/v1/agents/:name/chat/sessions/:id/messages` | Send message (SSE stream) |
| DELETE | `/api/v1/agents/:name/chat/sessions/:id` | Delete session |

## Provider (Public — for Electron client consumption)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/providers` | Providers of the tenant + shared seed rows; locked API key returned **masked**, never plaintext |
| GET | `/api/v1/providers/:id` | Single Provider (same masking) |
| GET | `/api/v1/providers/runtime-config` | Runtime-facing provider config (for agent runtime consumption) |

Plaintext key reveal is admin/maintainer only: `POST /api/v1/admin/providers/:id/reveal-key`.

## Skill (Public)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/skills` | Skill list (supports `?type=expert\|community`) |
| GET | `/api/v1/skills/:name` | Skill details |
| GET | `/api/v1/skills/:name/download` | Presigned download link (valid for 1 hour) |

## Scene (Public)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/scenes` | Scene list |
| GET | `/api/v1/scenes/:name` | Scene details |

## Chat (Regular Users)

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/chat/push` | Push sessions/messages (up to 50 sessions per request, conflict detection via `updated_at`). Auth: JWT/CLI token (default) **or** `X-Chat-Push-Key: <CHAT_PUSH_API_KEY>` — in key mode, per-session `user_name` (required) becomes `user_id`/`display_name`; tenant from `org`: builtin mode ignores `org` (always `"default"`); casdoor mode uses explicit `org`, or when omitted resolves to the registered default tenant org (400 if none registered) |

## Admin Endpoints (`/api/v1/admin/*`)

Covers CRUD + Probe for Agent / Tool / Skill / Scene / Provider / Knowledge / Chat. See `cmd/server/main.go` for the complete list. Notable members:

| Method | Path | Group | Description |
|---|---|---|---|
| GET | `/api/v1/admin/agents/:name/deploy` | adminRead | Deployment status (runtimeUrl/apiKey included; member-readable) |
| POST | `/api/v1/admin/agents/:name/deploy` (+ `/stop`, `/start`, DELETE) | adminWrite | Deploy / lifecycle control |
| GET | `/api/v1/admin/agents/:name/files`, `/files/content` | read / write | Files listing is member-readable; **content is admin/maintainer only** |
| GET×3 + DELETE | `/api/v1/admin/chat/sessions...` | adminRead | Chat history. Handler-level `chatScopeUserID` isolation: member sees/deletes only their own sessions (others → 404); admin/maintainer see all sessions of the tenant |
| GET/PUT/DELETE | `/api/v1/admin/aigc/config` (+ `POST /config/rotate-key`) | adminWrite | Per-tenant AIGC content-labeling config (GB 45438-2025) |
| GET/POST/DELETE | `/api/v1/admin/users`, `/admin/invites` | admin only | User management / invite links |
| GET | `/api/v1/admin/audit-logs` | admin only | Audit log query — filters + snapshot pagination, see [Audit Logs](#audit-logs-admin-only) |

### Audit Logs (admin only)

`GET /api/v1/admin/audit-logs` — audit trail query, admin-only (`RequireAdmin`; maintainer/member → 403). Audit logs are read-only: there is no update/delete endpoint.

Query parameters:

| Param | Default | Description |
|---|---|---|
| `page` | `1` | Page number, ≥ 1; invalid → 400 「无效的分页参数」 |
| `page_size` | `20` | ≥ 1, capped at 100 (values > 100 are clamped, not rejected); invalid → 400 「无效的分页参数」 |
| `category` | — | Exact match: `auth` \| `user` \| `invite` \| `provider` \| `agent` \| `token` \| `aigc` |
| `action` | — | Exact match, e.g. `user.update_role` |
| `user` | — | Fuzzy match on `userName` / `userId` (LIKE `%…%`) |
| `from`, `to` | — | RFC3339 timestamps; closed interval on `createdAt` (`from` ≤ t ≤ `to`); invalid → 400 「无效的时间范围」 |
| `snapshotId` | — | Decimal string, optional. Snapshot-consistent pagination: omit on the first request and the server captures the tenant-wide `MAX(id)`; echo the returned `snapshotId` on subsequent requests so items and `total` share the same `id <= snapshotId` view. `"0"` is the empty-set sentinel (tenant has no logs). Invalid → 400 「无效的快照参数」 |

Response (ordered `createdAt DESC, id DESC`):

```json
{"items":[{"id":"42","tenantId":"default","userId":"7","userName":"alice","category":"user","action":"user.update_role","targetType":"user","targetId":"2","targetName":"bob","status":"success","detail":{"field":"role","from":"member","to":"maintainer"},"remoteIp":"10.0.0.1","userAgent":"…","createdAt":"2026-09-10T12:00:00.123456Z"}],"total":1,"snapshotId":"42"}
```

- `id` / `snapshotId` are decimal strings: uint64 values above 2^53-1 lose precision as JS numbers — keep them as strings, never convert to number.
- `detail` is a JSON object or `null`, shaped by a strongly-typed allowlist per `action`.
- `status` is three-state: `success` / `failure` / `partial`.

## CLI Tokens (`/api/v1/cli/*` — admin/maintainer only)

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/cli/issue-token` | Issue a long-lived CLI token (`cli_<hex>`; only SHA-256 hash stored) |
| GET | `/api/v1/cli/tokens` | List own tokens |
| DELETE | `/api/v1/cli/tokens/:id` | Revoke a token |

A CLI token grants the same permissions as the issuing user.

## Ops API (`/api/v1/ops/*` — X-Ops-Key header)

Mounted only when `OPS_API_KEY` is set. Used to register per-org Casdoor OAuth clients (multi-org login); see configuration.md for the runbook.

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/ops/tenant-clients` | Upsert an org → Casdoor Application mapping |
| GET | `/api/v1/ops/tenant-clients` | List registered orgs (no secrets returned) |
| DELETE | `/api/v1/ops/tenant-clients/:org` | Delete a mapping (409 if it is the default and others remain; idempotent 204) |

## Agent ID vs Deployment Key vs Public URL

Deployed agents carry three independent identities since deployer v3.1 (see configuration.md):

- **Agent ID** (bare `<name>`): the runtime agent graph identity; the hub chat/detail proxies address runtimes at `/v1/agents/<name>` (subagents keep bare names too).
- **Deployment key** (`<org>-<name>`): Kong entity / deployer container key — all lifecycle addressing (get/start/stop/delete) and gateway entity naming.
- **Public URL**: casdoor mode `https://<gateway>/<org>/<name>`; builtin mode `https://<gateway>/<name>` (no `/default` prefix; the deployment key still keeps the `default-` prefix). Without Kong, the hub proxy serves the same paths under `/runtime`.

### Provider Probe Example

```bash
# Test connectivity of a saved Provider
curl -X POST http://localhost:8081/api/v1/admin/providers/1/probe \
  -H "Authorization: Bearer $TOKEN"

# Test an unsaved configuration
curl -X POST http://localhost:8081/api/v1/admin/providers/probe \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "baseUrl": "https://api.anthropic.com",
    "apiKey": "sk-ant-...",
    "protocol": "anthropic",
    "authStyle": "api_key"
  }'
```

## Core Capability Endpoints Quick Reference

| Domain | Key Endpoint |
|---|---|
| **Agent** | `GET /api/v1/agents/manifest` |
| **Tool** | `POST /api/v1/admin/tools`（multipart 上传自定义工具）；`PUT /api/v1/admin/tools/:name/file`（补传/替换）；`GET /api/v1/admin/tools/:name/download` |
| **Skill** | `GET /api/v1/skills/:name/download` |
| **Provider** | `GET /api/v1/providers` |
| **Scene** | `GET /api/v1/scenes` |
| **Chat** | `POST /api/v1/chat/push` |

## Group collaboration (H4)

Groups are tenant-scoped, durable collaboration units. Channels belong to one
group; sessions are lightweight, agenda-bound channel conversations rather
than approval or voting workflows.

| Method | Path | Description |
|---|---|---|
| GET/POST | `/api/v1/admin/groups` | List/create groups |
| GET/PUT/DELETE | `/api/v1/admin/groups/:id` | Read/update/delete a group |
| GET/POST | `/api/v1/admin/groups/:id/members` | List/add Agent members |
| PATCH/DELETE | `/api/v1/admin/groups/:id/members/:agentId` | Change role/remove member |
| GET | `/api/v1/admin/groups/:id/audit` | Read the append-only collaboration audit for the group |
| GET/POST | `/api/v1/admin/groups/:id/channels` | List/create channels |
| GET/PUT/DELETE | `/api/v1/admin/channels/:id` | Read/update/delete a channel |
| GET/PUT | `/api/v1/admin/channels/:id/subscriptions` | List/upsert Agent subscription |
| GET/POST | `/api/v1/admin/channels/:id/sessions` | List/create lightweight sessions; create accepts optional `participantAgentIds` |
| GET | `/api/v1/admin/sessions/:id` | Read session |
| POST | `/api/v1/admin/sessions/:id/start` | Start a draft session |
| POST | `/api/v1/admin/sessions/:id/complete` | Complete with `{ "summary": "..." }` |

Values: group visibility `private|tenant`; member role
`leader|member|observer|guest`; channel visibility `group|members`;
subscription mode `all|mentions|none`. Every lookup and mutation is constrained
by the authenticated tenant. Session transitions are only
`draft -> active -> completed`.

Session creation snapshots participants into durable rows. Send
`participantAgentIds` to choose them explicitly; when omitted, the current
effective channel recipients are snapshotted. An optional host is always
included and marked with role `host`; other attendees use `participant`.

The group audit covers group create/update/delete, member add/role change/remove,
channel create/update/delete, subscription upsert and session create/start/
complete. Each event exposes `resourceType`, `resourceId`, `action`, `before`,
`after` and `createdAt`, allowing the UI to reconstruct what actually changed.

Organization MCP `group_send` and `channel_publish` always deliver
asynchronously and persist a separate status/reply/error record per recipient.
`audience=round_robin` selects exactly one eligible subscriber/member per call;
its cursor is persisted per tenant and group/channel (plus optional role), so a
Hub restart does not reset the rotation. `aggregation=all_replies` collects all
completed replies, while `first_success` reaches aggregate success on the first
successful reply without cancelling the remaining deliveries.

`aggregation=leader_summary` means **use the reply from a leader among this
dispatch's recipients as the aggregate result**. It does not secretly invoke a
second Agent summarization pass (which could recurse or deadlock). If this
dispatch has no recipient leader, the aggregate result stays empty while every
recipient delivery remains available in the audit trail.

### Custom Tools (issue #88)

- 单文件 `.ts/.mts/.js/.mjs`，≤5 MiB；工具名来自文件默认导出的 `name`（Hub 不执行代码，Runtime 部署时校验）。
- `tools.source`：`builtin`（共享只读预设）/ `custom`（租户制品）；custom 制品状态派生 `ready|missing`。
- 删除仍被 Agent 挂载的自定义工具返回 `409` + `data.agents` 名单；内置工具拒绝一切写操作。
- 部署请求向 agent-deployer 下发 `customTools []ToolSource{name,url,hash,fileName}`（仅 custom+ready，按名排序；URL = OSS_CDN_HOST + 内容寻址 key）。
- PUT /api/v1/admin/tools/:name 仅接受 title/description/descriptionEn；其他字段（如 isDefault）会被静默忽略。
# H5 工作流与审批

工作流是租户隔离、版本冻结的通用有向图，不包含任何垂直业务步骤。`dependsOn` 表达串行、并行与汇合；`transitions[].condition.equals` 提供确定性的条件分支。步骤类型为 `task`、`handoff` 或 `approval`。

- `POST/GET /api/v1/admin/workflows`：新建、列出工作流模板。
- `GET /api/v1/admin/workflows/:id`：模板、版本、步骤与转移。
- `POST /api/v1/admin/workflows/:id/versions`：创建不可变草稿版本。
- `POST /api/v1/admin/workflow-versions/:id/publish`：发布版本。
- `POST /api/v1/admin/workflow-versions/:id/executions`：以 `runId`、`input` 和 `idempotencyKey` 启动执行。
- `GET /api/v1/admin/workflow-executions?workflowId=&status=`：执行列表。
- `GET /api/v1/admin/workflow-executions/:id`：步骤、审批与决定快照。
- `GET /api/v1/admin/workflow-executions/:id/audit`：完整执行审计。
- `POST /api/v1/admin/workflow-step-runs/:id/complete|fail`：完成或失败一个任务步骤。
- `POST /api/v1/admin/workflow-approvals/:id/decisions`：`approve`、`reject` 或 `conditional_approve`。
- `POST /api/v1/admin/workflow-executions/:id/process-timeouts`：处理到期步骤并激活升级路径。

Agent 运行时可通过 organization MCP 的 `workflow_start`、`workflow_step_complete`、`workflow_step_fail` 和 `approval_vote` 使用相同能力。步骤启动时会把 `agent`、群组 `role` 或整个 `group` 解析为不可变的受派人快照，异步派发到每个 Agent；普通步骤只有快照中的 Agent 可以回执。审批人在审批创建时同样冻结为快照，后续成员与角色变化不会改写历史。所有幂等键都绑定请求指纹，同一个键换目标或载荷会明确冲突。
