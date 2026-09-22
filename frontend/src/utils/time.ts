import i18next from '@/i18n'

/**
 * Format an ISO timestamp as a human-friendly relative string.
 * Past: "刚刚", "5 分钟前", "3 小时前", "2 天前"; future: "5 分钟后" etc.;
 * beyond 30 days in either direction falls back to YYYY-MM-DD.
 */
export function formatTime(input: string | number | Date | undefined | null): string {
  if (input == null) return ''
  const date = new Date(input)
  if (Number.isNaN(date.getTime())) return ''

  const now = Date.now()
  const diff = date.getTime() - now
  const seconds = Math.floor(Math.abs(diff) / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (seconds < 60) return i18next.t('time.justNow')
  if (minutes < 60) return i18next.t(diff > 0 ? 'time.minutesLater' : 'time.minutesAgo', { n: minutes })
  if (hours < 24) return i18next.t(diff > 0 ? 'time.hoursLater' : 'time.hoursAgo', { n: hours })
  if (days < 30) return i18next.t(diff > 0 ? 'time.daysLater' : 'time.daysAgo', { n: days })

  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}
