import { create } from 'zustand'
import {
  defaultThemeId,
  getTheme,
  type ThemeAppearance,
  type ThemePreference
} from '@/styles/themes'

const THEME_STORAGE_KEY = 'agent-hub.theme'
const APPEARANCE_STORAGE_KEY = 'agent-hub.appearance'

function readStoredValue<T extends string>(key: string, fallback: T): T {
  if (typeof window === 'undefined') return fallback
  return (window.localStorage.getItem(key) as T | null) ?? fallback
}

function getSystemAppearance(): ThemeAppearance {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

interface ThemeState {
  themeId: string
  preference: ThemePreference
  appearance: ThemeAppearance
  setThemeId: (themeId: string) => void
  setPreference: (preference: ThemePreference) => void
  syncSystemAppearance: () => void
}

export const useThemeStore = create<ThemeState>((set, get) => {
  const preference = readStoredValue<ThemePreference>(
    APPEARANCE_STORAGE_KEY,
    'system'
  )
  const storedThemeId = readStoredValue(THEME_STORAGE_KEY, defaultThemeId)
  const themeId = getTheme(storedThemeId).id

  return {
    themeId,
    preference,
    appearance: preference === 'system' ? getSystemAppearance() : preference,
    setThemeId: (nextThemeId) => {
      const safeThemeId = getTheme(nextThemeId).id
      window.localStorage.setItem(THEME_STORAGE_KEY, safeThemeId)
      set({ themeId: safeThemeId })
    },
    setPreference: (nextPreference) => {
      window.localStorage.setItem(APPEARANCE_STORAGE_KEY, nextPreference)
      set({
        preference: nextPreference,
        appearance:
          nextPreference === 'system' ? getSystemAppearance() : nextPreference
      })
    },
    syncSystemAppearance: () => {
      if (get().preference === 'system') {
        set({ appearance: getSystemAppearance() })
      }
    }
  }
})
