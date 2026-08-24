# Security Notes

⚠️ **Security considerations for the current version**:

1. **Never commit `PROVIDER_ENCRYPTION_KEY` to the repository** — once leaked, all API Keys can be decrypted.
2. **Provider API keys are masked on read** — `GET /api/v1/providers` / `/:id` and the admin list return masked keys only; plaintext is retrievable solely via `POST /api/v1/admin/providers/:id/reveal-key` (adminWrite group = admin/maintainer, tenant-scoped). Keep the reveal endpoint audited; Electron clients should not need plaintext keys.
3. **`OPS_API_KEY` is a high-privilege credential** — it can register arbitrary Casdoor OAuth clients for any org (`/api/v1/ops/tenant-clients`), i.e. mint new login entry points. If leaked, an attacker can direct hub logins to a Casdoor application they control. Keep it unconfigured unless onboarding new orgs, store it in a secret manager, and rotate on suspicion. Requests require the `X-Ops-Key` header; when unset, the ops endpoints are not mounted at all.
4. **`CHAT_PUSH_API_KEY` is a narrow-scope credential** — it only authorizes writing chat sessions/messages via `POST /api/v1/chat/push` (header `X-Chat-Push-Key`). It cannot read any data, cannot reach `/api/v1/ops/*`, and grants no user identity. Keep it independent from `OPS_API_KEY` so a leak of either does not affect the other; rotate on suspicion.
5. **`tenant_oauth_clients.client_secret_enc` / `cert_enc` are AES-GCM encrypted at rest** (`PROVIDER_ENCRYPTION_KEY`); ops API responses never return secrets.
6. **CLI tokens carry the issuing user's full permissions** (`/api/v1/cli/*`, admin/maintainer only) — a leaked CLI token is equivalent to the user's session until expiry/revocation. Tokens are stored as SHA-256 hashes; revoke immediately on suspicion.
7. **CORS defaults to AllowAll + AllowCredentials** — production must explicitly configure allowlist.
8. **ServiceRouter reverse proxy has SSRF risk** — `EndpointURL` lacks scheme / private subnet validation.
9. **OAuth state stored in memory** — process restart loses in-flight logins; recommend migrating to Redis.
10. **Token Exchange logging** — currently prints response bodies containing tokens; should be removed in production.

## Reporting Vulnerabilities

If you discover a security vulnerability, please do **not** open a public issue. Email **zerone-agents@proton.me** directly; we will respond within 72 hours.
