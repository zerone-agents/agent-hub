# Local Development

Local development guide: decoupled frontend/backend startup, testing, build artifacts, and team conventions.

## Prerequisites

- Go ≥ 1.24
- Node.js ≥ 22
- Docker + Docker Compose
- Local Go: `/opt/homebrew/bin/go` (macOS Homebrew path)

## Option 1: Docker Compose (Recommended)

```bash
git clone https://github.com/zerone-agents/agent-hub.git
cd agent-hub

# Start all dependencies + app
docker compose up -d

# App listens on http://localhost:8081
# Health check: curl http://localhost:8081/health
```

On first startup, it will automatically:
1. Run AutoMigrate to create all tables
2. Seed 5 built-in Providers (Anthropic / OpenAI / GLM / Kimi / Bailian)

## Option 2: Local Development (Frontend/Backend Decoupled)

**Start dependencies**:

```bash
docker compose up -d mysql minio createbuckets casdoor
```

**Start backend**:

```bash
export DATABASE_URL="mg:mgpassword@tcp(localhost:3306)/middle_ground?charset=utf8mb4&parseTime=True&loc=Local"
export CASDOOR_ENDPOINT="http://localhost:8000"
export CASDOOR_CLIENT_ID="<your-client-id>"
export CASDOOR_CLIENT_SECRET="<your-client-secret>"
export CASDOOR_CERTIFICATE="<your-cert>"
export CASDOOR_ORGANIZATION="<your-org>"
export OSS_ENDPOINT="http://localhost:9000"
export OSS_BUCKET="middle-ground"
export OSS_ACCESS_KEY="minioadmin"
export OSS_SECRET_KEY="minioadmin"
export OSS_FORCE_PATH_STYLE="true"
export OSS_CDN_HOST="http://localhost:9000/middle-ground"
export PROVIDER_ENCRYPTION_KEY="<64-hex-chars>"  # openssl rand -hex 32
export MULTIRAG_BASE_URL="http://localhost:8000"
export MULTIRAG_API_KEY="<multirag-service-api-key>"
export MULTIRAG_TIMEOUT_SECONDS="30"
export MULTIRAG_UPLOAD_TIMEOUT_SECONDS="3600"

go run ./cmd/server
```

**Start frontend**:

```bash
cd frontend
npm install
npm run dev    # Default port 3000, auto-proxies /api and /auth to 192.168.151.49:8081
```

> 💡 Bypass login during development: set `VITE_BYPASS_AUTH=true` in `frontend/.env.local` (already ignored by `.gitignore`).

## Build Artifacts

```bash
# Build frontend (output in frontend/dist)
cd frontend && npm run build

# Build backend (auto-embeds frontend dist)
go build -o bin/server ./cmd/server
```

---

## Testing

**Frontend** (Vitest + Testing Library + MSW):

```bash
cd frontend
npm test          # watch mode
npm run test:run  # single run
```

**Backend**:

```bash
go test -v ./...
```

> ⚠️ Backend test coverage is currently thin; prioritize adding tests for Provider encryption/decryption, Chat Push conflict detection, and Skill CRUD.

---

## Team Conventions

- **Git**: Conventional Commits (`feat:` / `fix:` / `refactor:` / `chore:` / `style:`)
- **Error messages**: User-facing errors in Chinese, internal errors with English stack traces
- **Field naming**: DB fields snake_case, JSON fields camelCase, Go fields PascalCase
- **i18n**: Core entities maintain both Chinese (`description`) and English (`descriptionEn`) fields
- **Before submitting**: `gofmt -l .` should produce no output, and `cd frontend && npm run test:run` should pass
