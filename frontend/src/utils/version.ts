/**
 * 扩展 manifest 的版本号比较。
 *
 * 为什么不能用字符串比较：字典序下 `'1.10.0' < '1.9.0'` 为真，会把更高版本
 * 判成更低版本 —— 回滚候选列表因此会漏掉真正的低版本、混入更高的版本。
 *
 * 规则（semver 主干，够用且不抛错）：
 * - 数字段逐段按数值比较，段数不同时缺省补 0（`'1.2' == '1.2.0'`）；
 * - 带预发布后缀的一方更小（`'1.0.0-rc.1' < '1.0.0'`）；
 * - 无法解析成数字的段退回字典序兜底，保证结果稳定、绝不抛错。
 */
export function compareVersions(a: string, b: string): number {
  const pa = splitVersion(a)
  const pb = splitVersion(b)
  const len = Math.max(pa.core.length, pb.core.length)
  for (let i = 0; i < len; i += 1) {
    const cmp = compareSegment(pa.core[i], pb.core[i])
    if (cmp !== 0) return cmp
  }
  if (pa.pre !== '' && pb.pre === '') return -1
  if (pa.pre === '' && pb.pre !== '') return 1
  if (pa.pre === pb.pre) return 0
  return pa.pre < pb.pre ? -1 : 1
}

/** 版本 a 是否严格低于 b。 */
export function isVersionLower(a: string, b: string): boolean {
  return compareVersions(a, b) < 0
}

function splitVersion(raw: string | undefined): { core: string[]; pre: string } {
  const value = (raw ?? '').trim().replace(/^v/i, '')
  const dash = value.indexOf('-')
  if (dash < 0) return { core: value.split('.'), pre: '' }
  // 预发布后缀本身可能含 `-`（1.0.0-rc-1），按第一个 `-` 切分即可
  return { core: value.slice(0, dash).split('.'), pre: value.slice(dash + 1) }
}

function compareSegment(a: string | undefined, b: string | undefined): number {
  const na = toFiniteNumber(a)
  const nb = toFiniteNumber(b)
  if (na !== null && nb !== null) {
    return na === nb ? 0 : na < nb ? -1 : 1
  }
  const sa = a ?? ''
  const sb = b ?? ''
  if (sa === sb) return 0
  return sa < sb ? -1 : 1
}

function toFiniteNumber(segment: string | undefined): number | null {
  if (segment === undefined || segment.trim() === '') return 0
  const n = Number(segment)
  return Number.isFinite(n) ? n : null
}
