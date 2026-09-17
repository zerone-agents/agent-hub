import { useEffect, useMemo } from 'react'
import { ConfigProvider } from 'antd'
import { RouterProvider } from 'react-router/dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider as LobeThemeProvider } from '@lobehub/ui'
import { ThemeProvider as StyleThemeProvider } from 'antd-style'
import { App as AntdApp } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import enUS from 'antd/locale/en_US'
import { router } from '@/routes'
import { queryClient } from '@/lib/query-client'
import { createAntdTheme } from '@/lib/antd-theme'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { ManualCopyHost } from '@/components/ManualCopyDialog'
import { consumeAuthParams } from '@/lib/consume-auth-params'
import { useLanguage } from '@/hooks/useLanguage'
import { tokens as tk } from '@/styles/tokens'
import { useTranslation } from 'react-i18next'
import { getTheme, type ThemeColors } from '@/styles/themes'
import { useThemeStore } from '@/stores/theme'

const cssVariableNames: Record<keyof ThemeColors, string> = {
  background: '--background',
  foreground: '--foreground',
  card: '--card',
  cardForeground: '--card-foreground',
  popover: '--popover',
  popoverForeground: '--popover-foreground',
  primary: '--primary',
  primaryForeground: '--primary-foreground',
  secondary: '--secondary',
  secondaryForeground: '--secondary-foreground',
  muted: '--muted',
  mutedForeground: '--muted-foreground',
  accent: '--accent',
  accentForeground: '--accent-foreground',
  destructive: '--destructive',
  border: '--border',
  input: '--input',
  ring: '--ring',
  chart1: '--chart-1',
  chart2: '--chart-2',
  chart3: '--chart-3',
  chart4: '--chart-4',
  chart5: '--chart-5',
  sidebar: '--sidebar',
  sidebarForeground: '--sidebar-foreground',
  sidebarPrimary: '--sidebar-primary',
  sidebarPrimaryForeground: '--sidebar-primary-foreground',
  sidebarAccent: '--sidebar-accent',
  sidebarAccentForeground: '--sidebar-accent-foreground',
  sidebarBorder: '--sidebar-border',
  sidebarRing: '--sidebar-ring'
}

// One-shot SSO token extraction via consumeAuthParams. Done at module load
// (before React mounts) so the credentials are already stored when
// RequireAuth runs its first render; the helper strips only the
// token/refreshToken params and preserves the rest of the query + hash
// (the previous inline logic wiped the whole query string). Previously
// this lived inside the App component behind a useRef guard, but reading
// a ref during render violates react-hooks/refs and reassigning
// module-scope let from within a component violates react-hooks/globals —
// both are side effects during render.
if (typeof window !== 'undefined') {
  const cleaned = consumeAuthParams(window.location.href)
  if (cleaned !== null) {
    window.history.replaceState({}, '', cleaned)
  }
}

export default function App() {
  // Extract token from URL before the router renders so that RequireAuth
  // sees the credentials on its first render (Casdoor SSO callback).
  // The init runs once at module load — equivalent to the previous
  // `useRef(true)` "init once" guard, but without reading a ref during render
  // (which violates react-hooks/refs).
  const themeId = useThemeStore((state) => state.themeId)
  const appearance = useThemeStore((state) => state.appearance)
  const syncSystemAppearance = useThemeStore(
    (state) => state.syncSystemAppearance
  )
  const { language } = useLanguage()
  const { t } = useTranslation()
  // antd Form 校验消息：值保留 antd 的 ${label} 语法（资源侧注释有说明），
  // 语言切换时 ConfigProvider 随 language 重渲染、消息随之切换。
  const validateMessages = {
    default: t('validate.default'),
    required: t('validate.required'),
    enum: t('validate.enum'),
    whitespace: t('validate.whitespace'),
    types: {
      email: t('validate.types.email'),
      url: t('validate.types.url')
    },
    string: {
      len: t('validate.string.len'),
      min: t('validate.string.min'),
      max: t('validate.string.max')
    },
    number: {
      min: t('validate.number.min'),
      max: t('validate.number.max')
    }
  }
  const selectedTheme = getTheme(themeId)
  const antdTheme = useMemo(
    () => createAntdTheme(selectedTheme, appearance),
    [selectedTheme, appearance]
  )

  useEffect(() => {
    const root = document.documentElement
    const colors = selectedTheme[appearance]

    Object.entries(colors).forEach(([key, value]) => {
      root.style.setProperty(cssVariableNames[key as keyof ThemeColors], value as string)
    })
    root.dataset.theme = selectedTheme.id
    root.dataset.appearance = appearance
    root.style.colorScheme = appearance
  }, [selectedTheme, appearance])

  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    media.addEventListener('change', syncSystemAppearance)
    return () => { media.removeEventListener('change', syncSystemAppearance); }
  }, [syncSystemAppearance])

  return (
    <ErrorBoundary>
      <LobeThemeProvider
        appearance={appearance}
        theme={antdTheme}
        customFonts={[tk.fontSans, tk.fontMono]}
      >
        <StyleThemeProvider theme={antdTheme}>
          <ConfigProvider
            locale={language === 'zh' ? zhCN : enUS}
            theme={antdTheme}
            form={{ validateMessages }}
          >
            <AntdApp>
              <QueryClientProvider client={queryClient}>
                <RouterProvider router={router} />
              </QueryClientProvider>
              <ManualCopyHost />
            </AntdApp>
          </ConfigProvider>
        </StyleThemeProvider>
      </LobeThemeProvider>
    </ErrorBoundary>
  )
}
