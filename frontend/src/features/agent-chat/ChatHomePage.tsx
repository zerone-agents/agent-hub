import { useNavigate } from 'react-router'
import { createStyles } from 'antd-style'
import { Empty } from 'antd'
import { ArrowLeftIcon, ChatCircleDotsIcon, SignOutIcon } from '@phosphor-icons/react'
import { usePublicAgents } from '@/queries/useAgents'
import { useAuthMode } from '@/features/login/useAuthMode'
import { useUserInfo } from '@/queries/useUserInfo'
import { isGuestUser } from '@/lib/auth-guest'
import { useAuthStore } from '@/stores/auth'
import BrandMark from '@/components/BrandMark'
import ThemeControls from '@/components/ThemeControls'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: ${t.paper};
  `,
  header: css`
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 24px;
    border-bottom: 1px solid var(--border);
  `,
  brand: css`
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 16px;
    font-weight: 700;
    color: ${t.ink};
  `,
  spacer: css`
    flex: 1;
  `,
  badge: css`
    padding: 2px 10px;
    border-radius: 999px;
    font-size: 12px;
    background: color-mix(in srgb, ${t.softAccent} 18%, transparent);
    color: ${t.ink};
  `,
  headerBtn: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card);
    color: ${t.text};
    padding: 6px 12px;
    font-size: 13px;
    cursor: pointer;
    &:hover { background: color-mix(in srgb, ${t.ink} 6%, transparent); }
  `,
  username: css`
    font-size: 13px;
    color: ${t.textMuted};
  `,
  main: css`
    flex: 1;
    padding: 32px 24px;
    max-width: 1080px;
    width: 100%;
    margin: 0 auto;
    box-sizing: border-box;
  `,
  grid: css`
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 16px;
  `,
  card: css`
    all: unset;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 20px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--card);
    cursor: pointer;
    transition: box-shadow 0.15s ease, transform 0.15s ease;
    &:hover { box-shadow: var(--elevation-2); transform: translateY(-2px); }
    &:focus-visible { outline: 2px solid ${t.ink}; outline-offset: 2px; }
  `,
  cardIcon: css`
    width: 40px;
    height: 40px;
    border-radius: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
  `,
  cardTitle: css`
    font-size: 15px;
    font-weight: 600;
    color: ${t.text};
  `,
  cardDesc: css`
    font-size: 13px;
    color: ${t.textMuted};
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  `,
}))

/**
 * Agent 聊天第二主页（/agents/chat，spec 6.1）：guest 落地态与正式用户的
 * 聊天总览。数据来自公开聊天视图（guest 仅见 guestEnabled，服务端过滤）。
 */
export default function ChatHomePage() {
  const { styles } = useStyles()
  const navigate = useNavigate()
  const { data: agents, isLoading } = usePublicAgents()
  const { data: mode } = useAuthMode()
  const { data: user } = useUserInfo()
  const logout = useAuthStore((s) => s.logout)
  const guest = isGuestUser(user, mode?.mode)

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <BrandMark size={28} />
          Agent 聊天
        </div>
        <div className={styles.spacer} />
        {guest ? (
          <span className={styles.badge}>体验模式</span>
        ) : (
          <button type="button" className={styles.headerBtn} onClick={() => { void Promise.resolve(navigate('/dashboard')) }}>
            <ArrowLeftIcon size={14} /> 返回管理
          </button>
        )}
        <span className={styles.username}>{user?.name}</span>
        <button type="button" className={styles.headerBtn} onClick={() => { void logout() }}>
          <SignOutIcon size={14} /> 退出
        </button>
        <ThemeControls />
      </header>
      <main className={styles.main}>
        {!isLoading && (agents ?? []).length === 0 ? (
          <Empty description={guest ? '暂无可体验的 Agent，请联系管理员开放' : '暂无 Agent'} />
        ) : (
          <div className={styles.grid}>
            {(agents ?? []).map((a) => (
              <button
                key={a.name}
                type="button"
                className={styles.card}
                onClick={() => { void Promise.resolve(navigate(`/agents/${encodeURIComponent(a.name ?? '')}/chat`)) }}
              >
                <div
                  className={styles.cardIcon}
                  style={{ background: a.config?.iconBgColor || t.inkLight, color: a.config?.iconColor || t.ink }}
                >
                  <ChatCircleDotsIcon size={20} weight="duotone" />
                </div>
                <div className={styles.cardTitle}>
                  {a.config?.title?.zh ?? a.config?.title?.en ?? a.name}
                </div>
                {(a.config?.description?.zh ?? a.config?.description?.en) && (
                  <div className={styles.cardDesc}>{a.config?.description?.zh ?? a.config?.description?.en}</div>
                )}
              </button>
            ))}
          </div>
        )}
      </main>
    </div>
  )
}
