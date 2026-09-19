# Agent 关系验收矩阵

最后验证时间：2026-09-11（Asia/Shanghai）

## 线上真实 Agent 路由

以下案例均由已部署的 Aliyun Bailian `qwen3.6-plus` Agent 先调用
`agent_relations` 读取有向边，再调用 `agent_send`；异步投递继续通过
`agent_message_status` 查询到终态。仅创建数据库关系不计为通过。

| Case | 关系类型 | 路由 | 动作 | 投递 | Message ID | 结果 |
|---|---|---|---|---|---|---|
| CASE-LIVE-DIRECTOR-01-1 | oversight | speeding-director → speeding-mo-yuncen | report | sync | `dc6a2450-ccac-44a1-ae47-9ebb6c60e38a` | completed |
| CASE-LIVE-DIRECTOR-01-2 | advisor | speeding-director → speeding-xie-heng | consult | sync | `71b11dbe-dd7d-48ec-bb4e-34e9782e1409` | completed |
| CASE-LIVE-DIRECTOR-01-3 | peer | speeding-director → speeding-shen-lu | inform | async | `ff200f4c-5405-4054-9c13-191bdb46911e` | completed |
| CASE-LIVE-DIRECTOR-01-4 | opponent | speeding-director → speeding-luo-chongxiao | challenge | async | `99aa9725-eebf-42ce-b43f-b2ebcd338814` | completed |
| CASE-LIVE-DIRECTOR-01-5 | external | speeding-director → speeding-qiu-yan | inform | async | `2dc26374-0bda-47ba-9dc8-8f64ae57f0fc` | completed |
| CASE-LIVE-DIRECTOR-01-6 | representative | speeding-director → speeding-zhong-rui | report | sync | `02653e84-d3eb-49bd-9f60-38ea6b2cf434` | completed |
| CASE-LIVE-REPORTS-TO-01 | reports_to | speeding-mo-yuncen → speeding-director | report | async | `0acf580d-ae9b-4582-89d0-6b043ee68b0b` | completed |
| CASE-LIVE-REVIEWER-01 | reviewer | speeding-zhu-jian → speeding-director | challenge | async | `e8c5df63-3cc8-47b5-b43e-8dc63570d14e` | completed |

同一目标的两个异步案例会串行进入目标队列，曾短暂保持 `running`，之后均完成；
这证明队列没有丢消息，但也表明 UI 后续应显示排队位置和运行耗时。

## 关系动态

`CASE-DYNAMIC-01` 在 `speeding-director → speeding-shen-lu` 上记录
`task_completed` 事件，关系分值由 `0` 变为 `+10`，事件原因、时间和前后分值均可复盘。

## 自动化回归

- 8 种基础关系在没有反向边的纯单向 `A → B` 条件下均可送达。
- 双向关系会落成两条可独立编辑、独立删除的有向边。
- `A → B → C` 可在两个独立回合中依次送达。
- 反向无边、动作不在白名单、跨租户读取均被拒绝。
- `none`、`summary_only`、`shared_thread` 三种上下文策略分别执行隔离。
- `sync` 和 `async` 均覆盖；同目标并发消息串行排队。
- 目标处理当前消息时嵌套转发会被拦截，防止单轮递归爆炸；后续独立回合仍可继续发送。
- 关系事件按幂等键去重，分值有界，并保存变更前后值。

验证命令：

```bash
go test ./... -count=1
npm --prefix frontend test -- --run
npm --prefix frontend run build
```

本次结果：Go 全量测试通过；前端 73 个测试文件、474 个测试通过；生产构建通过。
