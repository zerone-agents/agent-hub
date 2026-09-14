# H7 验收记录

独立审查 Agent 每阶段结论 + 最终页面验收清单。

## 阶段审查结论

| 阶段 | 结论 | P0/P1 | 审查人 |
|---|---|---|---|
| H7.0-H7.6 统一集成（6ea204b） | NO-GO | P1×3：迁移日志 From 污染 / inverse 跨 Schema 套用 / upstream 无 SSRF 阻断 | agent-18 |
| P1 修复（3500163） | GO | 3 项 P1 全部修复并带回归测试（多 Schema 回滚逐字节恢复、v3→v2 不越界、私网 upstream 拒绝+转发前重解析）；P0=0 | 主 Agent 复核 |
| 服务器端到端冒烟（部署后） | 24/25 | FAIL 1：模板安装 GroupMember.JoinedAt 零值被 MySQL NO_ZERO_DATE 拒绝（SQLite 测试环境恰好放行，环境差异 bug） | agent-20 |
| 修复 437801f + 复测 | 25/25 PASS | 模板安装/幂等重放/同 key 异 mapping 409/冲突检测 409 全过；测试产物与临时 token 全部清理零残留 | agent-20 复测 |

## 最终页面验收（12 步，用户操作）

1. 查看扩展市场或扩展列表
2. 打开扩展详情，理解它增加什么能力
3. 查看权限并安装
4. 启用和停用扩展
5. 从 v1 升级到 v2
6. 回滚到 v1
7. 安装一个通用团队模板
8. 安装 Speeding 示例模板或 H6 动态人物示例包
9. 启动 Run 并看到扩展提供的页面
10. 查看扩展调用、Token、费用和错误
11. 停用扩展后，旧数据仍然可读
12. 卸载前看到依赖和影响范围

## 最终交付信息（交付时填）

- 已实现范围：H7.0 扩展注册中心（manifest 严格校验/版本/管理 API/详情页）、H7.1 生命周期（安装/启停/升级/数据迁移/回滚/卸载/依赖检查/失败回滚/影响范围）、H7.2 UI 插槽（7 类插槽声明/4 种声明式组件/错误边界/降级/代理限流）、H7.3 模板库（预览/映射/冲突检测/幂等/部分选择/导出/团队+Speeding 种子模板/安装向导）、H7.4 权限与隔离（grants auto/approval、Enforce 中间件、审计、撤销、限流、敏感字段掩码、11 项跨租户隔离矩阵）、H7.5 用量与运维（Token/费用/错误率/趋势/预算/健康/CSV 导出/告警）、H7.6 SDK/CLI（zerone 五命令/ed25519 签名/Go SDK/示例扩展/开发者指南/migrate-manifest）
- 未实现范围：组织级情绪/认知（按 H6 分层决策本轮不做，只留字段）；支付/扣费/发票；workflow.detail.tab 插槽挂载（页面结构复杂，列入后续）；扩展代理 extension_call 埋点待联调；多副本部署的分布式限流（当前进程内）
- 页面验收地址：服务器 Hub 前端（与 H6 相同入口），导航新增「模板库」「用量运维」，「扩展能力」页升级为完整注册中心
- 验收步骤：见上方 12 步清单
- 测试结果：go test ./... 31 包全绿（含 -race）；前端 vitest 80 文件 518 用例全过 + tsc + 生产构建通过
- 独立审查结论：首轮 NO-GO（P1×3）→ 修复后 GO，P0=0；3 个 P1 均有回归测试（多 Schema 回滚逐字节恢复、v3→v2 不越界、upstream 私网拒绝+转发前重解析）
- Git 提交：3500163（修复后 HEAD），分支 codex/agent-relations 已推送
- 部署版本：zerone-agent-hub-h6:latest（镜像 9b54f1b5ab10），容器 quickstart-hub-1，AutoMigrate 建 12 张新表，存量数据核验无损（8 agents/1 群组/17 关系/7 人物状态），H6 persona MCP 回归通过
- 已知限制：见"未实现范围"；启动日志有一条 state_schemas 重复注册 warning（H6 persona 启动幂等注册与种子模板撞名，非致命，已被错误处理吞掉）；CommitState 幂等只查 key 不查内容（H6 既有语义，P2）；用量埋点在 chat 路径暂无 model 名（runtime 不回传）

