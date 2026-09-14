# H7 决策记录

主 Agent 记录所有产品/架构决策，防止中断后重开讨论。

## D1：Manifest 版本定为 v1（在 v1alpha1 协议上产品化）

- 沿用 `agenthub.extension/v1alpha1` 协议标识，数据模型落为 `extension_manifests`（替代/扩展 capability_packages 语义）。
- 复用 `internal/extensionmanifest` 校验器，升级为严格校验（必填、命名规范、 semver、依赖范围、权限声明白名单）。

## D2：H6 四包作为验收包，Core 零 Speeding/情绪代码

- Core 只理解"经 Schema 校验的命名空间状态"；情绪/认知/记忆/关系全部留在扩展包。
- 验收时 H6 四包必须以扩展形式安装/停用/卸载，证明机制通用。

## D3：阶段推进顺序

H7.0 → H7.1 →（H7.2 / H7.3 / H7.4 并行）→ H7.5 → H7.6 → H7.7 审查部署。

## D4：公共文件唯一改动权归主 Agent

cmd/server/main.go、pkg/database/database.go、前端路由/导航由主 Agent 在集成时统一接线；子 Agent 只提供接线说明（需要的 provider/service/路由），不得自行修改。

（后续决策持续追加）
