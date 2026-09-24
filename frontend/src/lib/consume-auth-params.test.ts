import { afterEach, describe, expect, it } from 'vitest'
import { consumeAuthParams } from './consume-auth-params'

afterEach(() => { localStorage.clear() })

describe('consumeAuthParams', () => {
  it('无 token 返回 null 且不写 localStorage', () => {
    expect(consumeAuthParams('http://x/static/agents/chat?x=1')).toBeNull()
    expect(localStorage.getItem('access_token')).toBeNull()
  })
  it('消费 token 并仅删认证参数、保留其余 query 与 hash', () => {
    const out = consumeAuthParams('http://x/static/agents/chat?x=1&token=t1&refreshToken=r1#f')
    expect(out).toBe('http://x/static/agents/chat?x=1#f')
    expect(localStorage.getItem('access_token')).toBe('t1')
    expect(localStorage.getItem('refresh_token')).toBe('r1')
  })
  it('从 fragment 消费 token 并删除（issue #185 新载体）', () => {
    const out = consumeAuthParams('http://x/static/agents/chat?x=1#token=t1&refreshToken=r1')
    expect(out).toBe('http://x/static/agents/chat?x=1')
    expect(localStorage.getItem('access_token')).toBe('t1')
    expect(localStorage.getItem('refresh_token')).toBe('r1')
  })
  it('fragment 中 token 与业务 hash 共存时保留业务 hash', () => {
    const out = consumeAuthParams('http://x/static/agents/chat#f&token=t1&refreshToken=r1')
    expect(out).toBe('http://x/static/agents/chat#f')
    expect(localStorage.getItem('access_token')).toBe('t1')
  })
})
