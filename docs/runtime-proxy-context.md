# Runtime 运行请求正文边界

Go runtime 代理的 `POST /runtime/.../v1/agents/{id}/runs` 在转发前检查正文：

- 普通运行请求正文最多 **16 MiB**，已知 Content-Length 超界时不读取正文；未知长度或 chunked 请求最多读取上限加一字节，超界返回 413。
- 含 `turnContext` 或 `appendSystemPrompt` 的运行请求正文最多 **1 MiB**；临时字符串最多 6000 个 Unicode code point，禁止同时提供两个字段，类型错误返回 400。
- 正文读取总期限为 **15 秒**，读取超时返回 408。该期限在读取完成后清除，不缩短随后生成或 SSE 的期限。
- 1～16 MiB 的普通正文暂存于 0600 文件描述符，写入前立即 unlink，并在成功、拒绝和读取失败时关闭；每个请求最多占用 16 MiB 临时磁盘空间。

只有使用临时字段时，代理才探测目标 runtime 的 `capabilities.turnContext`。旧 runtime 缺少声明、探测失败或鉴权拒绝时返回 503，不向 run 端点转发。普通无临时字段请求保持既有上游鉴权和正文内容。

此门覆盖 Go runtime 代理。Kong 直通模式使用独立链路，Java 仍须探测实际目标 runtime 并完成相应版本验收。
