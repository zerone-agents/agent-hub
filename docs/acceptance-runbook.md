# 服务器验收 Runbook

每轮迭代的**唯一一遍验收**在服务器上做。本地只负责编译 + 单测绿，不上页面点验。
每步结果打勾或记偏差，全部走完约 10–15 分钟。

适用版本：H6 动态人物能力包（`codex/agent-relations` 分支，H6 实现完成后首验）。

## 0. 部署前

- [ ] CI（lint + 后端测试 + 前端测试）全绿
- [ ] 镜像构建成功并推送（`swr` + `docker.io` 双仓库）
- [ ] 服务器数据库已备份（`cloud_sessions` / `cloud_messages` 为用户数据）
- [ ] 记录部署的 commit hash（发布目录命名用）

## 1. 冒烟

- [ ] `GET /health` 返回 healthy，database 延迟正常
- [ ] 管理页面可登录（casdoor 模式确认 `AUTH_MODE=casdoor`，避免 SSO "失效"假象）
- [ ] 既有数据可见：旧群组、关系、运行记录未丢

## 2. H6 功能流（主战场）

前置：建 Agent A、B；创建一个 Run 并绑定四个能力包（emotion / belief / subjective-memory / relationship-dynamics）。

- [ ] **投递产生看法**：A 收到一条消息 → `belief_list` 出现该事实，状态 known/believed
- [ ] **未知事实不注入**：B 未收到该消息 → B 的 `belief_list` 无该事实；Run 的 prompt 预览（Compose 接口）中 B 的片段不含此事实
- [ ] **双认知争议**：给 A、B 投递同一事实的矛盾证据 → 两端 `belief_list` 状态分歧 → `GET /api/v1/admin/runs/:runId/belief-disputes` 可见争议条目
- [ ] **背叛三联动**：注入背叛事件（经工作流步骤或消息）→
  - [ ] A 的 `emotion_status` 心情下降、叙述句更新
  - [ ] A `memory_record` 写下背叛记忆后，`memory_recall` 可检索到
  - [ ] B `relation_view` 中 A 对 B 的态度分下降、stance 变化
- [ ] **关系影响执行**：态度降至 hostile（< -60，需 ≥2 次严重背叛）后 →
  - [ ] B 向 A 发起 assign 类请求：消息被标记需确认（`guardReason=relation_gate_need_confirm`），工具结果可见
  - [ ] B 向 A 发 inform：正常送达，无拦截标记
- [ ] **记忆进提示词**：Compose 预览中该 Agent 的片段含 `recent_memory` 阶段，内容与 recall 结果一致
- [ ] **跨局隔离**：同一 Agent 开第二个 Run → 看不到第一个 Run 的记忆与态度，心情回 baseline
- [ ] **跨租户隔离**（多租户环境）：租户 X 的管理端与 Agent 看不到租户 Y 的 persona-state
- [ ] **卸载安全**：禁用/卸载四个包 → 群组消息、工作流投票等 Core 功能正常；包工具返回"能力尚未启用"；Compose 不再渲染包片段

## 3. 回归

- [ ] H4：群组发送、频道发布、会议会话正常
- [ ] H5：工作流执行、投票（quorum / veto / 超时升级）正常
- [ ] 提示词合成：非 Run 会话的部署 system prompt 路径不受影响

## 4. 收尾

- [ ] 结果记录到版本说明（通过项 / 偏差项）
- [ ] 全部通过 → `docs/version-roadmap.md` H6 勾选并注明验收日期与 commit
- [ ] 有偏差 → 回滚镜像或记 issue，不打勾

## 已知限制（本轮验收不覆盖）

- 真实模型调用（百炼额度未恢复时，Agent 对话/投票派发的 AI 执行不可验）
- 组级聚合状态（团队氛围）未实现，非退出门槛
- Run 解析目前取 Agent 最近开始的 running Run；显式会话绑定后续补
