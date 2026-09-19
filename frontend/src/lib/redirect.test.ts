import { describe, expect, it } from 'vitest'
import { loginRedirectUrl, sanitizeRedirect } from './redirect'

describe('sanitizeRedirect', () => {
  it('合法路径原样返回（含 query/hash）', () => {
    expect(sanitizeRedirect('/agents/chat?x=1#f')).toBe('/agents/chat?x=1#f')
  })
  it('空/协议相对/绝对 URL/反斜杠/超长回退 /', () => {
    expect(sanitizeRedirect(null)).toBe('/')
    expect(sanitizeRedirect('//evil.com')).toBe('/')
    expect(sanitizeRedirect('https://evil.com')).toBe('/')
    expect(sanitizeRedirect('/a\\b')).toBe('/')
    expect(sanitizeRedirect(`/${'a'.repeat(600)}`)).toBe('/')
  })
})

describe('loginRedirectUrl', () => {
  it('去 basename 并保留 search/hash', () => {
    expect(loginRedirectUrl('/static/agents/chat', '?x=1', '#f')).toBe(
      `/static/login?redirect=${encodeURIComponent('/agents/chat?x=1#f')}`
    )
  })
  it('根路径与裸 /static 回退 /', () => {
    expect(loginRedirectUrl('/static', '', '')).toBe(`/static/login?redirect=${encodeURIComponent('/')}`)
    expect(loginRedirectUrl('/static/', '', '')).toBe(`/static/login?redirect=${encodeURIComponent('/')}`)
  })
})
