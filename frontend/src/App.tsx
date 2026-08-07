import { useEffect, useMemo, useRef } from 'react'
import { ConfigProvider } from 'antd'
import { RouterProvider } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider as LobeThemeProvider } from '@lobehub/ui'
import { ThemeProvider as StyleThemeProvider } from 'antd-style'
import { App as AntdApp } from 'antd'
import { router } from '@/routes'
import { queryClient } from '@/lib/query-client'
import { createAntdTheme, formValidateMessages } from '@/lib/antd-theme'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { setTokens } from '@/api/client'
import { tokens as t } from '@/styles/tokens'
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

export default function App() {
  // Extract token from URL before the router renders so that RequireAuth
  // sees the credentials on its first render (Casdoor SSO callback).
  const tokenInitialized = useRef(false)
  const themeId = useThemeStore((state) => state.themeId)
  const appearance = useThemeStore((state) => state.appearance)
  const syncSystemAppearance = useThemeStore(
    (state) => state.syncSystemAppearance
  )
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

  if (!tokenInitialized.current && typeof window !== 'undefined') {
    tokenInitialized.current = true
    const params = new URLSearchParams(window.location.search)
    const token = params.get('token')
    if (token) {
      setTokens(token, params.get('refreshToken') ?? undefined)
      window.history.replaceState({}, '', window.location.pathname)
    }
  }

  return (
    <ErrorBoundary>
      <LobeThemeProvider
        appearance={appearance}
        theme={antdTheme}
        customFonts={[t.fontSans, t.fontMono]}
      >
        <StyleThemeProvider theme={antdTheme}>
          <ConfigProvider
            theme={antdTheme}
            form={{ validateMessages: formValidateMessages }}
          >
            <AntdApp>
              <QueryClientProvider client={queryClient}>
                <RouterProvider
                  router={router}
                  future={{ v7_startTransition: true }}
                />
              </QueryClientProvider>
            </AntdApp>
          </ConfigProvider>
        </StyleThemeProvider>
      </LobeThemeProvider>
    </ErrorBoundary>
  )
}
