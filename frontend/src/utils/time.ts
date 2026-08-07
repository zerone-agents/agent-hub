/**
 * Format an ISO timestamp as a human-friendly relative string.
 * Examples: "刚刚", "5 分钟前", "3 小时前", "2 天前", fallback to YYYY-MM-DD.
 */
export function formatTime(input: string | number | Date | undefined | null): string {
  if (input == null) return ''
  const date = new Date(input)
  if (Number.isNaN(date.getTime())) return ''

  const now = Date.now()
  const diff = now - date.getTime()
  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (seconds < 60) return '刚刚'
  if (minutes < 60) return `${minutes} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  if (days < 30) return `${days} 天前`

  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}
