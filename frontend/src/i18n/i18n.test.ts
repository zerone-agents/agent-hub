import { beforeEach, describe, expect, it } from 'vitest'
import dayjs from 'dayjs'
import {
  LANGUAGE_STORAGE_KEY,
  readStoredLanguage,
  setAppLanguage
} from './index'

describe('i18n 初始化与语言切换', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.lang = ''
  })

  it('无存储值时默认 zh', () => {
    expect(readStoredLanguage()).toBe('zh')
  })

  it('存储值 en 时读出 en；无效值回退 zh', () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'en')
    expect(readStoredLanguage()).toBe('en')
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'fr')
    expect(readStoredLanguage()).toBe('zh')
  })

  it('setAppLanguage 四同步：localStorage + html lang + dayjs + i18next', () => {
    setAppLanguage('en')
    // changeLanguage 内部语言事件在同步段已触发（i18next 同步 emit）
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    expect(dayjs.locale()).toBe('en')
    setAppLanguage('zh')
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('zh')
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(dayjs.locale()).toBe('zh-cn')
  })
})
