import { setTokens } from '@/api/client'

/**
 * 从完整 href 消费 SSO 回落的 token/refreshToken：写入 localStorage 后返回
 * 仅删除这两个参数的 URL（保留其余 query 与 hash）；无 token 返回 null。
 *
 * 载体优先级：fragment（issue #185 起，token 不进网关日志）> query（旧格式
 * 兼容，滚动发布期间可能收到旧链接）。提取为纯函数供 App 模块加载与单测共用。
 */
export function consumeAuthParams(href: string): string | null {
  const hashIdx = href.indexOf('#')
  const beforeHash = hashIdx >= 0 ? href.slice(0, hashIdx) : href
  const hash = hashIdx >= 0 ? href.slice(hashIdx + 1) : ''
  const qIdx = beforeHash.indexOf('?')
  const base = qIdx >= 0 ? beforeHash.slice(0, qIdx) : beforeHash
  const query = new URLSearchParams(qIdx >= 0 ? beforeHash.slice(qIdx + 1) : '')
  const frag = new URLSearchParams(hash)

  const token = frag.get('token') ?? query.get('token')
  if (!token) return null
  setTokens(token, frag.get('refreshToken') ?? query.get('refreshToken') ?? undefined)

  query.delete('token')
  query.delete('refreshToken')
  // fragment 按 & 分段过滤（而非 URLSearchParams.toString()）：后者会把
  // 裸 hash（如 "#f"）重建成 "#f="，破坏业务 hash 原样性。
  const authSegment = /^(token|refreshToken)=/
  const cleanedHash = hash
    .split('&')
    .filter((segment) => segment !== '' && !authSegment.test(segment))
    .join('&')
  const restQuery = query.toString()
  return `${base}${restQuery ? `?${restQuery}` : ''}${cleanedHash ? `#${cleanedHash}` : ''}`
}
