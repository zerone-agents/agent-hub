import { type ReactNode } from 'react'
import { createStyles } from 'antd-style'
import BrandMark from '@/components/BrandMark'
import ThemeControls from '@/components/ThemeControls'
import HeaderLinks from '@/components/HeaderLinks'
import UserDropdown from '@/components/UserDropdown'
import { tokens as t } from '@/styles/tokens'

// 聊天域共享页眉壳：/agents/chat 总览与 /agents/:name/chat 聊天页同款排版
// （对齐管理页 AppHeader：sticky 毛玻璃、52px、右侧 HeaderLinks + 主题控件 +
// 用户下拉——菜单仅修改密码与退出）。left 由页面注入（总览页为品牌、聊天页
// 为返回按钮 + Agent 切换器）；fill 切换滚动模型（聊天页视口填充内部滚动，
// 总览页文档滚动）。
const useStyles = createStyles(({ css }) => ({
  page: css`
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--background);
  `,
  pageFill: css`
    height: 100vh;
    min-height: 0;
    overflow: hidden;
  `,
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
    padding: 0 32px;
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
    min-width: 0;
  `,
  brand: css`
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 16px;
    font-weight: 700;
    color: ${t.ink};
  `,
  actions: css`
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  `,
  main: css`
    flex: 1;
    width: 100%;
    box-sizing: border-box;
  `,
  mainFill: css`
    display: flex;
    flex-direction: column;
    min-height: 0;
  `,
}))

interface ChatLayoutProps {
  /** 页眉左侧内容（默认：BrandMark + 「Agent 聊天」品牌）。 */
  left?: ReactNode
  /** 注入在右侧 actions 组头部的节点（如 guest 的「体验模式」徽标）。 */
  badge?: ReactNode
  /** true = 视口填充模式（页内滚动，聊天页用）；默认文档滚动（总览页用）。 */
  fill?: boolean
  children: ReactNode
}

export default function ChatLayout({ left, badge, fill, children }: ChatLayoutProps) {
  const { styles, cx } = useStyles()
  return (
    <div className={cx(styles.page, fill && styles.pageFill)}>
      <header className={styles.header}>
        <div className={styles.inner}>
          <div className={styles.left}>
            {left ?? (
              <div className={styles.brand}>
                <BrandMark size={28} />
                Agent 聊天
              </div>
            )}
          </div>
          <div className={styles.actions}>
            {badge}
            <HeaderLinks />
            <ThemeControls />
            {/* 用户下拉与管理页同款，菜单仅保留修改密码与退出 */}
            <UserDropdown />
          </div>
        </div>
      </header>
      <main className={cx(styles.main, fill && styles.mainFill)}>{children}</main>
    </div>
  )
}
