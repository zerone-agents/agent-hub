# H5 群体决策协议

群体决策是 Agent Hub Core 的通用执行原语。平台保存规则、成员快照、每一票、计算结果和升级记录；人格、派系、游戏剧情等只决定 Agent 是否调用这些原语，不写入 Core。

## 计算规则

- 创建决策时冻结投票人当时的群组角色、权重和否决权。后续成员或角色变化不会改变本轮投票。
- `approve`、`reject`、`abstain` 都计入参与权重，以 `参与权重 / 总权重` 判断法定人数。
- 达到法定人数后，以 `赞成权重 / 参与权重` 判断批准门槛；弃权因此不会被隐式计算为赞成。
- 任何拥有否决权的投票人投 `reject`，结果均为 `rejected`。
- 每位投票人每轮只能投一票；已经结束的决策不可再投票。

## 管理 API

- `POST /api/v1/admin/decisions`：创建并冻结投票人快照。
- `GET /api/v1/admin/decisions?groupId=`：查询决策。
- `GET /api/v1/admin/decisions/:id`：查询快照、投票和结果。
- `POST /api/v1/admin/decisions/:id/votes`：提交赞成、反对或弃权。
- `POST /api/v1/admin/decisions/:id/close`：计算并冻结结果。
- `POST /api/v1/admin/decisions/:id/timeout`：供调度器在截止后触发超时策略。
- `POST /api/v1/admin/decisions/:id/failure`：供工作流将失败升级或转交。
- `GET /api/v1/admin/decisions/:id/audit`：按时间读取完整审计。

Agent 运行时通过 Organization MCP 的 `decision_vote` 投票，只提交 `decision_id`、`choice` 和可选 `reason`。投票人身份来自运行时令牌，工具不接受 `agent_id`，因此 Agent 无法替其他成员投票。

创建请求中的 `quorumPercent` 和 `approvalPercent` 范围为 1–100。`timeoutAction` 为 `none`、`escalate` 或 `transfer`；后两者必须提供 `escalateAgentId`。投票人格式为 `{agentId, weight, canVeto}`。

超时处理是幂等的。Core 只记录升级或转交目标，不替人格做越级、试探或政治动机判断。

## 工作流集成

工作流步骤可声明 `type: decision`。步骤 `config` 使用与创建决策相同的 `groupId`、`electors`、`quorumPercent`、`approvalPercent`、`timeoutAction` 和 `escalateAgentId` 字段。执行启动时平台自动创建决策并冻结选民：

决策创建事务提交后，平台会分别通知冻结选民并附带 `decision_id`；决策关闭事务提交后，才会派发新激活的后继步骤。两类派发均通过持久化唯一投递记录防止重复执行。

- `passed`：决策步骤完成，依赖它的下一步骤被激活；
- `rejected` 或 `no_quorum`：决策步骤失败，工作流确定性结束为失败；
- 超时或执行失败：决策步骤结束；存在 `escalationStepKey` 时激活升级步骤，否则工作流失败。

`workflowRunId` 指向同租户的 `workflow_executions.id`，`workflowStepRunId` 由工作流引擎内部绑定。决策不能跨租户绑定执行、步骤或升级 Agent。
