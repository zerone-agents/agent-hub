# Deployment

## CI / Release (`.github/workflows/`, GitHub Actions)

```
ci.yml    → lint (gofmt + go vet) / test (go build + go test) / frontend-test / frontend-lint
release.yml → 由 `hub-vX.Y.Z` tag 触发：完整 lint + test 全绿后构建镜像，双推：
             · swr.cn-east-3.myhuaweicloud.com/zerone/agent-hub (vX.Y.Z + latest)
             · docker.io/zeroneai/agent-hub                    (vX.Y.Z + latest)
cli-publish.yml → zhub CLI 发布流水线
```

GitHub Release 不由 workflow 自动创建：workflow 成功后手动 `gh release create`（标题 `Hub-vX.Y.Z`，标记 Latest）。

发布前检查项：main 最新 push 的 CI + CodeQL 全绿；本地 `gofmt -l` / `go vet` 通过。

## Image

- Multi-stage build：前端（node:22-alpine，vite）→ 后端（golang:1.25-alpine）→ 运行层 `alpine:latest`（可按需固定为 `alpine:3.20`）
- Final image only contains `server` binary + `ca-certificates` + `tzdata`
- Exposed port: `8081`
- Health check: `GET /health`

## Quick Start

见 `quickstart/`（`docker-compose.yml` 默认拉取 `zeroneai/agent-hub` 镜像，builtin 认证模式；`docker-compose.build.yml` 为本地构建覆盖文件，`-f` 叠加使用）。

## Trusted Proxies 与审计来源 IP

审计日志（`audit_logs.remote_ip`）记录每条写操作的来源 IP。Hub 默认**不信任任何代理**（直连语义）：取 TCP 对端地址、忽略 `X-Forwarded-For`——这是安全默认值，但在反向代理之后部署时来源 IP 会失真。

Hub 部署在反向代理（Nginx / Kong 等）之后时，必须把代理网段配进 `SERVER_TRUSTED_PROXIES`，否则审计记录的 IP 只会是代理地址：

```bash
SERVER_TRUSTED_PROXIES="10.0.0.0/8,172.16.0.0/12"
```

- 逗号分隔的 CIDR 列表（每项去空白，空项丢弃）
- 未设置 / 空值 / 纯空白 = 不信任任何代理（不是错误）
- 非法 CIDR 启动直接失败（fail-fast）：错误配置后的信任状态不可保证，宁可拒绝启动
- 只把真正直连 Hub 的代理网段加入列表——`X-Forwarded-For` 可伪造，信任过宽 = 审计 IP 可被伪造

## Production Deployment Checklist

- [ ] Set `PROVIDER_ENCRYPTION_KEY` to a strong random value (do not commit to the repository)
- [ ] Explicitly configure `SERVER_CORS_ORIGINS` (do not use the default AllowAll)
- [ ] 部署在反向代理（Nginx / Kong 等）之后时，设置 `SERVER_TRUSTED_PROXIES` 为代理网段 CIDR，否则审计日志 remote_ip 记录的是代理地址（见「Trusted Proxies 与审计来源 IP」）
- [ ] ⚠️ **SaaS / 已有 Casdoor 的部署升级时必须显式设置 `AUTH_MODE=casdoor`**（默认 `builtin` 会导致 SSO 失效，看起来像账号丢失；breaking change）
- [ ] 需要接入多组织登录时，配置 `OPS_API_KEY` 并按 configuration.md 的 runbook 登记各组织 Casdoor client
- [ ] v2.0.0 升级演练：存量 agent 需逐个重新部署以落位租户限定部署键（三步迁移见 configuration.md「部署键与 runtime URL」）；仅当存量数据无法自动归属租户时，临时设置 `CASDOOR_ORGANIZATION` 作为一次性回填逃生舱（完成后移除）
- [ ] 审批初始 admin：builtin 模式首访创建；casdoor 模式下 Casdoor 组织管理员登录自动成为 admin（无需在 Casdoor 创建任何 agent-hub 角色），其余用户登录后待审批分配角色
- [ ] OSS 整体可选（`OSS_ENDPOINT` 留空即禁用）；启用时将 Bucket 设为公共读或配置 `OSS_CDN_HOST` CDN 加速 Skill 下载
- [ ] Back up the database regularly (`cloud_sessions` / `cloud_messages` are user data)

## Single ECS Deployment Recommendation

See [architecture.md#recommended-configuration-for-single-ecs-deployment](architecture.md#recommended-configuration-for-single-ecs-deployment).
