# 任务 1 交付报告

- 分支：`codex/game-seats`
- 最后提交：在该分支执行 `git rev-parse HEAD`（精确哈希同时写在 PR 描述中）
- PR：https://github.com/zerone-agents/agent-hub/pull/252（草稿）

## 从零运行

按 [AI 座位服务本地启动](../game-seats-setup.md) 配置现有 Hub DeepSeek Provider、服务密钥、数据库，再运行种子命令和 Hub 服务。

## 验收

| 条目 | 结果 | 证据 |
|---|---|---|
| roster 返回 6 个居民 | 通过 | 本地 SQLite 启动 Hub 后，`GET /roster?gameKey=wolfcha` 返回 6 条；无密钥返回 401。 |
| 同一玩家两局共享记忆 | 通过（模拟模型） | `TestGameSeatVotePersistsAcrossSessionsAndIsIdempotent`：第二局生成时，新增系统消息包含第一局“玩家投票给老周”，原 messages 保持原内容。 |
| 投票改变情绪、态度，重复事件不重复写 | 通过 | 同一测试；本地 curl 显示态度 -20、情绪强度 -20、记忆 1 条；重复提交 204 后数值未变。 |
| 普通与流式 OpenAI 返回 | 通过（模拟模型） | 本地 curl：普通请求返回 `chat.completion` JSON；流式返回 `chat.completion.chunk` SSE 和 `[DONE]`。 |
| 超出预算 | 通过 | Go 测试把预算设为 2，第 3 次调用返回 `ErrGameSeatBudget`；HTTP 映射为 429。 |
| `go test ./...` | 未通过（原分支问题） | 基线运行已出现 `TestGroupSendFansOutAsIndependentAsyncDeliveries` 超时；新增实现后的全量运行仍出现该测试及 `TestRunWorkersDiscoversTenantsOnceAndStopsWithContext` 超时。新增测试单独通过。 |

真实 DeepSeek 调用尚未验证；本地使用模拟 OpenAI 兼容服务验证协议。运行真实演示时需要在 Hub 后台配置有效的 DeepSeek Provider 和模型。

## 文件与原因

| 文件 | 说明 |
|---|---|
| `internal/domain/gameseat/model.go` | 保存居民、跨局 Run 绑定、每局 Session、座位和事件幂等序号。 |
| `internal/application/services/game_seat_service.go` | 复用 Run、Memory、Emotion、RelationDynamics，并代理文字模型。 |
| `internal/application/services/game_seat_seed.go` | 幂等创建 6 个固定居民 Agent 与 roster，音色留空。 |
| `internal/application/services/game_seat_service_test.go` | 验证跨局记忆、投票幂等、模型响应和预算。 |
| `internal/handler/game_seat.go` | 提供约定的 HTTP 接口及独立服务密钥鉴权。 |
| `cmd/seed-game-seats/main.go` | 提供重复执行的居民种子命令。 |
| `cmd/server/main.go` | 挂载迁移和服务路由。 |
| `docs/game-seats-setup.md` | 记录启动、配置与接口检查。 |

## 差异与已知问题

- Hub 的 Run 人格能力仍在 `9ee76a2` 开发基线，未进入远端 `main`，因此本 PR 需以该基线为目标；后续合并到 `main` 依赖基线先合入。
- 当前对局预算按 Session 的模型调用次数计数；上游模型失败也占用一次预算。
- 本地 curl 使用模拟模型；没有真实 DeepSeek 响应时延与内容质量数据。
- `voice` 留空，符合第 3 版任务 1 的要求。
