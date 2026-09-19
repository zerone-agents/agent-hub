import { describe, expect, it } from 'vitest'
import { isGuestUser } from './auth-guest'

describe('isGuestUser', () => {
  it('显式 guest 角色任意 mode 均为 guest', () => {
    expect(isGuestUser({ role: 'guest' }, 'builtin')).toBe(true)
    expect(isGuestUser({ role: 'guest' }, 'casdoor')).toBe(true)
  })
  it('casdoor 空 roles 为隐式 guest', () => {
    expect(isGuestUser({ role: undefined }, 'casdoor')).toBe(true)
  })
  it('正式角色 / builtin 空 roles / 无用户不是 guest', () => {
    expect(isGuestUser({ role: 'member' }, 'casdoor')).toBe(false)
    expect(isGuestUser({ role: undefined }, 'builtin')).toBe(false)
    expect(isGuestUser(null, 'casdoor')).toBe(false)
  })
})
