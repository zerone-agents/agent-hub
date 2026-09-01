import { afterEach, describe, expect, it, vi } from 'vitest'

import { copyToClipboard, copyOrManual } from './clipboard'

/**
 * jsdom does not implement window.isSecureContext (undefined) and does not
 * expose navigator.clipboard — the default state is therefore exactly the
 * insecure-context scenario these tests target. Stubs are installed as own,
 * configurable properties and deleted in afterEach to restore that state.
 */
describe('copyToClipboard', () => {
  afterEach(() => {
    delete (navigator as unknown as Record<string, unknown>).clipboard
    delete (window as unknown as Record<string, unknown>).isSecureContext
    delete (document as unknown as Record<string, unknown>).execCommand
  })

  it('uses the async Clipboard API in secure contexts', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    const exec = vi.fn()
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })

    expect(await copyToClipboard('hello')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(exec).not.toHaveBeenCalled() // secure path must not touch the fallback
  })

  it('falls back to execCommand in insecure contexts (plain HTTP + IP)', async () => {
    const exec = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })

    expect(await copyToClipboard('fallback-text')).toBe(true)
    expect(exec).toHaveBeenCalledWith('copy')
    // the scratch textarea must be removed after copying
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('falls back to execCommand when the Clipboard API rejects', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    const exec = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })

    expect(await copyToClipboard('retry')).toBe(true)
    expect(exec).toHaveBeenCalledWith('copy')
  })

  it('returns false when the fallback reports failure', async () => {
    const exec = vi.fn().mockReturnValue(false)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })

    expect(await copyToClipboard('x')).toBe(false)
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('returns false when no copy path exists at all', async () => {
    // jsdom: execCommand unimplemented — an undefined own property throws on call
    Object.defineProperty(document, 'execCommand', { value: undefined, configurable: true })

    expect(await copyToClipboard('x')).toBe(false)
    expect(document.querySelector('textarea')).toBeNull()
  })
})

describe('copyOrManual', () => {
  afterEach(() => {
    delete (navigator as unknown as Record<string, unknown>).clipboard
    delete (window as unknown as Record<string, unknown>).isSecureContext
    delete (document as unknown as Record<string, unknown>).execCommand
    document.body.innerHTML = ''
  })

  it('secure context：走 Clipboard API 且不弹手动复制框', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })

    expect(await copyOrManual('https://example.com/x')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('https://example.com/x')
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('insecure context：尽力复制并弹出手动复制框（全文只读 + 用户自选）', async () => {
    const exec = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })
    // jsdom 默认无 window.isSecureContext → 自动走非安全分支

    const text = 'http://127.0.0.1/runtime'
    expect(await copyOrManual(text)).toBe(true)
    expect(exec).toHaveBeenCalledWith('copy') // 尽力路径照常尝试

    await vi.waitFor(() => {
      expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    })
    const ta = document.querySelector('textarea')
    expect(ta).not.toBeNull()
    expect(ta?.value).toBe(text)
    expect(ta?.readOnly).toBe(true)
  })
})
