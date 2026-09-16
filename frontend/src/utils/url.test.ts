import { describe, it, expect } from 'vitest'
import { safeExternalHref } from './url'

describe('safeExternalHref', () => {
  it('allows http / https / mailto', () => {
    expect(safeExternalHref('https://example.com/a?b=1')).toBe('https://example.com/a?b=1')
    expect(safeExternalHref('http://example.com')).toBe('http://example.com')
    expect(safeExternalHref('mailto:ops@example.com')).toBe('mailto:ops@example.com')
  })

  it('allows same-origin relative paths and anchors', () => {
    expect(safeExternalHref('/extensions/3')).toBe('/extensions/3')
    expect(safeExternalHref('#section')).toBe('#section')
    expect(safeExternalHref('docs/a.md')).toBe('docs/a.md')
  })

  it('rejects javascript: and friends', () => {
    // 这四种都是前端把扩展 href 原样塞进 <a href> 时的真实 XSS 面
    expect(safeExternalHref('javascript:alert(1)')).toBe('')
    expect(safeExternalHref('JavaScript:alert(1)')).toBe('')
    expect(safeExternalHref('java\nscript:alert(1)')).toBe('')
    expect(safeExternalHref('  javascript:alert(1)  ')).toBe('')
    expect(safeExternalHref('data:text/html,<script>alert(1)</script>')).toBe('')
    expect(safeExternalHref('blob:https://example.com/x')).toBe('')
    expect(safeExternalHref('file:///etc/passwd')).toBe('')
  })

  it('rejects protocol-relative and backslash forms', () => {
    expect(safeExternalHref('//evil.com/steal')).toBe('')
    expect(safeExternalHref('\\/\\/evil.com/steal')).toBe('')
    expect(safeExternalHref('/\\evil.com')).toBe('')
  })

  it('rejects non-string and blank input', () => {
    expect(safeExternalHref(undefined)).toBe('')
    expect(safeExternalHref(null)).toBe('')
    expect(safeExternalHref(42)).toBe('')
    expect(safeExternalHref('   ')).toBe('')
  })
})
