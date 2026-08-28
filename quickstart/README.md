# Quick Start

Fastest way to run Zerone Agent Hub locally with the **built-in user system** (username/password, admin-invite — no external identity provider).

The stack: **MySQL 8** + **agent-hub server** (one image that compiles the frontend and embeds it; served at `/static/` alongside the API) + **agent-deployer** (manages agent-runtime containers on the host Docker daemon, so you can deploy and chat with agents end to end).

## Prerequisites

- Docker & Docker Compose
- Git

## Run

The default stack pulls the prebuilt images from Docker Hub — no source build needed.

```bash
# 1. Clone the repository
git clone https://github.com/zerone-agents/agent-hub.git
cd agent-hub/quickstart

# 2. Copy environment template
cp .env.example .env

# 3. (Optional) Set AUTH_JWT_SECRET — when empty, the server generates an
#    ephemeral random secret at startup (all sessions invalidate on restart).
#    To set a stable one:
#      sed -i.bak "s/AUTH_JWT_SECRET=.*/AUTH_JWT_SECRET=$(openssl rand -hex 32)/" .env

# 4. Set DEPLOYER_DATA_DIR in .env (required) — an ABSOLUTE path on the Docker
#    host holding agent configs/sessions/skills. The same path is bind-mounted
#    into the deployer and every runtime container it creates, so it must be
#    identical on both sides. Examples:
#      Linux:  DEPLOYER_DATA_DIR=/var/lib/agent-deployer
#      macOS:  DEPLOYER_DATA_DIR=/Users/<you>/agent-hub-data/deployer

# 5. (Recommended) Set AGENT_DEPLOYER_API_KEY in .env — generate one with
#    `openssl rand -hex 32`. Empty disables deployer auth (local dev only).

# 6. Start (pulls zeroneai/agent-hub + zeroneai/agent-deployer)
docker compose up -d

# 7. Open the app
open http://localhost:8081/static/
```

> **China networks:** if Docker Hub pulls fail, switch to the Huawei Cloud SWR
> mirrors in `.env` (`DEPLOYER_IMAGE` / `RUNTIME_IMAGE`, see `.env.example`),
> and configure a registry mirror for the daemon.

> **Remote server deployments:** set `AGENT_DEPLOYER_PUBLIC_HOST` to the
> server's public IP/domain. Browsers and the hub's health probes reach runtime
> containers at `<public-host>:<dynamic-port>`, so the firewall / cloud
> security group must allow the Docker ephemeral port range (32768-60999).

### Build from source

To run from a local source build instead of the published image (e.g. for testing unreleased changes), add the build override file:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

This compiles the frontend and backend from the repo root `Dockerfile` and tags it as the `hub` image.

## First-run setup

Because `AUTH_MODE=builtin` and the database starts empty, the first visit redirects to the **setup screen**:

1. You'll be prompted to create the initial admin account. The username is fixed as **`admin`**; you choose the password (≥8 chars, must contain both letters and digits).
2. After creating it, you're logged in automatically.
3. Further users are added via the admin UI: **avatar menu → 用户管理 → 创建邀请**. Copy the one-time invite link (shown only once) and send it to the invitee; they register at that link.

Login afterwards: http://localhost:8081/static/login — username `admin` + the password you set.

## Roles

| Role | Invite users | Manage resources | View/use |
|---|---|---|---|
| `admin` | ✅ | ✅ | ✅ |
| `maintainer` | ❌ | ✅ | ✅ |
| `member` | ❌ | ❌ | ✅ |

## Configuration

See [.env.example](.env.example) for all options. Optional integrations (model providers, Kong, knowledge base) are disabled by default — uncomment and fill in to enable. The agent-deployer integration is wired up in-stack by default (`AGENT_DEPLOYER_URL=http://deployer:8080`); set `AGENT_DEPLOYER_API_KEY` so the hub and deployer authenticate with each other.

> **Switching to Casdoor SSO:** this quickstart uses the built-in auth. For the hosted SaaS / enterprise deployments that delegate to Casdoor, set `AUTH_MODE=casdoor` and provide the `CASDOOR_*` variables instead (see `docs/configuration.md`).

## Operations

```bash
docker compose logs -f hub        # follow server logs
docker compose logs -f deployer   # follow deployer logs
docker compose down               # stop (keeps the MySQL volume + deployer data)
docker compose down -v            # stop and wipe the database (deployer data stays on the host path)
```

## Production Deployment

For production deployment, see [docs/deployment.md](../docs/deployment.md).
