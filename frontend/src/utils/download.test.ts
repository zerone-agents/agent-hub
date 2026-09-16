import { describe, it, expect, vi, afterEach } from 'vitest'
import { filenameFromDisposition, saveBlob } from './download'

describe('filenameFromDisposition', () => {
  it('returns fallback for missing / non-string headers', () => {
    expect(filenameFromDisposition(undefined, 'fallback.csv')).toBe('fallback.csv')
    expect(filenameFromDisposition(null, 'fallback.csv')).toBe('fallback.csv')
    expect(filenameFromDisposition(123, 'fallback.csv')).toBe('fallback.csv')
    expect(filenameFromDisposition('   ', 'fallback.csv')).toBe('fallback.csv')
  })

  it('parses a plain quoted filename', () => {
    expect(filenameFromDisposition('attachment; filename="usage_records.csv"', 'f')).toBe(
      'usage_records.csv'
    )
  })

  it('parses an unquoted filename', () => {
    expect(filenameFromDisposition('attachment; filename=usage_records.csv', 'f')).toBe(
      'usage_records.csv'
    )
  })

  it('prefers RFC 5987 filename* and decodes UTF-8', () => {
    const header = "attachment; filename=\"usage.csv\"; filename*=UTF-8''%E7%94%A8%E9%87%8F.csv"
    expect(filenameFromDisposition(header, 'f')).toBe('用量.csv')
  })

  it('falls back to filename when filename* cannot be decoded', () => {
    // `%E7%94` 是不完整的 UTF-8 序列，decodeURIComponent 会抛 URIError
    const header = "attachment; filename=\"usage.csv\"; filename*=UTF-8''%E7%94"
    // 解码异常不能冒泡，退到 filename
    expect(filenameFromDisposition(header, 'f')).toBe('usage.csv')
  })
})

describe('saveBlob', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('creates an object URL, clicks a download anchor, then revokes', () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })

    const clicked: HTMLAnchorElement[] = []
    const originalClick = HTMLAnchorElement.prototype.click
    HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
      clicked.push(this)
    }

    try {
      saveBlob(new Blob(['a,b\n1,2'], { type: 'text/csv' }), 'usage_records.csv')
    } finally {
      HTMLAnchorElement.prototype.click = originalClick
      vi.unstubAllGlobals()
    }

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
    expect(clicked).toHaveLength(1)
    expect(clicked[0].download).toBe('usage_records.csv')
    // 关键的回归点：anchor 不能留在 DOM 里
    expect(document.querySelectorAll('a')).toHaveLength(0)
  })

  it('cleans up anchor and object URL even when the click handler throws', () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })

    const originalClick = HTMLAnchorElement.prototype.click
    HTMLAnchorElement.prototype.click = function () {
      throw new Error('click exploded')
    }

    try {
      expect(() => {
        saveBlob(new Blob(['x']), 'x.csv')
      }).toThrow('click exploded')
    } finally {
      HTMLAnchorElement.prototype.click = originalClick
      vi.unstubAllGlobals()
    }

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
    expect(document.querySelectorAll('a')).toHaveLength(0)
  })
})
