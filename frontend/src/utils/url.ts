/**
 * 外部链接协议的收敛工具。
 *
 * 扩展 manifest 的 `ui.slots[].data` 在后端是自由 `map[string]any`，不做协议校验；
 * 前端若把 `href` 原样塞进 `<a href>`，`javascript:` / `data:` 就能注入执行
 * （点一下就等价于 XSS）。因此所有"扩展提供的链接"都必须先过本函数。
 */

/** 允许在 `<a href>` 中使用的协议白名单。 */
const ALLOWED_PROTOCOLS = new Set(['http:', 'https:', 'mailto:'])

/**
 * 校验一个扩展声明的 href，返回可安全放进 `<a href>` 的字符串；
 * 不通过校验时返回 `''`，调用方应退化为纯文本渲染。
 *
 * 放行：`https://…`、`http://…`、`mailto:…`，以及同源相对写法（`/x`、`#x`、`x/y`）。
 * 拒绝：`javascript:`、`data:`、`blob:`、`file:` 等一切白名单外协议，
 * 以及协议相对写法 `//host/x`（浏览器会当 http(s) 发往外站）。
 */
export function safeExternalHref(raw: unknown): string {
  if (typeof raw !== 'string') return ''
  const value = raw.trim()
  if (value === '') return ''
  // 控制字符（含 \t \n \r）会被 URL 解析器在内部剥离，可能拼出 `java\nscript:`，
  // 这类"看起来无害"的输入一律直接拒绝，不做归一化后再判定。
  // eslint-disable-next-line no-control-regex -- 这里就是要匹配控制字符本身
  if (/[\u0000-\u001f\u007f]/.test(value)) return ''
  // 反斜杠在部分浏览器里等价于 `/`，`\/\/host` 是常见的协议相对绕过写法
  if (value.includes('\\')) return ''
  // 协议相对：`//evil.com/x` 会被解析成当前页协议 + 外站主机
  if (value.startsWith('//')) return ''
  // 同源相对路径与锚点：交给浏览器按当前源解析，安全
  if (value.startsWith('/') || value.startsWith('#')) return value
  try {
    const parsed = new URL(value)
    return ALLOWED_PROTOCOLS.has(parsed.protocol) ? value : ''
  } catch {
    // 没有协议段的相对写法（`docs/a`）解析不出 URL —— 只接受不含冒号的
    // 形式，避免 `javascript:alert(1)` 这类"冒号藏在路径里"的漏网
    return value.includes(':') ? '' : value
  }
}
