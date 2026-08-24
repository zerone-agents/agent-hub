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

## Tenant-Scoped Deployment Keys

Since multi-tenancy, deployed agents are addressed by tenant-scoped keys (see configuration.md):

- Runtime URL: `https://<gateway>/<org>/<name>` (builtin mode: `/default/<name>`).
- Deployment key (Kong entity / deployer container key): `<org>-<name>`.
- The hub chat proxy addresses runtimes by qualified name `/v1/agents/<org>-<name>` — **bare names return 404**, except subagents, which keep bare names.

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
| **Tool** | `GET /api/v1/admin/tools` |
| **Skill** | `GET /api/v1/skills/:name/download` |
| **Provider** | `GET /api/v1/providers` |
| **Scene** | `GET /api/v1/scenes` |
| **Chat** | `POST /api/v1/chat/push` |
