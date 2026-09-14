# Zerone Extension SDK（Go）

Agent Hub H7 扩展管理 API 的 Go 客户端。零依赖（仅标准库），中文错误透传。

## 快速开始

```go
client := sdk.NewExtensionAdminClient("http://localhost:8080", os.Getenv("ZERONE_TOKEN"))
client.Tenant = "acme" // 可选，经 X-Tenant-Id 透传

// 注册（manifest 为严格 v1 JSON）
data, err := client.Register(sdk.RegisterInput{
    Manifest: json.RawMessage(manifestJSON),
    Source:   "upload",
})

// 安装 → 启用
data, err = client.Install(1, "0.1.0")
data, err = client.Enable(1)

// 升级前先看影响面
data, err = client.Impact(1)
data, err = client.Upgrade(1, "2.0.0")

// 出问题就回滚 / 卸载
data, err = client.Rollback(1, "")          // 回滚到上一版本
data, err = client.Uninstall(1, false, false)
```

## 签名注册

```go
data, err := client.Register(sdk.RegisterInput{
    Manifest:  json.RawMessage(manifestJSON),
    Signature: base64Sig,  // ed25519，对规范 manifest JSON 签名
    PublicKey: base64Pub,  // base64/hex 均可
})
// 验签通过才会注册；公钥指纹（sha256 前 16 字节 hex）记入 version.signedBy
// 验签失败：*sdk.APIError，StatusCode=400，Message 为服务端中文错误
```

## 错误处理

服务端统一包络 `{success:false, error, code}`；HTTP 状态码映射为类型化错误：

| 状态码 | 预定义错误 | 典型场景 |
|---|---|---|
| 400 | `sdk.ErrValidation` | manifest 校验失败 / 签名校验失败 |
| 401 | `sdk.ErrUnauthorized` | token 无效 |
| 403 | `sdk.ErrPermission` | 非管理员 |
| 404 | `sdk.ErrNotFound` | 扩展/版本不存在 |
| 409 | `sdk.ErrConflict` | 已安装其他版本（走升级） |

```go
var apiErr *sdk.APIError
if errors.As(err, &apiErr) && errors.Is(err, sdk.ErrConflict) {
    log.Println("冲突：", apiErr.Message) // 中文原文
}
```

## 方法一览

Register / List / Get / GetVersion / Install / Enable / Disable /
Upgrade / Rollback / Uninstall / Impact。

更多可运行示例见 [example_test.go](example_test.go)（`go test ./sdk/ -v`）。
