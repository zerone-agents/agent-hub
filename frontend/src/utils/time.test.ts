import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { formatTime } from './time'

describe('formatTime', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-17T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns 刚刚 for < 60 seconds ago', () => {
    expect(formatTime('2026-06-17T11:59:45Z')).toBe('刚刚')
  })

  it('returns X 分钟前 for minutes ago', () => {
    expect(formatTime('2026-06-17T11:55:00Z')).toBe('5 分钟前')
  })

  it('returns X 小时前 for hours ago', () => {
    expect(formatTime('2026-06-17T09:00:00Z')).toBe('3 小时前')
  })

  it('returns X 天前 for days ago', () => {
    expect(formatTime('2026-06-15T09:00:00Z')).toBe('2 天前')
  })

  it('falls back to YYYY-MM-DD for > 30 days', () => {
    expect(formatTime('2026-05-01T09:00:00Z')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('returns empty string for invalid input', () => {
    expect(formatTime('not-a-date')).toBe('')
  })
})
