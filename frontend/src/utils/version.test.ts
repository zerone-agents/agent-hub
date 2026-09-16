import { describe, it, expect } from 'vitest'
import { compareVersions, isVersionLower } from './version'

describe('compareVersions', () => {
  it('compares numeric segments by value, not lexicographically', () => {
    // 这条是 P2-18 的核心回归点：字符串比较会把 1.10.0 判成小于 1.9.0
    expect(compareVersions('1.10.0', '1.9.0')).toBe(1)
    expect(isVersionLower('1.10.0', '1.9.0')).toBe(false)
    expect(isVersionLower('1.9.0', '1.10.0')).toBe(true)
    expect(compareVersions('2.0.0', '10.0.0')).toBe(-1)
  })

  it('treats missing trailing segments as zero', () => {
    expect(compareVersions('1.2', '1.2.0')).toBe(0)
    expect(compareVersions('1', '1.0.1')).toBe(-1)
    expect(compareVersions('1.0', '1')).toBe(0)
  })

  it('orders pre-release below the release', () => {
    expect(compareVersions('1.0.0-rc.1', '1.0.0')).toBe(-1)
    expect(compareVersions('1.0.0', '1.0.0-rc.1')).toBe(1)
    expect(compareVersions('1.0.0-rc.1', '1.0.0-rc.1')).toBe(0)
    expect(compareVersions('1.0.0-alpha', '1.0.0-beta')).toBe(-1)
  })

  it('ignores a leading v prefix', () => {
    expect(compareVersions('v1.2.0', '1.2.0')).toBe(0)
  })

  it('is total and never throws on malformed input', () => {
    expect(() => compareVersions('', '')).not.toThrow()
    expect(compareVersions('', '1.0.0')).toBe(-1)
    expect(compareVersions('abc', 'abc')).toBe(0)
    expect(compareVersions('1.x', '1.y')).toBe(-1)
    expect(compareVersions('1.0.0', '1.0.0')).toBe(0)
  })
})

describe('isVersionLower (回滚候选过滤)', () => {
  it('keeps only versions strictly below the installed one', () => {
    const installed = '1.10.0'
    const candidates = ['1.2.0', '1.9.0', '1.10.0', '1.11.0', '2.0.0']
    expect(candidates.filter((v) => isVersionLower(v, installed))).toEqual(['1.2.0', '1.9.0'])
  })
})
