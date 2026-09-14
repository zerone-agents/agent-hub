# hello-stats —— 最小扩展示例

声明式扩展：dashboard.card 统计卡片 + 只读统计路由 + 状态 Schema + ui:read 权限。
不含任何 Go 代码。

## 全流程（create → validate → dev → pack → 安装）

```bash
# 1. 生成骨架（本目录即 create 的产物形态，可直接用）
zerone extension create io.zerone.hello-stats --dir ./examples/extensions

# 2. 本地严格校验
zerone extension validate ./examples/extensions/hello-stats

# 3. 开发模式：改动 extension.yaml 后自动重新校验并推送到 registry
zerone --server http://localhost:8080 --token cli_xxx extension dev ./examples/extensions/hello-stats

# 4. 打包（+ ed25519 签名）
zerone extension pack ./examples/extensions/hello-stats
zerone extension pack ./examples/extensions/hello-stats --sign --key ./ed25519.key
# 产物：io.zerone.hello-stats-0.1.0.tgz / .sha256 / .sig

# 5. 上传安装（SDK 示例）
```

```go
client := sdk.NewExtensionAdminClient("http://localhost:8080", token)
// 注册（签名包把 .sig 里的 signature/public_key 一并提交）
data, _ := client.Register(sdk.RegisterInput{Manifest: manifestJSON})
// 找到扩展 id 后安装并启用
id := parseID(data)
client.Install(id, "0.1.0")
client.Enable(id)
```

安装后：dashboard 出现 Hello Stats 卡片；`GET /api/v1/extensions/hello-stats/stats`
返回 `{"runs":0,"messages":0}` 聚合统计。
