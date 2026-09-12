# 扩展清单离线校验

H0 提供 `agenthub.extension/v1alpha1` 的静态清单校验，不安装能力包，也不执行包内代码。

能力包根目录必须包含 `extension.yaml`，可在仓库根目录运行：

```bash
go run ./cmd/extension-validate /path/to/capability-package
```

校验器当前检查：

- `apiVersion`、`kind`、元数据、兼容范围、权限和贡献声明结构；
- 包名、反向域名命名空间和 SemVer 资源版本；
- 未在 v1alpha1 中声明的未知字段；
- 所有贡献文件必须使用安全的包内相对路径；
- 清单引用的文件必须存在且为普通文件。

规范 Schema 位于
`internal/extensionmanifest/schema/agenthub-extension-v1alpha1.schema.json`。有效与无效示例位于
`internal/extensionmanifest/testdata`。

本阶段不会验证贡献文件内部的状态、事件、工具或 UI Schema，也不会解析依赖版本范围、
检查权限是否超出命名空间，或校验数字签名。这些属于后续 registry/install 流程。
