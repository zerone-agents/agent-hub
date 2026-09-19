# ADR-0003：事件投递、顺序与因果链语义

- 状态：Accepted for H0
- 日期：2026-09-12
- 关联协议：`agenthub.extension/v1alpha1`

## 背景

Agent 消息、工具调用和状态变化会产生多跳副作用。必须允许真实的
`A → B → A → B`，同时避免重复扣款、无限递归、同步死锁和上下文越权。

## 决策

平台采用 **持久化、默认异步、至少一次投递、消费者幂等** 的事件模型。

- 每个事件包含 `id`、`tenantId`、`runId`、`type@version`、`source`、`actor`、
  `subject`、`correlationId`、`causationId`、`rootEventId`、`idempotencyKey` 和时间。
- 派生事件继承 `correlationId/rootEventId`，并把直接原因写入 `causationId`。
- 同一 `run + subject` 的提交应稳定排序；不承诺不同主体或不同 Run 的全局顺序。
- 重试不得创建新的业务效果；消费者以事件 ID 或业务幂等键去重。
- 失败经过有限重试和退避后进入死信，并产生可审计状态。
- 每次跨 Agent 投递必须重新验证静态关系动作、上下文策略和当前 Run 权限。
- 因果链受 hop、事件数、工具次数、Token 和截止时间预算约束；超限产生 guard 事件。
- 同步模式只允许有界的单跳请求响应，不允许同步调用栈循环等待；后续传播转为事件。

## 时间字段

事件区分：

- `occurred_at`：业务事实发生时间；
- `scheduled_at`：允许消费的时间；
- `recorded_at`：平台持久化时间。

Speeding 的虚拟时间可作为扩展字段或应用时间源，但不得替代平台审计时间。

## 一致性边界

- 单次状态 commit 与其 `state.changed` 事件必须通过事务性 outbox 保持一致。
- 外部副作用采用 proposal/confirmation 或补偿机制，不宣称 exactly-once。
- H2 允许先使用数据库队列；协议不绑定 Kafka、Redis 或特定云服务。

## 明确不做

- 不用进程内 `active` 标志作为最终循环治理方案。
- 不把“禁止目标 Agent 再发消息”作为防爆策略。
- 不沿因果链自动继承更宽的数据可见性。
