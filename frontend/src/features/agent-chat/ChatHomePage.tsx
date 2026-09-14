import { useNavigate } from 'react-router'
import { useMemo } from 'react'
import { createStyles } from 'antd-style'
import { Empty } from 'antd'
import { ChatCircleDotsIcon } from '@phosphor-icons/react'
import type { Agent } from '@/api/agents'
import { usePublicAgents } from '@/queries/useAgents'
import { useAuthMode } from '@/features/login/useAuthMode'
import { useUserInfo } from '@/queries/useUserInfo'
import { isGuestUser } from '@/lib/auth-guest'
import ChatLayout from './ChatLayout'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  badge: css`
    padding: 2px 10px;
    border-radius: 999px;
    font-size: 12px;
    background: color-mix(in srgb, ${t.softAccent} 18%, transparent);
    color: ${t.ink};
  `,
  main: css`
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
  section: css`
    margin-bottom: 24px;
  `,
  sectionTitle: css`
    display: flex;
    align-items: center;
    gap: 8px;
    color: ${t.text};
    font-size: ${t.textBase};
    font-weight: 600;
    margin-bottom: 12px;
  `,
  sectionCount: css`
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 6px;
    border-radius: 10px;
    background: ${t.inkSubtle};
    color: ${t.ink};
    font-size: 12px;
    font-weight: 600;
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
  const guest = isGuestUser(user, mode?.mode)

  // 与管理页 Agent 列表同款分组：group 为空（nullish 或空串/空白串——DB 列默认
  // 空字符串，?? 不回退空串）归「默认分组」；组内按 name 排序；默认分组垫底。
  const groupedSections = useMemo(() => {
    const grouped = (agents ?? []).reduce<Record<string, Agent[]>>((acc, agent) => {
      const group = agent.group?.trim() ? agent.group : '默认分组'
      acc[group] ??= []
      acc[group].push(agent)
      return acc
    }, {})
    const sections = Object.entries(grouped).map(([name, list]) => ({
      name,
      agents: list.sort((a, b) => a.name.localeCompare(b.name))
    }))
    sections.sort((a, b) => {
      if (a.name === '默认分组') return 1
      if (b.name === '默认分组') return -1
      return a.name.localeCompare(b.name)
    })
    return sections
  }, [agents])

  return (
    <ChatLayout badge={guest ? <span className={styles.badge}>体验模式</span> : undefined}>
      <div className={styles.main}>
        {!isLoading && (agents ?? []).length === 0 ? (
          <Empty description={guest ? '暂无可体验的 Agent，请联系管理员开放' : '暂无 Agent'} />
        ) : (
          groupedSections.map((section) => (
            <section key={section.name} className={styles.section}>
              <div className={styles.sectionTitle}>
                <span>{section.name}</span>
                <span className={styles.sectionCount}>{section.agents.length}</span>
              </div>
              <div className={styles.grid}>
                {section.agents.map((a) => (
                  <button
                    key={a.name}
                    type="button"
                    className={styles.card}
                    onClick={() => { void Promise.resolve(navigate(`/agents/${encodeURIComponent(a.name)}/chat`)) }}
                  >
                    <div
                      className={styles.cardIcon}
                      style={{ background: a.config.iconBgColor ?? t.inkLight, color: a.config.iconColor ?? t.ink }}
                    >
                      <ChatCircleDotsIcon size={20} weight="duotone" />
                    </div>
                    <div className={styles.cardTitle}>
                      {a.config.title?.zh ?? a.config.title?.en ?? a.name}
                    </div>
                    {(a.config.description?.zh ?? a.config.description?.en) && (
                      /* eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Record<string, string> index is typed string but backend may omit zh/en; the guard above narrows description so the chains read as redundant — the nullish fallback is a real runtime path */
                      <div className={styles.cardDesc}>{a.config.description?.zh ?? a.config.description?.en}</div>
                    )}
                  </button>
                ))}
              </div>
            </section>
          ))
        )}
      </div>
    </ChatLayout>
  )
}
