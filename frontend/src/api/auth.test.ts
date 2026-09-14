import { describe, expect, it } from 'vitest'
import { buildLoginUrl } from './auth'

describe('buildLoginUrl', () => {
  it('无参裸登录端点', () => {
    expect(buildLoginUrl('', '')).toBe('/auth/login')
  })
  it('org + redirect 组装', () => {
    expect(buildLoginUrl('org-a', '/agents/chat')).toBe('/auth/login?org=org-a&redirect=%2Fagents%2Fchat')
  })
  it('redirect="/" 视为无回源', () => {
    expect(buildLoginUrl('', '/')).toBe('/auth/login')
  })
})
