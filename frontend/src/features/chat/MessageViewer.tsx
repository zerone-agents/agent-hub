import { useEffect, useRef } from 'react'
import { Spin, Pagination } from 'antd'
import { createStyles } from 'antd-style'
import type { ChatSession } from '@/api/chat'
import { useChatMessages } from '@/queries/useChat'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
import MessageBubble from './MessageBubble'

const useStyles = createStyles(({ css }) => ({
  main: css`
    flex: 1; display: flex; flex-direction: column; min-width: 0;
    height: 100%;
    overflow: hidden;
  `,
  empty: css`
    flex: 1; display: flex; flex-direction: column; align-items: center;
    justify-content: center; gap: 12px; color: ${t.textMuted};
  `,
  emptyTitle: css`font-size: 16px; font-weight: 600; color: ${t.text};`,
  header: css`
    padding: 16px 24px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
    display: flex; justify-content: space-between; align-items: flex-start;
    flex-shrink: 0;
  `,
  headerInfo: css`flex: 1; min-width: 0;`,
  headerTitle: css`
    font-size: 16px; font-weight: 600; color: ${t.text};
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    letter-spacing: -0.01em;
  `,
  metaRow: css`display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px;`,
  chip: css`
    display: inline-block; padding: 1px 8px; border-radius: 3px;
    font-size: 11px; background: ${t.inkSubtle}; color: ${t.textSecondary};
    font-family: ${t.fontMono};
  `,
  headerTime: css`font-size: 12px; color: ${t.textMuted}; flex-shrink: 0; margin-left: 16px;`,
  messages: css`
    flex: 1; overflow-y: auto; padding: 24px;
  `,
  msgLoading: css`display: flex; justify-content: center; padding: 40px 0;`,
  msgEmpty: css`text-align: center; padding: 40px 0; color: ${t.textMuted}; font-size: 13px;`,
  pagination: css`
    display: flex; justify-content: center; padding: 12px 0;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent); flex-shrink: 0;
  `
}))

interface MessageViewerProps {
  session: ChatSession
}

export default function MessageViewer({ session }: MessageViewerProps) {
  const { styles } = useStyles()
  const { data, isLoading, page, pageSize, setPage } = useChatMessages(session.id)
  const scrollRef = useRef<HTMLDivElement>(null)

  const messages = data?.items ?? []
  const total = data?.total ?? 0

  // Scroll to top when messages change (same as Vue version)
  useEffect(() => {
    if (scrollRef.current && !isLoading) {
      scrollRef.current.scrollTop = 0
    }
  }, [isLoading, session.id])

  return (
    <div className={styles.main}>
      {/* Header */}
      <div className={styles.header}>
        <div className={styles.headerInfo}>
          <div className={styles.headerTitle}>{session.title || '未命名会话'}</div>
          <div className={styles.metaRow}>
            {session.user_id && (
              <span className={styles.chip}>
                {session.display_name || session.user_name || session.user_id.slice(0, 8)}
              </span>
            )}
            {session.model && <span className={styles.chip}>{session.model}</span>}
            {session.agent_id && <span className={styles.chip}>{session.agent_id}</span>}
            {session.status && <span className={styles.chip}>{session.status}</span>}
          </div>
        </div>
        <span className={styles.headerTime}>{formatTime(session.updated_at)}</span>
      </div>

      {/* Messages */}
      <div className={styles.messages} ref={scrollRef}>
        {isLoading ? (
          <div className={styles.msgLoading}><Spin size="medium" /></div>
        ) : messages.length === 0 ? (
          <div className={styles.msgEmpty}>该会话暂无消息</div>
        ) : (
          messages.map((msg) => <MessageBubble key={msg.id} message={msg} />)
        )}
      </div>

      {/* Pagination */}
      {total > pageSize && (
        <div className={styles.pagination}>
          <Pagination
            current={page}
            total={total}
            pageSize={pageSize}
            size="small"
            simple
            onChange={(p) => { setPage(p); }}
          />
        </div>
      )}
    </div>
  )
}
