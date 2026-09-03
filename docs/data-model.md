# Data Model

Source of truth: `internal/domain/*/` (`model.go` / per-entity files). All GORM entities are AutoMigrated on startup by `pkg/database`.

## Multi-Tenant Conventions

- Business tables carry a `tenant_id` column (`varchar(64)`, default `''`). Tenant ID = Casdoor org name (casdoor mode) or `default` (builtin mode).
- Empty string `''` is the **shared sentinel**: rows with `tenant_id=''` are global seed/builtin rows (built-in tools, MCP servers, shared provider presets) — readable by every tenant, writable by none (copy-on-write for providers).
- Name uniqueness is tenant-scoped via composite unique indexes: `uk_agents_tenant_name` (agents), `uk_tenant_key` (provider_summaries), `uk_skills_tenant_name` (skills), `uk_scenes_tenant_name` (scenes), `uk_mcp_tenant_name` (mcp_servers), `uk_tools_tenant_name` (tools), `uk_tenant_id` (aigc_configs).
- `tools.source`（builtin|custom）与制品字段（file_name/file_url/file_hash/file_size）：custom 四字段完整 = ready，否则 missing（迁移存量）；custom 强制 is_default=false，builtin 恒 ready。

## Entity Overview

```
agent.AgentConfig (agents) ─┬─ (N) agent.AgentSubagent → AgentConfig      (agent_subagents)
                            ├─ (N) agent.AgentTool     → agent.Tool        (agent_tools)
                            └─ (N) agent.AgentSkill    → skill.Skill       (agent_skills)

scene.Scene (scenes) (N) ─→ (1) agent.AgentConfig   (scene.agent_id)

mcp.McpServer (mcp_servers) (N) ←─ (N) agent.AgentMcpServer (agent_mcp_servers)

provider.ProviderSummary (provider_summaries) ─┬─ (N) provider.ProviderAttribute (provider_attributes, EAV)
                                                └─ (N) provider.ProviderModel      (provider_models)

chat.Session (cloud_sessions) (1) ─→ (N) chat.Message (cloud_messages)

auth.UserIdentity (user_identities)   # casdoor 模式：租户成员表（角色真实源）
auth.TenantOAuthClient (tenant_oauth_clients)  # 多组织登录：org → Casdoor Application 凭证
auth.CLIToken (cli_tokens) / auth.Invite (invites) / auth.RefreshToken (refresh_tokens) / auth.User (users)

aigc.Config (aigc_configs)            # 纯 per-tenant：每运营主体一行
```

`internal/domain/tenant/` contains only `context.go` (gin tenant context helpers, `DefaultID = "default"`) — there is no tenant.Tenant/User/ServiceDeployment/Resource entity. `internal/domain/knowledge/` defines gateway DTO types only (no local tables; datasets/documents live in multirag).

## Key Entities

### agent.AgentConfig (`agents`)

Single source of agent configuration. Notable columns: `name` (+ `tenant_id` composite unique), `content_hash`, `system_prompt`, i18n `title` / `description` (JSON maps), provider binding (`provider_id` / `model_id` / `model_selection_id`), `field_overrides` (JSON, secret fields AES-GCM encrypted), platform visibility (`desktop_enabled` / `mobile_enabled`), `max_session_queries`, deployment state (`runtime_port`, `deployment_status`, `deployed_at`, `runtime_token` — AES-GCM encrypted, write-only).

Association tables (`agent_subagents`, `agent_tools`, `agent_skills`) use composite PKs with `OnDelete:CASCADE`.

### auth.UserIdentity (`user_identities`)

Per-tenant membership record for casdoor mode — the source of truth for roles. `provider` + `external_id` (casdoor user Id) unique; `tenant_id` = org name; `role` (admin/maintainer/member, empty = pending assignment); `status` (pending/active).

### auth.TenantOAuthClient (`tenant_oauth_clients`)

Multi-org login mapping: PK `org` → Casdoor Application credentials. `client_secret_enc` / `cert_enc` are AES-GCM ciphertexts; empty `cert_enc` = verify with the global `CASDOOR_CERTIFICATE`. `default_key` implements the "exactly one default org" invariant via a nullable unique index (`uk_default_key`): the default row stores the org name, all others NULL.

### provider.ProviderSummary (`provider_summaries`) + EAV children

Replaces the dropped `vendor_presets` table. The summary row holds descriptive info (`key` + `tenant_id` composite unique, protocol, authStyle, base_url, form `fields` JSON, `locked_api_key` AES-GCM encrypted); extensible per-provider config lives in `provider_attributes` (EAV: unique `(provider_id, attr_key)`, value typed by `attr_type` string/bool/int); models are normalized rows in `provider_models` (unique `(provider_id, selection_id)`, `OnDelete:CASCADE`, includes `model_type`, `context_window`, `aigc_code`).

### chat.Session / chat.Message (`cloud_sessions` / `cloud_messages`)

Both carry `tenant_id` (two-level isolation: tenant → user). Composite PKs (`user_id + id`; message adds `session_id`). Sessions hold model/agent binding (`model`, `model_selection_id`, `provider_id`, `agent_id`, `runtime_session_id`, `source`); messages hold role/content, `token_usage`, `feedback`, and the AIGC label (`aigc`).

### aigc.Config (`aigc_configs`)

Per-tenant AIGC content-labeling config (GB 45438-2025), one row per tenant (`uk_tenant_id`, no shared fallback): USCC, company name, 27-char ContentProducer code, and `signing_key_encrypted` (never exposed via API).

### Auth-adjacent tables

- `users` — builtin local accounts (bcrypt password hash, role/status).
- `cli_tokens` — long-lived CLI tokens (`cli_<hex>`; only SHA-256 `token_hash` stored, unique).
- `invites` — one-time registration links (`inv_<hex>` token hash, role, expiry, `used_at`).
- `refresh_tokens` — opaque session tokens (`rt_<hex>`; only SHA-256 hash stored; revocation = row delete, rotation = delete + insert).

## Secrets at Rest

| Column | Protection |
|---|---|
| `agents.runtime_token`, `agents.field_overrides` (secret fields) | AES-GCM (`PROVIDER_ENCRYPTION_KEY`) |
| `provider_summaries.locked_api_key` | AES-GCM; API responses return masked values, plaintext only via admin reveal-key |
| `tenant_oauth_clients.client_secret_enc` / `cert_enc` | AES-GCM |
| `aigc_configs.signing_key_encrypted` | AES-GCM; never returned by any API |
| `cli_tokens.token_hash`, `invites.token_hash`, `refresh_tokens.token_hash` | SHA-256 (plaintext never stored) |
| `users.password_hash` | bcrypt |

AutoMigrate runs automatically on application startup (in production, consider extracting it into a separate Job to avoid multi-replica race conditions).
