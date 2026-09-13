/** 校验回源路径：单 / 开头、拒协议相对/绝对 URL/反斜杠/超长（>512），非法回退 /。 */
export function sanitizeRedirect(value: string | null | undefined): string {
  if (!value) return '/'
  if (!value.startsWith('/') || value.startsWith('//') || value.includes('\\') || value.length > 512) {
    return '/'
  }
  return value
}

/** 401 整页跳登录：记录去 /static basename 的来源（含 query/hash）。 */
export function loginRedirectUrl(pathname: string, search: string, hash: string): string {
  let route = pathname.replace(/^\/static(?=\/|$)/, '') + search + hash
  if (!route) route = '/'
  return `/static/login?redirect=${encodeURIComponent(route)}`
}
