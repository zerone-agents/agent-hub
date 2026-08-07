# Quick Start

This directory contains the fastest way to run Zerone Agent Hub locally.

## Prerequisites

- Docker & Docker Compose
- Git

## Run

```bash
# 1. Clone the repository
git clone https://github.com/zerone-agents/agent-hub.git
cd agent-hub/quickstart

# 2. Copy environment template
cp .env.example .env
# Edit .env with your settings

# 3. Start services
docker compose up -d

# 4. Open in browser
open http://localhost:8080
```

## Configuration

See [.env.example](.env.example) for all available configuration options.

## Production Deployment

For production deployment, see [docs/deployment.md](../docs/deployment.md).
