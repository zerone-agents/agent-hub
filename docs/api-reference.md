# API Reference

All `/api/v1/*` endpoints require `Authorization: Bearer <JWT>`. The `/api/v1/admin/*` endpoints additionally require the `agents-admin` role.

## Authentication

| Method | Path | Description |
|---|---|---|
| GET | `/auth/login` | Redirect to Casdoor login |
| GET | `/auth/callback` | OAuth callback, returns token |
| GET | `/auth/userinfo` | Current user info |
| POST | `/auth/refresh` | Refresh access_token |
| POST | `/auth/logout` | Revoke token |

## Agent (Public)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/agents/manifest` | Agent manifest (includes contentHash for client-side cache invalidation) |
| GET | `/api/v1/agents` | Full configuration of enabled Agents |
| GET | `/api/v1/agents/:name` | Single Agent details |

## Provider (Public — for Electron client consumption)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/providers` | All Providers (including decrypted Locked API Key) |
| GET | `/api/v1/providers/:id` | Single Provider |

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
| POST | `/api/v1/chat/push` | Push sessions/messages (up to 50 sessions per request, conflict detection via `updated_at`) |

## Admin Endpoints (`/api/v1/admin/*`)

Covers CRUD + Probe for Agent / Tool / Skill / Scene / Provider / Chat. See `cmd/server/main.go` for the complete list.

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
