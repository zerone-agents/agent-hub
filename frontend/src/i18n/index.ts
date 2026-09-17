import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import zh from './locales/zh.json'
import en from './locales/en.json'

export type AppLanguage = 'zh' | 'en'
export const LANGUAGE_STORAGE_KEY = 'agent-hub.language'
export const DEFAULT_LANGUAGE: AppLanguage = 'zh'

/** 读持久化语言：仅认 'en' 为有效英文偏好，其余（含无效值/未设置）一律 zh。 */
export function readStoredLanguage(): AppLanguage {
  if (typeof window === 'undefined') return DEFAULT_LANGUAGE
  return window.localStorage.getItem(LANGUAGE_STORAGE_KEY) === 'en' ? 'en' : 'zh'
}

/**
 * 语言切换唯一入口，四同步：
 * 1. localStorage 持久化；2. <html lang>（无障碍/浏览器翻译抑制）；
 * 3. dayjs.locale（antd 6 日期文案跟随）；4. i18next.changeLanguage
 * （驱动全部 react-i18next 文案与 useLanguage 订阅者重渲染）。
 */
export function setAppLanguage(lang: AppLanguage): void {
  window.localStorage.setItem(LANGUAGE_STORAGE_KEY, lang)
  document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en'
  dayjs.locale(lang === 'zh' ? 'zh-cn' : 'en')
  void i18next.changeLanguage(lang)
}

// 模块加载即初始化（资源同步打包，无异步 backend）：首次渲染前语言已就绪，
// 避免首帧闪错语言。html lang / dayjs 与初始语言同源同步。
const initial = readStoredLanguage()
document.documentElement.lang = initial === 'zh' ? 'zh-CN' : 'en'
dayjs.locale(initial === 'zh' ? 'zh-cn' : 'en')

void i18next.use(initReactI18next).init({
  resources: {
    zh: { translation: zh },
    en: { translation: en }
  },
  lng: initial,
  fallbackLng: DEFAULT_LANGUAGE,
  interpolation: { escapeValue: false } // React 已做 XSS 转义
})

export default i18next
