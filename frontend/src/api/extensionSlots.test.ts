import { describe, it, expect } from 'vitest'
import { isSameOriginApiPath } from './extensionSlots'

describe('isSameOriginApiPath', () => {
  it('accepts the declared proxy prefix', () => {
    expect(isSameOriginApiPath('/api/v1/extensions/io.zerone.demo/stats')).toBe(true)
    expect(isSameOriginApiPath('/api/v1/extensions/io.zerone.demo/stats?days=7')).toBe(true)
  })

  it('rejects protocol-relative and absolute URLs', () => {
    // `//host/x` 会被浏览器当外站请求发出去（带 cookie 就是 CSRF/信息外泄）
    expect(isSameOriginApiPath('//evil.com/steal')).toBe(false)
    expect(isSameOriginApiPath('https://evil.com/x')).toBe(false)
    expect(isSameOriginApiPath('http://evil.com/x')).toBe(false)
    expect(isSameOriginApiPath('javascript:alert(1)')).toBe(false)
  })

  it('rejects paths outside the extension proxy prefix', () => {
    expect(isSameOriginApiPath('/api/v1/admin/extensions')).toBe(false)
    expect(isSameOriginApiPath('/api/v1/extensions')).toBe(false)
    expect(isSameOriginApiPath('api/v1/extensions/x')).toBe(false)
  })

  it('rejects backslashes, control characters and blank input', () => {
    expect(isSameOriginApiPath('/api/v1/extensions/\\evil')).toBe(false)
    expect(isSameOriginApiPath('/api/v1/extensions/x\nHost: evil')).toBe(false)
    expect(isSameOriginApiPath('')).toBe(false)
    expect(isSameOriginApiPath('   ')).toBe(false)
    expect(isSameOriginApiPath(undefined)).toBe(false)
    expect(isSameOriginApiPath(42)).toBe(false)
  })
})
