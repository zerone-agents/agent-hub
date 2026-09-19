import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useLanguage } from './useLanguage'
import { setAppLanguage } from '@/i18n'

describe('useLanguage', () => {
  it('初始语言跟随 i18next 实例', () => {
    const { result } = renderHook(() => useLanguage())
    expect(['zh', 'en']).toContain(result.current.language)
  })

  it('setAppLanguage 触发订阅者重渲染', () => {
    const { result } = renderHook(() => useLanguage())
    act(() => {
      setAppLanguage('en')
    })
    expect(result.current.language).toBe('en')
    act(() => {
      setAppLanguage('zh')
    })
    expect(result.current.language).toBe('zh')
  })

  it('hook 的 setLanguage 与 setAppLanguage 等效', () => {
    const { result } = renderHook(() => useLanguage())
    act(() => {
      result.current.setLanguage('en')
    })
    expect(result.current.language).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    act(() => {
      result.current.setLanguage('zh')
    })
    expect(result.current.language).toBe('zh')
  })
})
