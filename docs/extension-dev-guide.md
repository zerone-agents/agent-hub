# Agent Hub 扩展开发指南（H7.6）

面向扩展开发者：从 manifest 写法到 CLI 全流程、SDK 调用与安装失败的诊断手册。

- 协议背景：[platform-extension-protocol.md](platform-extension-protocol.md)
- 完整示例：[examples/extensions/hello-stats](../examples/extensions/hello-stats)（最小可用）、
  [examples/extensions/h6-persona-demo](../examples/extensions/h6-persona-demo)（H6 参考实现）
- Go SDK：[sdk/README.md](../sdk/README.md)

## 1. 快速开始

```bash
# 生成扩展骨架（manifest 模板已通过严格校验，可直接改）
zerone extension create io.zerone.hello --dir ./extensions

# 本地校验
zerone extension validate ./extensions/io.zerone.hello

# 开发模式：保存即自动重新校验并推送到 registry
zerone --server http://localhost:8080 --token cli_xxx extension dev ./extensions/io.zerone.hello

# 打包 + 签名
zerone extension pack ./extensions/io.zerone.hello --sign --key ./ed25519.key
```

## 2. manifest 全字段说明（严格 v1）

扩展根目录必须有 `extension.yaml`，内容为严格 v1 manifest 的 YAML 排版
（字段与注册 API 的 JSON 完全一致，CLI 负责 YAML→规范 JSON 转换）。

| 字段 | 必填 | 说明 |
|---|---|---|
| `apiVersion` | 是 | 固定 `agenthub.extension/v1alpha1` |
| `name` | 是 | DNS 式命名，小写字母开头，至少两段点分（如 `io.zerone.hello`），租户内唯一 |
| `version` | 是 | 语义化版本 `MAJOR.MINOR.PATCH`（可选 `-prerelease`、`+build`） |
| `displayName` | 是 | 展示名，≤160 字符 |
| `description` | 否 | ≤2000 字符 |
| `icon` / `publisher` | 否 | 图标 URL / 发布者 |
| `dependencies` | 否 | `[{name, version, optional?}]`，version 为范围表达式（见 §5） |
| `permissions` | 否 | `[{permission, scope, actions[]}]`，见 §4 |
| `ui.slots` | 否 | 七类白名单插槽，见 §3 |
| `stateSchemas` / `events` / `tools` / `relations` / `promptInjections` | 否 | 声明类贡献，条目为 `{name, description?, payload?}`，见 §3 |
| `migrations` | 否 | `[{from, to, ops[], rollback?}]`，见 §6 |

命名规范：声明条目 `name` 小写字母开头，可含 `.`、`_`、`-` 与数字，同组内不可重复。

## 3. 声明式 UI 组件

`ui.slots` 声明扩展占用哪些插槽（H7.2 渲染），白名单七类：

| 插槽 | 位置 |
|---|---|
| `sidebar` | 侧边栏 |
| `dashboard.card` | 仪表盘卡片 |
| `agent.detail.tab` | Agent 详情页标签 |
| `run.detail.tab` | 运行详情页标签 |
| `group.detail.tab` | 群组详情页标签 |
| `workflow.detail.tab` | 工作流详情页标签 |
| `settings.section` | 设置区 |

声明类贡献的四种典型形态（`payload` 为自由 JSON，由对应子系统消费）：

1. **状态 Schema**（`stateSchemas`）：`payload` 为 JSON Schema 2020-12，
   平台据此校验本扩展命名空间的 run_states 数据。
2. **事件词表**（`events`）：`payload` 携带 delta/severity 等结算参数。
3. **工具/路由**（`tools`）：`payload` 声明 `method`/`path`/`auth`/`effects`
   （只读端点 `effects: read`）。
4. **提示词注入**（`promptInjections`）：`payload` 声明 `stage`/`audience`/
   `onMissing`/`maxTokens`/`template`——只能影响表达，**不能授予权限**。

## 4. permissions 白名单

`permission` 必须是以下十一类之一（未声明即拒绝）：

```
agent  state  message  model  tool  event  network  storage  ui  group  workflow
```

- `scope`：资源命名空间（如 `io.zerone.hello/*`），必填 ≤160 字符。
- `actions`：允许的操作集合，至少一个，每项 ≤64 字符，不可重复。

## 5. 版本与依赖范围

- `version`：semver；发布后内容不可变（同 name+version 必须同内容哈希）。
- 依赖 `version` 支持：精确版本、`^1.2.3`（锁主版本）、`~1.2.3`（锁次版本）、
  比较符组合 `">=1.0.0 <2.0.0"`。

## 6. migrations 写法

