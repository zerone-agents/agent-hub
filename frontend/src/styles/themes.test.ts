import { describe, expect, it } from 'vitest'
import { defaultThemeId, getTheme, themes } from './themes'

describe('theme registry', () => {
  it('uses the terracotta palette as the default theme', () => {
    const theme = getTheme(defaultThemeId)

    expect(theme.id).toBe('terracotta')
    expect(theme.light.primary).toBe('#d96f4f')
    expect(theme.light.background).toBe('#f3f1ed')
  })

  it('keeps light and dark palettes structurally aligned', () => {
    for (const theme of themes) {
      expect(Object.keys(theme.dark).sort()).toEqual(
        Object.keys(theme.light).sort()
      )
    }
  })

  it('registers all built-in theme presets in display order', () => {
    expect(themes.map((theme) => theme.id)).toEqual([
      'terracotta',
      'slack',
      'discord',
      'twitter',
      'github',
      'facebook'
    ])
    expect(themes.map((theme) => theme.order)).toEqual([1, 2, 3, 4, 5, 6])
  })

  it('falls back to the default theme for unknown ids', () => {
    expect(getTheme('unknown-theme').id).toBe(defaultThemeId)
  })
})
