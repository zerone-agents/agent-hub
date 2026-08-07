<div align="center">

# Zerone Agent Hub

**开源 AI Agent 管理平台。**<br/>
统一控制台，配置、部署、管理你的 AI Agent。

[![License](https://img.shields.io/badge/License-Apache_2.0_Source_Available-blue)](./LICENSE)
[![GitHub stars](https://img.shields.io/github/stars/zerone-agents/agent-hub?style=flat)](https://github.com/zerone-agents/agent-hub/stargazers)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)

[快速开始](#快速开始) · [文档](#文档) · [生态](#生态) · [许可证](#许可证)

[English](README.md) | **简体中文**

</div>

---

> [!NOTE]
> **Zerone Agent Hub** 是 **source-available** 软件，基于 Apache 2.0 + 附加条款授权。
> 个人、教育、内部使用、小团队永久免费。
> 向第三方提供托管服务需获得授权。

## 什么是 Zerone Agent Hub？

Zerone Agent Hub 是 AI Agent 的控制平面——一个统一控制台，跨组织配置、部署、管理 agent。它管理 agent 定义、工具绑定、技能包、模型供应商凭据和会话归档，通过 manifest API 暴露给任何客户端（Claude Code 类 agent host、Electron 应用、CLI）消费。

**六大核心领域：** Agent · Tool · Skill · Provider · Scene · Chat

<!-- TODO: UI 完善后补充截图 -->
<!-- <p align="center"><img src="docs/assets/screenshot.png" width="800" alt="Zerone Agent Hub dashboard"></p> -->

---

## 快速开始

```bash
git clone https://github.com/zerone-agents/agent-hub.git
cd agent-hub/quickstart
docker compose up
```

打开 http://localhost:8080 即可使用。

本地开发（前后端分离 + 热更新）请见 [docs/development.md](docs/development.md)。

---

## 功能

- **Agent 管理** — system prompt、权限、图标、子 agent / 工具 / 技能关联
- **工具注册表** — 注册 agent 可调用的工具
- **技能包** — SKILL.md 打包，OSS 存储，支持 presigned 下载
- **Provider 预设** — Anthropic / OpenAI / GLM / Kimi / Bailian，Locked API Key AES-GCM 加密
- **场景模板** — 预设场景（绑定 Agent + Prompt 模板）
- **会话归档** — 客户端推送会话供管理员审计（支持 Markdown / tool_use / reasoning 分段渲染）
- **CLI 原生** — 通过 `@zerone-agent/zhub` 在终端管理一切（兼容 Claude Code、Cursor）

---

## 架构

Zerone Agent Hub 是 Go（Gin + GORM）后端 + 内嵌 React 19 SPA。它协调 [agent-runtime](https://github.com/zerone-agents/agent-runtime)（基于 Docker 的执行引擎），使用 Casdoor 做 SSO、MySQL 做存储、S3 兼容 OSS 存技能文件。

完整架构图、部署拓扑、项目结构请见 [docs/architecture.md](docs/architecture.md)。

---

## 生态

| 项目 | 说明 |
|---------|-------------|
| **Zerone Agent Hub**（当前仓库） | 管理控制台 — 主入口 |
| [open-agent-sdk](https://github.com/zerone-agents/open-agent-sdk) | 构建 Agent 的 SDK |
| [agent-runtime](https://github.com/zerone-agents/agent-runtime) | 基于 Docker 的 Agent 执行引擎 |

---

## 文档

- 📐 [架构与项目结构](docs/architecture.md)
- ⚙️ [配置（环境变量）](docs/configuration.md)
- 📡 [API 参考](docs/api-reference.md)
- 💻 [本地开发](docs/development.md)
- 🚢 [部署](docs/deployment.md)
- 🔐 [安全须知](docs/security.md)
- 🗄️ [数据模型](docs/data-model.md)

---

## 贡献

欢迎贡献！请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

快速约定：
- Conventional Commits（`feat:` / `fix:` / `refactor:` / `chore:` / `style:`）
- DB 字段 `snake_case` · JSON `camelCase` · Go `PascalCase`
- 提交前 `gofmt -l .` 应无输出

---

## 许可证

Zerone Agent Hub 是 **source-available** 软件，基于 [Zerone Agent Hub License](./LICENSE)。

**✅ 免费使用：**
- 个人和爱好者
- 教育和研究
- 单个组织内部使用（含多个 workspace）
- 基于 Zerone Agent Hub 后端/CLI 构建自定义方案

**💰 需要获得授权：**
- 向第三方提供 Zerone Agent Hub 托管服务（SaaS、托管服务）
- 移除或修改 Zerone Agent Hub UI 中的品牌标识（白标）

商业授权联系：**zerone-agents@proton.me**

完整条款见 [LICENSE](./LICENSE)。

---

<div align="center">

© 2025-2026 Zerone · 基于 [Zerone Agent Hub License](./LICENSE) 发布

</div>
