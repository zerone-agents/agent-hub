/**
 * 已鉴权文件下载工具。
 *
 * 为什么不直接 `<a href="/api/...">` 或 `window.open(url)`：
 * 这两者都是**浏览器导航**，请求由浏览器自己发出，不会经过 axios 拦截器，
 * 因此不会带 `Authorization` 头 —— 对需要鉴权的下载端点必然 401。
 * `window.open` 还有第二个坑：被弹窗拦截时返回 `null` 而**不抛异常**，
 * 外层 `try/catch` 成了死代码，用户点了没反应也看不到任何报错。
 *
 * 正确姿势是：先用 axios / fetch 把内容取成 blob，再交给本模块落盘。
 * 参考既有实现 `features/chat/PartFile.tsx` 与 `features/agent-chat/CwdFilePreview.tsx`。
 */

/** 触发浏览器保存一个内存中的 blob，并立即释放 objectURL。 */
export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  // 必须挂到文档里：Firefox 对游离节点的 click() 不触发下载
  document.body.appendChild(a)
  try {
    a.click()
  } finally {
    // click() 同步触发下载，此处释放是安全的；放在 finally 里可保证
    // 即便 click 抛错也不会把 anchor 和 objectURL 泄漏出去。
    a.remove()
    URL.revokeObjectURL(url)
  }
}

/**
 * 从 `Content-Disposition` 解析服务端建议的文件名。
 *
 * 解析不出（或格式异常）时返回 `fallback`，绝不抛错 —— 文件名缺失不该阻断下载。
 * 优先 `filename*`（RFC 5987，可带 UTF-8 编码），退回 `filename`。
 */
export function filenameFromDisposition(disposition: unknown, fallback: string): string {
  if (typeof disposition !== 'string' || disposition.trim() === '') {
    return fallback
  }
  const encoded = /filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/.exec(disposition)
  if (encoded) {
    try {
      const decoded = decodeURIComponent(encoded[1].trim())
      if (decoded !== '') return decoded
    } catch {
      // 编码损坏时静默退到 filename / fallback
    }
  }
  const plain = /filename\s*=\s*"?([^";]+)"?/.exec(disposition)
  if (plain) {
    const name = plain[1].trim()
    if (name !== '') return name
  }
  return fallback
}
