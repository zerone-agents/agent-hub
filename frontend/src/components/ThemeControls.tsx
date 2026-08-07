import { Desktop, Moon, Palette, Sun } from '@phosphor-icons/react'
import { Dropdown, Tooltip } from 'antd'
import { createStyles } from 'antd-style'
import { themes, type ThemePreference } from '@/styles/themes'
import { useThemeStore } from '@/stores/theme'

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
  { key: 'light', label: '浅色', icon: <Sun size={16} /> },
  { key: 'dark', label: '深色', icon: <Moon size={16} /> },
  { key: 'system', label: '跟随系统', icon: <Desktop size={16} /> }
]

export default function ThemeControls() {
  const { styles } = useStyles()
  const themeId = useThemeStore((state) => state.themeId)
  const preference = useThemeStore((state) => state.preference)
  const setThemeId = useThemeStore((state) => state.setThemeId)
  const setPreference = useThemeStore((state) => state.setPreference)

  const AppearanceIcon =
    preference === 'dark' ? Moon : preference === 'light' ? Sun : Desktop

  return (
    <div className={styles.controls} aria-label="主题设置">
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
              label: theme.label,
              icon: (
                <Palette size={16} color={theme.light.primary} weight="fill" />
              )
            }))
        }}
      >
        <Tooltip title="切换配色">
          <button type="button" className={styles.button} aria-label="切换配色">
            <Palette size={18} />
          </button>
        </Tooltip>
      </Dropdown>

      <Dropdown
        trigger={['click']}
        menu={{
          selectedKeys: [preference],
          onClick: ({ key }) => { setPreference(key as ThemePreference); },
          items: appearanceOptions
        }}
      >
        <Tooltip title="切换明暗模式">
          <button
            type="button"
            className={styles.button}
            aria-label="切换明暗模式"
          >
            <AppearanceIcon size={18} />
          </button>
        </Tooltip>
      </Dropdown>
    </div>
  )
}