## 部署后缺陷修复（2026-09-14 晚）

- 问题：用户打开运行详情页白屏，`Cannot read properties of undefined (reading 'map')`。
- 根因：`/runs/:id/belief-disputes` 接口的 `Dispute`/`DisputeEntry` Go 结构体缺 JSON tag，序列化成 PascalCase（`FactRef`/`Entries`），前端读 `dispute.entries` 得到 undefined 后 `.map` 崩溃。既有单测用 `json.Unmarshal` 反序列化断言，而 Unmarshal 匹配 key 不区分大小写，把该缺陷漏过了（H6 冒烟 therefore 未暴露）。
- 修复（commit 5d724c1）：
  1. 后端补 `json:"factRef"/"entries"/"agentId"/"status"/"confidence"` tag；
  2. 单测增加原始报文大小写断言（`"factRef"` 必须存在、`"FactRef"` 必须不存在）防回归；
  3. 前端 PersonaPanel 对无 `entries` 的争议条目做防御性过滤。
- 验证：go test（handler + services）通过；前端 vitest 18 用例通过；生产构建通过；部署后实测接口返回 `{"factRef":...,"entries":[{"agentId":...}]}`，线上首页引用新 chunk（index-Bd1Vx_D9 / RunCenterPage-CuB6aYUJ）。

## 第二轮审查修复（2026-09-14 晚，审查结论 NO-GO 后的整改）

审查发现 2 个 P0 + 2 个 P1，全部修复并部署（HEAD 2de2d6d）：

1. **P0 扩展权限未生效**：`ExtensionAuthz` 中间件已实现但从未挂载到路由。已挂到 admin 核心路由（runs 读写、states 读写、persona 视图、agents 只读、workflow 读写），Enforce 语义确认为默认拒绝（无 grants 记录即 403）。回归测试：无头请求行为不变、无授权扩展 403、已授权扩展放行、未知扩展拒绝。（eda6540）
2. **P0 SSRF 绕过**：代理转发存在 DNS 重解析 TOCTOU 与 302 跳转绕过。新增 `ValidateAndResolveIPs` + `SecureHTTPClient`（DialContext 固定已校验 IP、TLS ServerName 保持域名、禁止跟随跳转并把 3xx 原样透传含 Location）。（4f6bfed）
3. **P1 生命周期半完成状态**：Install/Upgrade/Rollback/Enable/Disable/Uninstall 的授权同步全部纳入同一事务，同步失败整体回滚（页面报错且状态未变）。新增 6 个失败注入回归测试。（fa8552e）
4. **P1 H6 硬编码**：情绪/认知/记忆/动态关系注册为四个内置扩展（io.zerone.emotion 等，默认已安装已启用，行为零变化），新增 PersonaCapabilityGate 运行时门控：停用后提示词不再注入记忆、工作流情绪/关系钩子不生效、组织 MCP 对应工具返回"能力已被停用"；数据与 schema 保留、persona 只读视图不受限。（9941f77 + fe30ee6 + 2de2d6d）

另修复交付可追溯性：
- 全部改动为真实 Git 提交并推送，GitHub `codex/agent-relations` HEAD = 2de2d6d；本地主仓已快进同步（README.zh-CN.md 的未提交改动原样保留）。
- 服务器 `/opt/speeding/current` 软链已指向实际运行目录 `/opt/speeding/releases/agent-hub-h6-03d243b`。
- 部署后验证：四个内置扩展 installed/enabled（查库确认）；H6 回归基线 emotion_status 返回 `{"mood":"angry","intensity":-75}` 不变。

遗留决策项：H7.4 权限白名单十类不含 "run"，扩展无法声明 run 类权限（当前对带扩展头的 run 端点请求恒 403，最严默认拒绝）。若 SDK 需要扩展读 run 数据，需 spec owner 决定是否在白名单加 "run" 类。
