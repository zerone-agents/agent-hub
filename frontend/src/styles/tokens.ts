/**
 * Semantic CSS-variable bridge for antd-style and inline consumers.
 * Runtime color values are supplied by styles/themes.ts through App.tsx.
 */
export const tokens = {
  // Brand
  ink: 'var(--primary)',
  inkHover: 'var(--primary-hover)',
  inkLight: 'var(--primary-soft)',
  inkLighter: 'var(--primary-softer)',
  inkSubtle: 'var(--primary-subtle)',
  softAccent: 'var(--soft-accent)',

  // Surfaces
  paper: 'var(--background)',
  surface: 'var(--card)',
  surfaceHover: 'var(--secondary)',

  // Text
  text: 'var(--foreground)',
  textSecondary: 'var(--text-secondary)',
  textTertiary: 'var(--muted-foreground)',
  textMuted: 'var(--text-muted)',

  // Status
  success: 'var(--success)',
  warning: 'var(--warning)',
  danger: 'var(--destructive)',

  // Elevation
  elevation1: 'var(--elevation-1)',
  elevation2: 'var(--elevation-2)',
  elevation3: 'var(--elevation-3)',

  // Radius
  radiusSm: 8,
  radius: 12,
  radiusLg: 16,

  // Typography
  fontSans:
    'ui-sans-serif, system-ui, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"',
  fontMono:
    'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',

  // Font sizes
  textXs: '0.75rem',
  textSm: '0.8125rem',
  textBase: '0.9375rem',
  textLg: '1.125rem',
  textXl: '1.375rem',
  text2xl: '1.75rem',
  text3xl: '2.25rem'
} as const

export type Tokens = typeof tokens
