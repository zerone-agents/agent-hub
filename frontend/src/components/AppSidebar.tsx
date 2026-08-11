import { Tooltip } from 'antd'
import { useNavigate, useLocation } from 'react-router'
import { createStyles } from 'antd-style'
import { NAV_ITEMS } from '@/lib/nav'
import { tokens as t } from '@/styles/tokens'
import BrandMark from '@/components/BrandMark'

const useStyles = createStyles(({ css }) => ({
  sidebar: css`
    width: 208px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    background: var(--sidebar);
    position: relative;
    z-index: 110;
    border-right: 1px solid color-mix(in srgb, var(--sidebar-border) 72%, transparent);
    box-shadow: 8px 0 18px -14px color-mix(in srgb, var(--foreground) 24%, transparent);
    transition: width 0.2s ease;
    overflow: hidden;
    @media (max-width: 768px) {
      display: none;
    }
  `,
  sidebarCollapsed: css`
    width: 64px;
  `,
  logo: css`
    display: flex;
    align-items: center;
    gap: 10px;
    height: 52px;
    padding: 0 20px;
    color: var(--sidebar-primary);
    flex-shrink: 0;
  `,
  logoCollapsed: css`
    padding: 0;
    justify-content: center;
  `,
  logoText: css`
    font-size: ${t.textXl};
    font-weight: 700;
    letter-spacing: -0.02em;
    line-height: 1;
    white-space: nowrap;
  `,
  nav: css`
    flex: 1;
    padding: 6px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    overflow-y: auto;
  `,
  navLink: css`
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 11px 18px;
    font-size: 14px;
    font-weight: 500;
    color: color-mix(in srgb, var(--sidebar-foreground) 66%, transparent);
    border-radius: ${t.radius}px;
    cursor: pointer;
    transition: color 0.2s ease, background 0.2s ease;
    border: none;
    background: transparent;
    white-space: nowrap;
    text-align: left;
    width: 100%;
    &:hover {
      color: var(--sidebar-foreground);
      background: var(--sidebar-accent);
    }
  `,
  navLinkCollapsed: css`
    padding: 11px;
    justify-content: center;
  `,
  navLinkActive: css`
    color: var(--sidebar-accent-foreground);
    font-weight: 600;
    background: var(--sidebar-accent);
    &:hover {
      color: var(--sidebar-accent-foreground);
      background: var(--sidebar-accent);
    }
  `
}))

interface AppSidebarProps {
  collapsed: boolean
}

export default function AppSidebar({ collapsed }: AppSidebarProps) {
  const { styles, cx } = useStyles()
  const navigate = useNavigate()
  const location = useLocation()

  return (
    <aside className={cx(styles.sidebar, collapsed && styles.sidebarCollapsed)}>
      <div className={cx(styles.logo, collapsed && styles.logoCollapsed)}>
        <BrandMark size={30} />
        {!collapsed && <span className={styles.logoText}>Zerone Hub</span>}
      </div>

      <nav className={styles.nav}>
        {NAV_ITEMS.map((item) => {
          const active = location.pathname.startsWith(item.path)
          const IconComp = item.icon
          const button = (
            <button
              key={item.id}
              type="button"
              aria-label={collapsed ? item.label : undefined}
              aria-current={active ? 'page' : undefined}
              className={cx(
                styles.navLink,
                collapsed && styles.navLinkCollapsed,
                active && styles.navLinkActive
              )}
              onClick={() => { navigate(item.path); }}
            >
              <IconComp size={18} weight={active ? 'fill' : 'regular'} />
              {!collapsed && item.label}
            </button>
          )
          return collapsed ? (
            <Tooltip key={item.id} title={item.label} placement="right">
              {button}
            </Tooltip>
          ) : (
            button
          )
        })}
      </nav>
    </aside>
  )
}
