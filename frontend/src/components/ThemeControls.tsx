import { DesktopIcon, MoonIcon, PaletteIcon, SunIcon } from '@phosphor-icons/react'
import { Dropdown, Tooltip } from 'antd'
import { createStyles } from 'antd-style'
import { themes, type ThemePreference } from '@/styles/themes'
import { useThemeStore } from '@/stores/theme'
import { useTranslation } from 'react-i18next'

const useStyles = createStyles(({ css }) => ({
  controls: css`
    display: flex;
    align-items: center;
    gap: 4px;
  `,
  button: css`
    width: 34px;
    height: 34px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    background: transparent;
    cursor: pointer;
    transition:
      color 160ms ease,
      background 160ms ease,
      border-color 160ms ease,
      transform 160ms ease;

    &:hover {
      color: var(--primary);
      background: var(--accent);
      border-color: var(--border);
    }

    &:active {
      transform: translateY(1px);
    }

    &:focus-visible {
      outline: 2px solid var(--ring);
      outline-offset: 2px;
    }
  `
}))

const appearanceOptions: {
  key: ThemePreference
  label: string
  icon: React.ReactNode
}[] = [
  { key: 'light', label: 'components.themeControls.light', icon: <SunIcon size={16} /> },
  { key: 'dark', label: 'components.themeControls.dark', icon: <MoonIcon size={16} /> },
  { key: 'system', label: 'components.themeControls.system', icon: <DesktopIcon size={16} /> }
]

export default function ThemeControls() {
  const { styles } = useStyles()
  const { t } = useTranslation()
  const themeId = useThemeStore((state) => state.themeId)
  const preference = useThemeStore((state) => state.preference)
  const setThemeId = useThemeStore((state) => state.setThemeId)
  const setPreference = useThemeStore((state) => state.setPreference)

  const AppearanceIcon =
    preference === 'dark' ? MoonIcon : preference === 'light' ? SunIcon : DesktopIcon

  return (
    <div className={styles.controls} aria-label={t('components.themeControls.settings')}>
      <Dropdown
        trigger={['click']}
        menu={{
          selectedKeys: [themeId],
          onClick: ({ key }) => { setThemeId(key); },
          items: themes
            .slice()
            .sort((a, b) => a.order - b.order)
            .map((theme) => ({
              key: theme.id,
              label: t(theme.label),
              icon: (
                <PaletteIcon size={16} color={theme.light.primary} weight="fill" />
              )
            }))
        }}
      >
        <Tooltip title={t('components.themeControls.switchColor')}>
          <button type="button" className={styles.button} aria-label={t('components.themeControls.switchColor')}>
            <PaletteIcon size={18} />
          </button>
        </Tooltip>
      </Dropdown>

      <Dropdown
        trigger={['click']}
        menu={{
          selectedKeys: [preference],
          onClick: ({ key }) => { setPreference(key as ThemePreference); },
          items: appearanceOptions.map((o) => ({ ...o, label: t(o.label) }))
        }}
      >
        <Tooltip title={t('components.themeControls.switchMode')}>
          <button
            type="button"
            className={styles.button}
            aria-label={t('components.themeControls.switchMode')}
          >
            <AppearanceIcon size={18} />
          </button>
        </Tooltip>
      </Dropdown>
    </div>
  )
}
