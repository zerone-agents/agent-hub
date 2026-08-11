import { useState, useEffect, useRef } from 'react'
import { Avatar, Breadcrumb, Dropdown } from 'antd'
import { useNavigate, useLocation, Link } from 'react-router'
import { SignOutIcon, KeyIcon, ListIcon, ShieldCheckIcon, SidebarSimpleIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useAuthStore } from '@/stores/auth'
import { NAV_ITEMS, getBreadcrumbs } from '@/lib/nav'
import { useKnowledgeDetail } from '@/queries/useKnowledge'
import { tokens as t } from '@/styles/tokens'
import ThemeControls from '@/components/ThemeControls'

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
  userArea: css`
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 12px 4px 16px;
    border-radius: 20px;
    cursor: pointer;
    transition: background 0.15s;
    &:hover {
      background: ${t.surfaceHover};
    }
  `,
  actions: css`
    display: flex;
    align-items: center;
    gap: 8px;
  `,
  userName: css`
    font-size: ${t.textSm};
    font-weight: 500;
    color: ${t.textSecondary};
    @media (max-width: 768px) {
      display: none;
    }
  `,
  avatar: css`
    background: var(--primary);
    color: var(--primary-foreground);
  `
}))

interface AppHeaderProps {
  onToggleSidebar: () => void
}

function getAvatarInitial(name?: string) {
  const firstCharacter = Array.from(name?.trim() ?? '')[0]
  return firstCharacter ? firstCharacter.toLocaleUpperCase() : 'U'
}

export default function AppHeader({ onToggleSidebar }: AppHeaderProps) {
  const { styles, cx } = useStyles()
  const navigate = useNavigate()
  const location = useLocation()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)

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

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  const handleNavClick = (path: string) => {
    navigate(path)
    setMobileMenuOpen(false)
  }

  const dropdownItems = [
    {
      key: 'cli-tokens',
      icon: <KeyIcon size={14} />,
      label: 'CLI Tokens',
      onClick: () => { navigate('/settings/cli-tokens'); }
    },
    {
      key: 'aigc-config',
      icon: <ShieldCheckIcon size={14} />,
      label: 'AIGC 标识配置',
      onClick: () => { navigate('/settings/aigc'); }
    },
    {
      key: 'logout',
      icon: <SignOutIcon size={14} />,
      label: '退出登录',
      onClick: handleLogout
    }
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
            aria-label="切换侧边栏"
          >
            <SidebarSimpleIcon size={20} />
          </button>

          {/* 移动端汉堡按钮 */}
          <button
            type="button"
            className={cx(styles.burgerBtn, mobileMenuOpen && styles.burgerBtnActive)}
            onClick={() => { setMobileMenuOpen(!mobileMenuOpen); }}
            aria-label="菜单"
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
          <ThemeControls />
          <Dropdown menu={{ items: dropdownItems }} trigger={['click']}>
            <div className={styles.userArea}>
              <span className={styles.userName}>{user?.name ?? 'Admin'}</span>
              <Avatar className={styles.avatar} size={28}>
                {getAvatarInitial(user?.name)}
              </Avatar>
            </div>
          </Dropdown>
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
                onClick={() => { handleNavClick(item.path); }}
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
