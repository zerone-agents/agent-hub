import { theme as antdAlgorithm, type ThemeConfig } from 'antd'
import { tokens as t } from '@/styles/tokens'
import {
  defaultThemeId,
  getTheme,
  type AppTheme,
  type ThemeAppearance
} from '@/styles/themes'

function withAlpha(hex: string, alpha: number): string {
  const value = hex.replace('#', '')
  const red = Number.parseInt(value.slice(0, 2), 16)
  const green = Number.parseInt(value.slice(2, 4), 16)
  const blue = Number.parseInt(value.slice(4, 6), 16)
  return `rgba(${red}, ${green}, ${blue}, ${alpha})`
}

export function createAntdTheme(
  theme: AppTheme,
  appearance: ThemeAppearance
): ThemeConfig {
  const colors = theme[appearance]
  const markdownAccent = withAlpha(colors.accent, 0.2)

  return {
    algorithm:
      appearance === 'dark'
        ? antdAlgorithm.darkAlgorithm
        : antdAlgorithm.defaultAlgorithm,
    token: {
      colorPrimary: colors.primary,
      colorPrimaryHover: colors.ring,
      colorBgBase: colors.background,
      colorBgContainer: colors.card,
      colorBgElevated: colors.popover,
      colorBgLayout: colors.background,
      colorText: colors.foreground,
      colorTextLightSolid: colors.primaryForeground,
      colorTextSecondary: colors.mutedForeground,
      colorTextTertiary: colors.mutedForeground,
      colorTextQuaternary: colors.mutedForeground,
      colorBorder: colors.border,
      colorBorderSecondary: colors.border,
      colorFillSecondary: colors.muted,
      colorFillTertiary: markdownAccent,
      // Lobe Markdown code blocks consume this token. Retain the preset's
      // accent hue while softening it against the card surface.
      colorFillQuaternary: markdownAccent,
      colorSuccess: colors.chart2,
      colorWarning: colors.chart3,
      colorError: colors.destructive,
      colorLink: colors.primary,
      controlOutline: colors.ring,
      borderRadius: t.radius,
      borderRadiusSM: t.radiusSm,
      borderRadiusLG: t.radiusLg,
      fontFamily: t.fontSans,
      fontFamilyCode: t.fontMono,
      fontSize: 13,
      fontSizeLG: 16,
      fontSizeHeading1: 30,
      fontSizeHeading2: 24,
      wireframe: false
    },
    components: {
      Layout: {
        bodyBg: colors.background,
        headerBg: colors.card,
        siderBg: colors.sidebar
      },
      Card: {
        borderRadiusLG: t.radius,
        colorBorderSecondary: colors.border
      },
      Modal: { borderRadiusLG: t.radiusLg },
      Button: {
        fontWeight: 600,
        primaryShadow: 'none',
        primaryColor: colors.primaryForeground,
        defaultBorderColor: colors.border
      },
      Menu: {
        itemBorderRadius: t.radiusSm,
        itemSelectedBg: colors.sidebarAccent,
        itemSelectedColor: colors.sidebarAccentForeground
      },
      Segmented: { borderRadius: t.radiusSm },
      Input: {
        borderRadius: t.radiusSm,
        activeBorderColor: colors.ring,
        hoverBorderColor: colors.primary
      },
      Select: {
        borderRadius: t.radiusSm,
        activeBorderColor: colors.ring,
        hoverBorderColor: colors.primary
      },
      Table: {
        headerBg: colors.muted,
        headerColor: colors.foreground,
        rowHoverBg: colors.secondary,
        borderColor: colors.border
      }
    }
  }
}

// Stable default retained for isolated component tests.
export const antdTheme = createAntdTheme(getTheme(defaultThemeId), 'light')

// Form validateMessages 已迁至 i18n 资源（validate.* key，值保留 antd 的
// ${label} 语法），由 App.tsx 内 t() 构造——语言切换时随 ConfigProvider 重渲染。
