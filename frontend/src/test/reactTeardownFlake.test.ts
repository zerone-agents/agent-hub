import { afterEach, describe, expect, it, vi } from 'vitest'
import { isReactTeardownFlake } from './reactTeardownFlake'

describe('isReactTeardownFlake', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns true for ReferenceError("window is not defined") with window torn down', () => {
    vi.stubGlobal('window', undefined)
    expect(isReactTeardownFlake(new ReferenceError('window is not defined'))).toBe(true)
  })

  it('returns false for a different message ("document is not defined")', () => {
    vi.stubGlobal('window', undefined)
    expect(isReactTeardownFlake(new ReferenceError('document is not defined'))).toBe(false)
  })

  it('returns false for a TypeError with the same message', () => {
    vi.stubGlobal('window', undefined)
    expect(isReactTeardownFlake(new TypeError('window is not defined'))).toBe(false)
  })

  it('returns false for a plain Error object and for a string', () => {
    vi.stubGlobal('window', undefined)
    expect(isReactTeardownFlake(new Error('window is not defined'))).toBe(false)
    expect(isReactTeardownFlake('window is not defined')).toBe(false)
  })

  it('returns false while window still exists (normal environment — never swallow real errors)', () => {
    vi.stubGlobal('window', {})
    expect(isReactTeardownFlake(new ReferenceError('window is not defined'))).toBe(false)
  })
})
