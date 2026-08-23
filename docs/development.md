# Local Development

Local development guide: decoupled frontend/backend startup, testing, build artifacts, and team conventions.

## Prerequisites

- Go ≥ 1.25
- Node.js ≥ 22
- Docker + Docker Compose

## Option 1: Docker Compose (Recommended)

```bash
git clone https://github.com/zerone-agents/agent-hub.git
cd agent-hub/quickstart

cp .env.example .env   # 至少设置 AUTH_JWT_SECRET（openssl rand -hex 32）
docker compose up -d   # 默认拉取 zeroneai/agent-hub 镜像

# App listens on http://localhost:8081/static/
# Health check: curl http://localhost:8081/health
```

要从源码本地构建镜像，用 `docker-compose.build.yml` 覆盖文件叠加：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

On first startup, it will automatically:
1. Run AutoMigrate to create all tables
2. Seed 5 shared provider templates — `anthropic-thirdparty` / `openai-thirdparty`（自定义兼容 API 模板，填写 base_url + api_key）/ `glm-cn`（GLM Coding Plan）/ `kimi-cn`（Kimi Code）/ `bailian`（Aliyun Bailian）——均为共享模板行，**不预填任何密钥**，各租户首次使用时按 copy-on-write 复制后填写

> 默认 builtin 认证模式：首访浏览器会弹出初始化页面创建 `admin` 账号，无需 Casdoor。

## Option 2: Local Development (Frontend/Backend Decoupled)

**Start dependencies** — 仓库根目录没有 compose 文件，MySQL 用 quickstart 的（或自备实例）：

```bash
cd quickstart && docker compose up -d mysql && cd ..
```

**Start backend** — 默认 builtin 模式的最小环境变量集：

```bash
export DATABASE_URL="root:root@tcp(localhost:3306)/agent_hub?charset=utf8mb4&parseTime=True&loc=Local"
export AUTH_JWT_SECRET="<openssl rand -hex 32>"
export AUTH_MODE=builtin   # 默认值，可省略

# 可选（按需）：
export PROVIDER_ENCRYPTION_KEY="<64-hex-chars>"   # Provider 密钥加密
export OSS_ENDPOINT="http://localhost:9000"       # OSS 整体可选，留空即禁用
export OSS_BUCKET="agent-hub" OSS_ACCESS_KEY="minioadmin" OSS_SECRET_KEY="minioadmin" OSS_FORCE_PATH_STYLE="true"
export MULTIRAG_BASE_URL="http://localhost:8000"  # 知识库可选
export MULTIRAG_API_KEY="<multirag-service-api-key>"

go run ./cmd/server
```

> quickstart 的 MySQL 容器默认不向宿主机暴露 3306（compose 中被注释掉），本地直连需自行放开端口映射，或另起 MySQL。数据库名默认 `agent_hub`。

**casdoor 模式（可选）**：如需本地调试 SSO，追加 `AUTH_MODE=casdoor` + `CASDOOR_ENDPOINT` / `CASDOOR_CLIENT_ID` / `CASDOOR_CLIENT_SECRET` / `CASDOOR_CERTIFICATE`（正常开发无需任何 `CASDOOR_*` 变量；`CASDOOR_ORGANIZATION` 仅作为存量数据无法归属租户时的一次性升级逃生舱，本地新环境不需要）。

**Start frontend**:

```bash
cd frontend
npm install
npm run dev    # Default port 7002; /api and /auth proxied per vite.config.ts
```

> vite dev server 默认把 `/api`、`/auth` 代理到 `vite.config.ts` 中配置的目标（当前为线上环境地址）。本地起后端时请把 `server.proxy` 的 `target` 改为 `http://localhost:8081`（按需修改，勿提交）。

> 💡 Bypass login during development: set `VITE_BYPASS_AUTH=true` in `frontend/.env.local` (already ignored by `.gitIgnore`).

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
go test ./...
```

测试位于各包内（`internal/handler/*_test.go`、`internal/application/services/*_test.go` 等），新增功能请随代码补充对应测试。

---

## Team Conventions

- **Git**: Conventional Commits (`feat:` / `fix:` / `refactor:` / `chore:` / `style:` / `docs:`)
- **Error messages**: User-facing errors in Chinese, internal errors with English stack traces
- **Field naming**: DB fields snake_case, JSON fields camelCase, Go fields PascalCase
- **i18n**: Core entities maintain both Chinese (`description`) and English (`descriptionEn`) fields
- **Before submitting**: `gofmt -l .` should produce no output, `go vet` passes, and `cd frontend && npm run test:run` should pass
