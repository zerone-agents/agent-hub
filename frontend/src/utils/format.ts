/** 人类可读的字节数（B / KB / MB），附件卡片与托盘共用。 */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return ''
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

/**
 * 把一段 JSON 文本格式化成可展示的多行形式。
 *
 * 为什么不能直接在 render 里写 `JSON.stringify(JSON.parse(raw), null, 2)`：
 * manifest 是用户粘贴/上游注册进来的，一旦不是合法 JSON 就会在 render 体内
 * 抛错被根 ErrorBoundary 接住 → 整页白屏。这里始终返回字符串，解析失败就
 * 原样回显，把"内容坏了"变成用户看得见的文本而不是系统级崩溃。
 */
export function prettyJson(raw: unknown): string {
  if (typeof raw !== 'string') return ''
  const text = raw.trim()
  if (text === '') return ''
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return raw
  }
}

/**
 * 把 JSON 文本解析成对象；输入为空或解析失败时返回 `fallback`（默认
 * `undefined`），绝不抛错。
 *
 * 用于"内容来自用户粘贴 / 上游注册、但需要在 render 体内取用"的场景：
 * 裸 `JSON.parse` 抛错会被根 ErrorBoundary 接住 → 整页白屏。
 */
export function parseJsonSafe<T>(raw: unknown, fallback?: T): T | undefined {
  if (typeof raw !== 'string' || raw.trim() === '') return fallback
  try {
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}