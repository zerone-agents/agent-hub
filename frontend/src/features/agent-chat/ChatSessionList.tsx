import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Popconfirm } from 'antd'
import { PlusIcon, TrashIcon, ChatCircleDotsIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { AgentChatSession } from '@/api/agent-chat'
import type { ApiEnvelope } from '@/api/client'
import {
  useAgentChatSessions,
  useCreateAgentChatSession,
  useDeleteAgentChatSession
} from '@/queries/useAgentChat'
import PrimaryButton from '@/components/PrimaryButton'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  sidebar: css`
    width: 280px;
    border-right: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    height: 100%;
    @media (max-width: 768px) {
      width: 100%;
      border-right: none;
    }
  `,
  sidebarHiddenMobile: css`
    @media (max-width: 768px) {
      display: none;
    }
  `,
  head: css`
    padding: 16px 8px 8px;
  `,
  title: css`
    font-size: 14px;
    font-weight: 600;
    color: ${tk.text};
    margin: 0 0 12px;
  `,
  list: css`
    flex: 1;
    overflow-y: auto;
    padding: 8px 8px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  `,
  empty: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 40px 0;
    color: ${tk.textMuted};
    font-size: 12px;
  `,
  item: css`
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 10px 12px;
    border-radius: ${tk.radiusSm}px;
    cursor: pointer;
    transition: background 0.12s;
    &:hover {
      background: ${tk.surfaceHover};
    }
  `,
  itemActive: css`
    background: ${tk.surfaceHover};
  `,
  body: css`
    flex: 1;
    min-width: 0;
  `,
  itemTitle: css`
    font-size: 13px;
    font-weight: 500;
    color: ${tk.text};
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  `,
  meta: css`
    font-size: 11px;
    color: ${tk.textMuted};
    margin-top: 2px;
  `,
  delBtn: css`
    flex-shrink: 0;
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: 3px;
    color: ${tk.textMuted};
    cursor: pointer;
    &:hover {
      background: rgba(220, 38, 38, 0.06);
      color: ${tk.danger};
    }
  `
}))

interface ChatSessionListProps {
  agentName: string
  selectedId: string | null
  onSelect: (s: AgentChatSession) => void
  onDeleted?: (id: string) => void
  streamingSessionId: string | null
  hideOnMobile?: boolean
}

export default function ChatSessionList({
  agentName,
  selectedId,
  onSelect,
  onDeleted,
  streamingSessionId,
  hideOnMobile
}: ChatSessionListProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { data } = useAgentChatSessions(agentName)
  const createSession = useCreateAgentChatSession(agentName)
  const deleteSession = useDeleteAgentChatSession(agentName)
  const [creating, setCreating] = useState(false)

  const sessions = (data?.items ?? [])

  const handleNew = async () => {
    setCreating(true)
    try {
      const res = await createSession.mutateAsync(undefined)
      const sess = (res.data as ApiEnvelope<AgentChatSession>).data
      if (sess) onSelect(sess)
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className={`${styles.sidebar} ${hideOnMobile ? styles.sidebarHiddenMobile : ''}`}>
      <div className={styles.head}>
        <h2 className={styles.title}>{t('agentChat.sessions')}</h2>
        <PrimaryButton
          icon={<PlusIcon size={14} weight="bold" />}
          loading={creating}
          disabled={!!streamingSessionId}
          onClick={handleNew}
          style={{ width: '100%' }}
        >
          {t('agentChat.newSession')}
        </PrimaryButton>
      </div>
      <div className={styles.list}>
        {sessions.length === 0 ? (
          <div className={styles.empty}>
            <ChatCircleDotsIcon size={32} weight="thin" color={tk.textMuted} />
            <span>{t('agentChat.noSessions')}</span>
          </div>
        ) : (
          sessions.map((s) => {
            const rowDisabled = !!streamingSessionId && s.id !== streamingSessionId
            return (
            <div
              key={s.id}
              className={`${styles.item} ${selectedId === s.id ? styles.itemActive : ''}`}
              style={rowDisabled ? { opacity: 0.4, pointerEvents: 'none' } : undefined}
              onClick={() => { if (!rowDisabled) onSelect(s) }}
            >
              <div className={styles.body}>
                <div className={styles.itemTitle}>{s.title || t('agentChat.sessionFallback')}</div>
                <div className={styles.meta}>{formatTime(s.updated_at)}</div>
              </div>
              {!streamingSessionId && (
                <Popconfirm
                  title={t('scenes.deleteConfirmTitle')}
                  okText={t('common.delete')}
                  okButtonProps={{ danger: true }}
                  cancelText={t('common.cancel')}
                  onConfirm={(e) => {
                    e?.stopPropagation()
                    deleteSession.mutate(s.id, {
                      onSuccess: () => onDeleted?.(s.id)
                    })
                  }}
                >
                  <button
                    type="button"
                    className={styles.delBtn}
                    title={t('common.delete')}
                    onClick={(e) => { e.stopPropagation(); }}
                  >
                    <TrashIcon size={13} />
                  </button>
                </Popconfirm>
              )}
            </div>
            )
          })
        )}
      </div>
    </div>
  )
}
