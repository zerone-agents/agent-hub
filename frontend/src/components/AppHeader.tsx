import { useState, useEffect, useRef } from 'react'
import { Breadcrumb } from 'antd'
import { useNavigate, useLocation, Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { KeyIcon, ListIcon, ScrollIcon, ShieldCheckIcon, SidebarSimpleIcon, UsersIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { MenuProps } from 'antd'
import { useAuthStore } from '@/stores/auth'
import { useCanWrite } from '@/hooks/useCanWrite'
import { NAV_ITEMS, getBreadcrumbs } from '@/lib/nav'
import { useKnowledgeDetail } from '@/queries/useKnowledge'
import { tokens as t } from '@/styles/tokens'
import ThemeControls from '@/components/ThemeControls'
import HeaderLinks from '@/components/HeaderLinks'
import LanguageSwitch from '@/components/LanguageSwitch'
import UserDropdown from '@/components/UserDropdown'

const useStyles = createStyles(({ css }) => ({
  header: css`
    position: sticky;
    top: 0;
    z-index: 100;
    background: color-mix(in srgb, var(--card) 92%, transparent);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    box-shadow: 0 1px 0 var(--border);
  `,
  inner: css`
    padding: 0 32px 0 12px;
    height: 52px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    @media (max-width: 768px) {
      padding: 0 16px;
    }
  `,
  left: css`
    display: flex;
    align-items: center;
    gap: 12px;
  `,
  toggleBtn: css`
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    background: transparent;
    border-radius: ${t.radiusSm}px;
    color: ${t.text};
    cursor: pointer;
    transition: background 0.15s;
    &:hover {
      background: ${t.inkSubtle};
    }
    @media (max-width: 768px) {
      display: none;
    }
  `,
  burgerBtn: css`
    display: none;
    @media (max-width: 768px) {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      border: none;
      background: transparent;
      border-radius: ${t.radiusSm}px;
      color: ${t.text};
      cursor: pointer;
      transition: background 0.15s;
      &:hover {
        background: ${t.inkSubtle};
      }
    }
  `,
  burgerBtnActive: css`
    background: ${t.inkSubtle} !important;
  `,
  breadcrumb: css`
    font-size: ${t.textSm};
    @media (max-width: 768px) {
      display: none;
    }
  `,
  mobileMenu: css`
    display: none;
    @media (max-width: 768px) {
      display: block;
      position: absolute;
      top: 52px;
      left: 0;
      right: 0;
      background: var(--popover);
      box-shadow: var(--elevation-2);
      border-bottom: 1px solid var(--border);
      padding: 8px 0;
      max-height: calc(100vh - 52px);
      overflow-y: auto;
      animation: slideDown 0.2s ease;
      @keyframes slideDown {
        from { opacity: 0; transform: translateY(-8px); }
        to { opacity: 1; transform: translateY(0); }
      }
    }
  `,
  mobileMenuItem: css`
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 20px;
    font-size: ${t.textBase};
    font-weight: 500;
    color: ${t.text};
    text-decoration: none;
    cursor: pointer;
    transition: background 0.15s;
    border: none;
    background: transparent;
    width: 100%;
    text-align: left;
    &:hover {
      background: ${t.inkSubtle};
    }
  `,
  mobileMenuItemActive: css`
    color: ${t.ink};
    font-weight: 600;
    background: ${t.inkSubtle};
  `,
  actions: css`
    display: flex;
    align-items: center;
    gap: 8px;
  `
}))

interface AppHeaderProps {
  onToggleSidebar: () => void
}

export default function AppHeader({ onToggleSidebar }: AppHeaderProps) {
  const { styles, cx } = useStyles()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const user = useAuthStore((s) => s.user)
  const canWrite = useCanWrite()

  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  // 知识库详情页面包屑显示具体库名
  const kbId = (/^\/knowledge\/([^/]+)/.exec(location.pathname))?.[1] ?? ''
  const { data: kbDetail } = useKnowledgeDetail(kbId)
  const breadcrumbs = getBreadcrumbs(location.pathname, kbDetail?.name)

  // 点击外部关闭菜单
  useEffect(() => {
    if (!mobileMenuOpen) return
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMobileMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => { document.removeEventListener('mousedown', handleClick); }
  }, [mobileMenuOpen])

  const handleNavClick = async (path: string) => {
    await navigate(path)
    setMobileMenuOpen(false)
  }

  // 管理页专属菜单项（注入 UserDropdown，排在「修改密码」之前）。
  const userMenuExtraItems: MenuProps['items'] = [
    ...(user?.role === 'admin'
      ? [{
          key: 'users',
          icon: <UsersIcon size={14} />,
          label: t('components.appHeader.users'),
          onClick: async () => { await navigate('/settings/users'); }
        }]
      : []),
    ...(user?.role === 'admin'
      ? [{
          key: 'audit-logs',
          icon: <ScrollIcon size={14} />,
          label: t('components.appHeader.auditLogs'),
          onClick: async () => { await navigate('/settings/audit-logs'); }
        }]
      : []),
    ...(canWrite
      ? [{
          key: 'cli-tokens',
          icon: <KeyIcon size={14} />,
          label: 'CLI Tokens',
          onClick: async () => { await navigate('/settings/cli-tokens'); }
        }]
      : []),
    ...(canWrite
      ? [{
          key: 'aigc-config',
          icon: <ShieldCheckIcon size={14} />,
          label: t('components.appHeader.aigcConfig'),
          onClick: async () => { await navigate('/settings/aigc'); }
        }]
      : [])
  ]

  return (
    <header className={styles.header} ref={menuRef}>
      <div className={styles.inner}>
        <div className={styles.left}>
          {/* 桌面端侧边栏收起切换 */}
          <button
            type="button"
            className={styles.toggleBtn}
            onClick={onToggleSidebar}
            aria-label={t('components.appHeader.toggleSidebar')}
          >
            <SidebarSimpleIcon size={20} />
          </button>

          {/* 移动端汉堡按钮 */}
          <button
            type="button"
            className={cx(styles.burgerBtn, mobileMenuOpen && styles.burgerBtnActive)}
            onClick={() => { setMobileMenuOpen(!mobileMenuOpen); }}
            aria-label={t('components.appHeader.menu')}
          >
            <ListIcon size={22} weight="bold" />
          </button>

          {/* 当前页面路径 */}
          <Breadcrumb
            className={styles.breadcrumb}
            items={breadcrumbs.map((item) => ({
              title: item.path ? <Link to={item.path}>{item.label}</Link> : item.label
            }))}
          />
        </div>

        <div className={styles.actions}>
          <HeaderLinks />
          <LanguageSwitch />
          <ThemeControls />
          <UserDropdown extraItems={userMenuExtraItems} />
        </div>
      </div>

      {/* 移动端下拉菜单 */}
      {mobileMenuOpen && (
        <div className={styles.mobileMenu}>
          {NAV_ITEMS.map((item) => {
            const active = location.pathname.startsWith(item.path)
            const IconComp = item.icon
            return (
              <button
                key={item.id}
                type="button"
                className={cx(styles.mobileMenuItem, active && styles.mobileMenuItemActive)}
                onClick={async () => { await handleNavClick(item.path); }}
              >
                <IconComp size={18} />
                {item.label}
              </button>
            )
          })}
        </div>
      )}
    </header>
  )
}
