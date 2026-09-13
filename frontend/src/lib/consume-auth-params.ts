import { setTokens } from '@/api/client'

/**
 * 从完整 href 消费 SSO 回落的 token/refreshToken：写入 localStorage 后返回
 * 仅删除这两个参数的 URL（保留其余 query 与 hash）；无 token 返回 null。
 * 提取为纯函数供 App 模块加载与单测共用（原内联逻辑会清掉整条 query）。
 */
export function consumeAuthParams(href: string): string | null {
  const hashIdx = href.indexOf('#')
  const beforeHash = hashIdx >= 0 ? href.slice(0, hashIdx) : href
  const hash = hashIdx >= 0 ? href.slice(hashIdx) : ''
  const qIdx = beforeHash.indexOf('?')
  const base = qIdx >= 0 ? beforeHash.slice(0, qIdx) : beforeHash
  const params = new URLSearchParams(qIdx >= 0 ? beforeHash.slice(qIdx + 1) : '')
  const token = params.get('token')
  if (!token) return null
  setTokens(token, params.get('refreshToken') ?? undefined)
  params.delete('token')
  params.delete('refreshToken')
  const rest = params.toString()
  return `${base}${rest ? `?${rest}` : ''}${hash}`
}
