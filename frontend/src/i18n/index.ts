import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import zh from './locales/zh.json'
import en from './locales/en.json'

export type AppLanguage = 'zh' | 'en'
export const LANGUAGE_STORAGE_KEY = 'agent-hub.language'
export const DEFAULT_LANGUAGE: AppLanguage = 'zh'

/**
 * 语言归一化（唯一映射来源）：仅 'en' 为有效英文，其余（含无效值/
 * 未设置/null/undefined）一律归 'zh'。useLanguage / LanguageSwitch /
 * readStoredLanguage 共用；新的**语言值归一**一律走本函数，勿再写
 * 'en' ? 'en' : 'zh' 三元（语言值域以外的选择三元——如 antd locale
 * 对象选择——不在此约束内）。
 */
export function normalizeLanguage(
  lang: string | undefined | null
): AppLanguage {
  return lang === 'en' ? 'en' : 'zh'
}

/** 语言相关环境同步：html lang（无障碍/翻译抑制）+ dayjs locale。 */
function syncLanguageEnv(lang: AppLanguage): void {
  document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en'
  dayjs.locale(lang === 'zh' ? 'zh-cn' : 'en')
}

/** 读持久化语言：无效/未设置存储值经 normalizeLanguage 归一。 */
export function readStoredLanguage(): AppLanguage {
  if (typeof window === 'undefined') return DEFAULT_LANGUAGE
  return normalizeLanguage(window.localStorage.getItem(LANGUAGE_STORAGE_KEY))
}

/**
 * 语言切换唯一入口：持久化（localStorage）→ 环境同步（html lang +
 * dayjs）→ i18next.changeLanguage（驱动全部 react-i18next 文案与
 * useLanguage 订阅者重渲染）。
 */
export function setAppLanguage(lang: AppLanguage): void {
  window.localStorage.setItem(LANGUAGE_STORAGE_KEY, lang)
  syncLanguageEnv(lang)
  void i18next.changeLanguage(lang)
}

// 模块加载即初始化（资源同步打包，无异步 backend）：首次渲染前语言已就绪，
// 避免首帧闪错语言。环境同步与 setAppLanguage 走同一函数，杜绝双份映射。
const initial = readStoredLanguage()
syncLanguageEnv(initial)

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
