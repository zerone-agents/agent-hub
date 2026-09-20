# H3/H6/H7 修复后线上验收

- 部署版本：`1ae7562`
- 验收范围：隔离 `ACCEPT-H1H7` Agent/Run，未修改 Speeding Agent
- 临时 CLI Token：验收后已删除

## 结果

| 项目 | 结果 | 线上证据 |
| --- | --- | --- |
| Guarded 正文隔离 | 通过 | `2a6029da-1278-4752-a521-388b3360a97f`：目标 Agent inbox 无此消息；发送方仅可见 `guarded/route_not_found`，不可见 content/reply/idempotencyKey；管理审计仍保留原始尝试。 |
| 父链收口 | 通过（主链） | Conversation `5bf1118b-a8a8-45fe-b9e8-3f9ddcb5524e`；A→B 根消息、B→C 与 B→A 均为 `completed`。 |
| Usage 归因 | 通过 | Usage row `14`：Run/Agent/Message 已关联，provider=`bailian`，model=`qwen3.6-plus`，input=`3405`，output=`335`，total=`3740`；同 message 仅一条。 |
| H6 Run 绑定 | 通过 | Bound Run `6c0f57dc…a5a7` 绑定 emotion/belief/subjective-memory/relationship-dynamics 四包。 |
| H6 动态状态 | 通过 | 主观记忆 importance 88/recallCount 1；A/B belief；B emotion calm/intensity 10；relation score 10；workflow `b674e0af…4720` completed。 |
| H6 提示词与隔离 | 通过 | Prompt snapshot `bfd09a7c…36e2` 包含 recent_memory，provenance 指向 `capability_package/io.zerone.subjective-memory`；未绑定 Run `034d0d39…7d51` 的 bindings/states 均为 0，prompt 无 memory 注入。 |

## 新待办

- 模型在轮询期间可能重复向同一 Agent 发起咨询，产生额外消息分支。主链已正常收口，但额外分支可能仍在 `queued/running`；后续需增加“同一父任务 + 目标 + 动作”的重复派发抑制或明确的并行分支语义。
