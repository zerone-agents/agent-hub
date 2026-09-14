# h6-persona-demo —— H6 情绪包的扩展机制参考实现

本目录回答一个问题：**H6 动态人物包如何用 H7 扩展机制实现？**

H6 的四个能力包（情绪/看法/记忆/关系）在 `docs/h6-dynamic-persona.md` 中以协议
文字描述，在 `internal/domain/emotion/` 等处以 Go 代码落地。本扩展把其中
**io.zerone.emotion（心情包）** 的全部语义形式化为严格 v1 manifest 声明：

| H6 协议概念 | 本扩展的声明位置 |
|---|---|
| emotion-state v1 状态（scope=run, subject=agent） | `stateSchemas[].payload`（字段与 `internal/domain/emotion/schema.go` 的 SchemaDocument 一一对应） |
| 心情事件词表（8 个词表项 + delta + 严重度系数） | `events[]`（betrayed/trusted/...，payload 带 delta 与 severityScale） |
| 叙述规则（强度分档 → 固定句式，模型不可发挥） | `promptInjections[].payload.template`（确定性模板，audience=agent:self） |
| `emotion_status()` MCP 工具（只读自己） | `tools[]`（effects=read，subject=caller_agent） |
| 衰减/钳制等结算规则 | 由平台规则引擎按声明执行（模型与结算分离） |

## 关键约束（H6 安全模型的声明式表达）

- **提示词不授予权限**：promptInjections 只是 dynamic_state 阶段的叙述注入；
  工具调用仍走原授权（relationActionsAnyOf）。
- **敏感字段**：audience=agent:self——心情叙述只进持有者本人的提示词，
  管理端只读可查（审计需要）。
- **确定性**：状态转移是纯函数；相同事件序列可重放，与读取时刻无关。
- **卸载安全**：停用/卸载本扩展后，Core 功能（群组、消息、工作流）不受影响，
  历史 run_states 数据依旧可读。

## 其余三包

看法（io.zerone.belief）、记忆（io.zerone.subjective-memory）、
关系动态（io.zerone.relationship-dynamics）按同样模式声明即可：
stateSchemas 承载状态结构，events 承载词表，promptInjections 承载注入片段，
tools 承载 MCP 只读接口。
