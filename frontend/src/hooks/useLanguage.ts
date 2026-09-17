import { useCallback, useEffect, useState } from 'react'
import i18next, { setAppLanguage, type AppLanguage } from '@/i18n'

function normalize(lng: string | undefined): AppLanguage {
  return lng === 'en' ? 'en' : 'zh'
}

/**
 * 响应式语言状态。i18next 实例是单一事实来源，本 hook 只订阅
 * languageChanged 事件——所有经 setAppLanguage 的切换（无论来自哪个组件）
 * 都会驱动使用方重渲染。
 */
export function useLanguage() {
  const [language, setLocal] = useState<AppLanguage>(() =>
    normalize(i18next.language)
  )
  useEffect(() => {
    const onChange = (lng: string) => setLocal(normalize(lng))
    i18next.on('languageChanged', onChange)
    return () => {
      i18next.off('languageChanged', onChange)
    }
  }, [])
  const set = useCallback((lang: AppLanguage) => setAppLanguage(lang), [])
  return { language, setLanguage: set }
}