JSON Patch 子集（`replace` / `add` / `remove`），升级正向执行 `ops`，
回滚优先用 `rollback`，否则按旧值推导逆操作：

```yaml
migrations:
  - from: 1.0.0
    to: 2.0.0
    ops:
      - op: replace
        path: /minimum
        value: 0
      - op: remove
        path: /deprecated
    rollback:
      - op: remove
        path: /minimum
```

约束：`path` 必须 `/` 开头的 JSON Pointer；`remove` 不得携带 `value`；
同一条迁移内路径不可重复；`from`/`to` 版本区间不可重复。

## 7. CLI 全流程

| 命令 | 说明 |
|---|---|
| `zerone extension create <name> --dir DIR` | 生成骨架；模板即通过严格校验 |
| `zerone extension validate <dir\|file>` | 本地严格校验，中文错误（YAML 错误带行号） |
| `zerone extension dev <dir>` | 轮询目录（400ms）+ 防抖 1s，校验通过后 POST 到 `--server`；失败打印中文错误、不重试风暴；Ctrl-C 退出 |
| `zerone extension pack <dir> [--sign] [--key 文件]` | 产物 `{name}-{version}.tgz` + `.sha256` + `.sig`（`ZERONE_SIGN_KEY` 可代替 `--key`） |
| `zerone extension migrate-manifest <old.yaml>` | 旧宽松格式 → 严格 v1，输出新文件与逐条变更说明 |

全局 flag：`--server`（默认 `http://localhost:8080` / `ZERONE_SERVER`）、
`--token`（`ZERONE_TOKEN`）、`--tenant`、`--output`。

签名：`--sign` 用 ed25519 私钥（base64/hex 的 64 字节私钥或 32 字节种子）
对**规范 manifest JSON**（键排序）签名。注册时把 `.sig` 中的
`signature`/`public_key` 一并提交，服务端验签通过后公钥指纹
（sha256 前 16 字节 hex）记入 `extension_versions.signed_by`。

## 8. SDK 示例

```go
client := sdk.NewExtensionAdminClient("http://localhost:8080", token)
client.Register(sdk.RegisterInput{Manifest: manifestJSON, Signature: sig, PublicKey: pub})
client.Install(id, "1.0.0")
client.Enable(id)
client.Impact(id)          // 升级/卸载前必查
client.Upgrade(id, "2.0.0")
client.Rollback(id, "")    // 回滚到上一版本
client.Uninstall(id, false, false)
```

错误处理：`errors.Is(err, sdk.ErrNotFound/ErrConflict/ErrValidation/...)`，
中文错误原文在 `apiErr.Message`。更多示例见 `sdk/example_test.go`。

## 9. 常见问题：安装失败诊断

### 9.1 manifest 校验失败（HTTP 400）

错误长相：`扩展 manifest 校验失败：name "xxx" 不符合 DNS 式命名；permissions[0].permission "emotion" 不在白名单内...`

排查步骤：
1. `zerone extension validate <dir>` 本地复现，逐条修中文错误；
2. 常见错：name 用了大写/单段、version 非三段 semver、权限类别用了复数
   （旧格式习惯）或不在白名单、actions 为空、插槽名拼错。

### 9.2 依赖缺失 / 冲突（HTTP 409）

错误长相：`扩展 io.zerone.b 依赖 io.zerone.a >=2.0.0 <3.0.0，未安装或版本不满足（当前 1.4.0）`

排查步骤：
1. 先注册并安装被依赖扩展：`zerone extension dev` 推送依赖方，或
   `client.List` 确认目标版本存在；
2. 用 `client.Impact(id)` 看依赖图——谁依赖它、它依赖谁；
3. 版本范围写宽一点（`^1.0.0`）还是收窄（`>=2.0.0 <3.0.0`）按兼容性决定。

### 9.3 签名校验失败（HTTP 400）

错误长相：`签名校验失败：签名与 manifest 内容不匹配`

排查步骤：
1. 确认签名对象是**规范 JSON**（用 `zerone extension pack --sign` 生成，
   不要手动对 YAML 或带排版的 JSON 签名）；
2. `signature` 与 `public_key` 必须**同时**提交，缺一即拒；
3. 编码只支持 base64 / hex；公钥必须是 32 字节 ed25519；
4. manifest 任何字段改动都会使签名失效——改内容必须重新 `pack --sign`。

### 9.4 其他

- **401/403**：token 无效或角色不是 admin/maintainer。
- **重复注册返回 201 且 alreadyExisted=true**：同内容哈希幂等命中，属正常。
- **dev 模式推送被拒**：先按 9.1–9.3 的错误长相对号入座；网络错误
  （连接拒绝/超时）会打印但不重试，检查 `--server` 后手动触发一次改动即可。
