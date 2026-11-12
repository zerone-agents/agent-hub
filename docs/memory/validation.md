# 记忆功能验证指南

该功能默认开启，配置好底层知识服务连接后可用；当前合同只接收 `dataClass=synthetic`。以下检查覆盖 Hub 的接口与消费规则；不代表生产准入、敏感数据保护、跨引擎一致性或性能承诺。

## 自动化检查

在仓库根目录执行：

```sh
gofmt -l .
go vet ./...
go test ./...
go test -race ./internal/memory ./internal/infrastructure/nativememory
cd frontend
npm ci
npm run lint
npm run test:run
npm run build
```

MySQL 集成测试使用专用测试实例。`TEST_MYSQL_DSN` 用于已有数据库迁移测试；记忆服务另用 `MEMORY_TEST_MYSQL_DSN`，要求回环 TCP 地址及空白的 `hub_memory_test_<8-32位十六进制>` 专用数据库，调用方负责建库和清理。CI 在其临时 MySQL 8 容器内单独创建该库，测试后清理。未设置对应变量时相关测试会跳过，不能据此声明 MySQL 已验收。不同数据库或存储引擎需另行验证。

| 范围 | 仓库内检查 |
| --- | --- |
| 应用配置、凭据与生命周期 | `internal/config/memory_test.go`、`internal/memory/service_test.go`、`internal/memory/subjects_test.go` |
| 原生对话、幂等及未知写入恢复 | `internal/memory/conversations_test.go`、`internal/infrastructure/nativememory/conversation_test.go` |
| 原生配置、启停、遗忘及召回诊断 | `internal/handler/memory_management_test.go`、`memory_forget_test.go`、`internal/infrastructure/nativememory/management_test.go`、`forget_test.go`、`diagnostics_test.go` |
| 后台角色、租户委托与审计 | `internal/handler/memory_admin_test.go`、`memory_management_test.go` |
| 临时上下文、会话历史及明确零工具 | `internal/handler/runtime_proxy_context_test.go`、`runtime_proxy_history_test.go`、`internal/application/services/agent_zero_tools_test.go` |
| 管理界面、状态与范围切换 | `frontend/src/features/memory/*.test.tsx`、`memoryApi.test.ts` |

## 原生自动记忆的真实服务检查

按 [接入说明](README.md) 配置独立私有记忆库、真实聊天及 embedding 模型、原生抽取 worker 和获委托的 Hub 管理账户。使用独立的合成用户与生命周期，不复用个人资料或生产数据。

1. 激活生命周期；读取 capabilities，确认自动提取和预期类型已启用。
2. 通过实际聊天入口生成回复，提交完整实际 `userInput` 与 `agentResponse`。确认原文读回一致，不手工制造派生记录。
3. 按原 operation ID 查询状态。分别观察原文存在、派生可读、任务完成；失败或未知不能标为完成。
4. 重复相同 operation 和同一 session/turn，确认不新增原文；同键异正文应冲突。对响应丢失仅做只读对账，不更换编号重发。
5. 新建 runtime 会话，保持同一业务生命周期，读取此前记忆。提交新的实际纠正对话，再以新会话核对最新原文进入上下文。原生排序仍可能包含旧记录，不要求所有旧派生自动消失。
6. 在管理页核对类型、正文、父来源、异步状态与配置；比较聊天供给和纯原生检索。后者不额外补最近原文，也不制造分数。
7. 在专用测试数据上停用单个派生，确认不影响兄弟；停用父原文，确认 Hub 不再供给父子内容。恢复父原文后，单独停用的子项仍停用。记录自身开关与实际供给状态分别核对。
8. 测试外用户、外生命周期、外应用及无委托管理员均无法读取或操作目标，拒绝后原生记录无变动。
9. 对本次专门创建的可遗忘合成记录，先保存原生直接检索、Hub 检索与聊天上下文的命中 ID，再从管理页遗忘原文。独立读回 `forget_at`，确认原文不再被底层记忆模块正常检索命中。原生接口不会级联打标派生记录：直接检索可能仍命中它们；Hub 因来源已遗忘而排除其供给，页面应区分“已遗忘”与“来源已遗忘”。
10. 从页面单独遗忘派生记录，再用同一查询、同一 bank/user/agent 范围复查：被遗忘的派生 ID 不再出现在原生命中中，Hub 检索与聊天上下文同样排除。管理列表保留记录并展示遗忘时间，启停开关和重复遗忘按钮禁用。刷新页面后状态应保持。不要把单次空结果代替“遗忘前可命中”的正向基线。

物理删除、授权撤回及注销清理仍不支持；不要调用这些接口清理验收数据。保留原始读回、查询参数、资源归属、实际模型回复和 worker 状态，不将仅 Hub 过滤后的空结果当作底层级联遗忘证明。

原生自动链路没有用户删除闭环，测试资源由隔离环境的管理员按其正常环境管理流程处理。上游模型、worker、SQL、向量存储和队列需要分别确认；Hub 单元测试或 HTTP 200 不能代替上述证明。

## 人工账本兼容检查

`scripts/memory-ledger-smoke.py` 仅针对单独配置的知识库账本后端，验证候选、审核、事实、纠正及该后端的清理合同；它不适用于原生自动记忆应用，也不能作为原生删除能力的证明。

```sh
HUB_MEMORY_URL=http://localhost:8080 \
HUB_MEMORY_TOKEN='<dedicated-test-service-token>' \
HUB_MEMORY_POLICY_VERSION=synthetic-v1 \
python3 scripts/memory-ledger-smoke.py
```

该脚本创建合成生命周期并执行知识库账本清理。只在专用账本测试应用运行，勿指向共享或生产应用。凭据从环境读取，不写入仓库。
