import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChatCircleDotsIcon, ArrowLeftIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { ChatSession } from '@/api/chat'
import { tokens as tk } from '@/styles/tokens'
import SessionListPanel from './SessionListPanel'
import MessageViewer from './MessageViewer'

const useStyles = createStyles(({ css }) => ({
  chatView: css`
    display: flex;
    height: calc(100dvh - 52px);
    margin: -24px -32px -24px;
    padding-top: 0;
    box-sizing: border-box;
    background: ${tk.surface};
    animation: fadeIn 0.3s ease;
    @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
    @media (max-width: 768px) {
      height: calc(100dvh - 52px);
      margin: -16px -16px -16px;
      padding-top: 0;
      flex-direction: column;
    }
  `,
  emptyPane: css`
    flex: 1; display: flex; flex-direction: column; align-items: center;
    justify-content: center; gap: 12px;
    @media (max-width: 768px) {
      display: none;
    }
  `,
  emptyTitle: css`font-size: 16px; font-weight: 600; color: ${tk.text};`,
  emptyDesc: css`font-size: 13px; color: ${tk.textMuted};`,
  mobileBack: css`
    display: none;
    @media (max-width: 768px) {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 12px 16px;
      font-size: ${tk.textSm};
      font-weight: 500;
      color: ${tk.ink};
      background: transparent;
      border: none;
      border-bottom: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
      cursor: pointer;
      width: 100%;
      transition: background 0.15s;
      &:hover {
        background: ${tk.inkSubtle};
      }
    }
  `,
  mobilePane: css`
    display: flex;
    flex-direction: column;
    height: 100%;
  `,
  messageContainer: css`
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  `,
}))

export default function ChatViewPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const [selectedSession, setSelectedSession] = useState<ChatSession | null>(null)

  return (
    <div className={styles.chatView}>
      <SessionListPanel
        selectedId={selectedSession?.id ?? null}
        onSelect={setSelectedSession}
        hideOnMobile={!!selectedSession}
      />
      {selectedSession ? (
        <div className={styles.messageContainer}>
          <button
            type="button"
            className={styles.mobileBack}
            onClick={() => { setSelectedSession(null); }}
          >
            <ArrowLeftIcon size={16} />
            {t('chat.backToList')}
          </button>
          <MessageViewer session={selectedSession} />
        </div>
      ) : (
        <div className={styles.emptyPane}>
          <ChatCircleDotsIcon size={56} weight="thin" color={tk.textMuted} />
          <div className={styles.emptyTitle}>{t('chat.pickTitle')}</div>
          <div className={styles.emptyDesc}>{t('chat.pickDesc')}</div>
        </div>
      )}
    </div>
  )
}
