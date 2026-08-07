# Security Notes

⚠️ **Security considerations for the current version**:

1. **Never commit `PROVIDER_ENCRYPTION_KEY` to the repository** — once leaked, all API Keys can be decrypted.
2. **`GET /api/v1/providers` returns plaintext API Keys** — any logged-in user can read them. For Electron integration scenarios, assess the threat model and consider desensitization or signed authentication.
3. **CORS defaults to AllowAll + AllowCredentials** — production must explicitly configure allowlist.
4. **ServiceRouter reverse proxy has SSRF risk** — `EndpointURL` lacks scheme / private subnet validation.
5. **OAuth state stored in memory** — process restart loses in-flight logins; recommend migrating to Redis.
6. **Token Exchange logging** — currently prints response bodies containing tokens; should be removed in production.

## Reporting Vulnerabilities

If you discover a security vulnerability, please do **not** open a public issue. Email **zerone-agents@proton.me** directly; we will respond within 72 hours.
