# AI 座位服务：本地启动

服务在 Agent Hub 中启用，接口前缀为 `/api/v1/game-seats`。先在 Hub 后台配置一个有 Provider 和模型的 Agent，再执行种子命令。设置 `WOLFCHA_TEMPLATE_AGENT_NAME` 可指定现有 Agent；不设置时使用第一个有模型配置的 Agent。种子命令会复制其模型配置给 9 个居民 Agent，并可重复执行。

```bash
export DATABASE_URL='sqlite:./game-seats-demo.db' # 或使用现有 Hub 数据库
export GAME_SEATS_SERVICE_KEY="$(openssl rand -hex 32)"
export GAME_SEATS_CALL_BUDGET=120
export WOLFCHA_TEMPLATE_AGENT_NAME='你已有的 Agent 名称' # 可选
# 按 Agent Hub 的现有方式配置并启动服务，后台先配置 Provider/Agent

go run ./cmd/seed-game-seats
go run ./cmd/server
```

服务密钥只设置在服务器环境变量，Wolfcha 的 `ZERONE_SERVICE_KEY` 使用同一值。`GAME_SEATS_SERVICE_KEY` 未设置时，服务路由不会挂载。`GAME_SEATS_CALL_BUDGET` 默认每局 120 次。种子数据为九名居民配置了不同的百炼 `qwen-audio-3.1-tts-flash` 音色；Wolfcha 服务端负责语音合成和播放，Hub 只返回音色 ID。

```bash
curl -H "Authorization: Bearer $GAME_SEATS_SERVICE_KEY" 'http://localhost:8081/api/v1/game-seats/roster?gameKey=wolfcha'
```

当前分支基于包含 Run 人格能力的开发提交 `9ee76a2`，这些能力尚未进入远端 `main`。先合入其基线，再合入 AI 座位服务分支。
