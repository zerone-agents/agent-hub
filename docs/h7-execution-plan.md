# H7 执行计划（主 Agent 持续更新）

状态文件。主 Agent 每完成一个内部阶段更新本文档；新主 Agent 可从本文档无缝接续。

## 总览

- 研发模式：主 Agent（项目经理+架构师+集成）+ 子 Agent 并行开发 + 独立审查 Agent。
- 内部阶段 H7.0–H7.7，用户只验收最终统一版本。
- 用户三次沟通：发任务 / 产品阻塞 / 最终验收。
- 公共文件（cmd/server/main.go、pkg/database/database.go、前端主路由/导航、统一提交、部署）只由主 Agent 改。

## 地基（已存在，复用不重建）

- `internal/extensionmanifest`：Manifest 校验器（schema/testdata）。
- `capability_packages` 表 + `GetEnabled`：H6 四包已按此注册。
- `run_state_changes` 审计、`StateSchema` 注册、租户隔离模式。
- docs/platform-extension-protocol.md（协议 v0.1，H7 将其落地为产品）。
- H6 动态人物包 = H7 扩展机制的验收包（情绪/认知/记忆/关系不得进 Core）。

## 阶段拆解与依赖

| 阶段 | 内容 | 依赖 | 负责 |
|---|---|---|---|
| H7.0 | 扩展注册中心：Manifest v1 模型、严格校验、版本、管理 API、详情页 | 现有 extensionmanifest | A |
| H7.1 | 生命周期：安装/启停/升级/迁移/回滚/卸载/依赖检查/失败回滚 | H7.0 | B |
| H7.2 | UI 插槽：7 类插槽、排序可见性、错误边界、降级 | H7.0 | C |
| H7.3 | 模板库：声明/预览/映射/冲突检测/幂等/导出 | H7.1 | D |
| H7.4 | 权限与隔离：声明式权限、撤销、审计、资源限制 | H7.0 | E |
| H7.5 | 用量与运维：Token/费用/错误率/预算/健康/导出 | H7.1 | F |
| H7.6 | SDK/CLI：create/validate/dev/pack、示例扩展、签名 | H7.0 | G |
| H7.7 | 独立审查 + 全量测试 + 统一提交 + 部署 + 页面验收 | 全部 | 主 Agent |

H7.0 必须先完成（所有阶段依赖注册中心）；H7.1 完成后 H7.3/H7.5 可并行。
每个阶段完成后独立审查 Agent 出 GO / NO-GO（P0/P1 清单），P0/P1 清零才进下一阶段。

## 当前状态

- [x] 任务拆解与计划落盘（本文档）
- [x] H7.0 扩展注册中心 — commit c980800，go test 全绿 + 前端构建过，接线完成（database.go/main.go/路由）；兼容层=双轨并存，H6 capability_packages 不动
- [ ] H7.1 扩展生命周期
- [ ] H7.2 UI 插槽
- [ ] H7.3 模板库
- [ ] H7.4 权限与隔离
- [ ] H7.5 用量与运维
- [ ] H7.6 SDK/CLI
- [ ] H7.7 审查/提交/部署/验收

## 最近提交

- `95c47b8` fix: negative emotion events never change mood（H7 起点）

## 测试命令

- 后端：`go test ./internal/...`
- 前端：`cd frontend && npm run build`（TypeScript 检查含在内）
