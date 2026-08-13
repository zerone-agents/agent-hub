# Quick Start

Fastest way to run Zerone Agent Hub locally with the **built-in user system** (username/password, admin-invite — no external identity provider).

The stack: **MySQL 8** + **agent-hub server** (one image that compiles the frontend and embeds it; served at `/static/` alongside the API).

## Prerequisites

- Docker & Docker Compose
- Git

## Run

The default stack pulls the prebuilt image from Docker Hub — no source build needed.

```bash
# 1. Clone the repository
git clone https://github.com/zerone-agents/agent-hub.git
cd agent-hub/quickstart

# 2. Copy environment template
cp .env.example .env

# 3. Set the JWT secret (required) — generate a random one:
#    On macOS/Linux:
#      sed -i.bak "s/AUTH_JWT_SECRET=.*/AUTH_JWT_SECRET=$(openssl rand -hex 32)/" .env
#    or edit .env manually.
#    The value must be at least 32 bytes.

# 4. Start (pulls zeroneai/agent-hub:latest)
docker compose up -d

# 5. Open the app
open http://localhost:8081/static/
```

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

See [.env.example](.env.example) for all options. Optional integrations (model providers, deployer, Kong, knowledge base) are disabled by default — uncomment and fill in to enable.

> **Switching to Casdoor SSO:** this quickstart uses the built-in auth. For the hosted SaaS / enterprise deployments that delegate to Casdoor, set `AUTH_MODE=casdoor` and provide the `CASDOOR_*` variables instead (see `docs/configuration.md`).

## Operations

```bash
docker compose logs -f hub        # follow server logs
docker compose down               # stop (keeps the MySQL volume)
docker compose down -v            # stop and wipe the database
```

## Production Deployment

For production deployment, see [docs/deployment.md](../docs/deployment.md).
