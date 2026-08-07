import { Robot, Warning } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { ContentPart } from '@/features/chat/parts'
import { ContentParts } from '@/features/chat/parts'
import { tokens as t } from '@/styles/tokens'
import type { StreamPhase } from './useChatStream'

const useStyles = createStyles(({ css }) => ({
  row: css`
    display: flex; gap: 10px; margin-bottom: 20px;
  `,
  avatar: css`
    width: 28px; height: 28px; border-radius: 50%; flex-shrink: 0;
    display: flex; align-items: center; justify-content: center;
    color: var(--primary-foreground); background: #059669;
  `,
  content: css`flex: 1; min-width: 0;`,
  role: css`
    font-size: 11px; font-weight: 600; color: ${t.textTertiary};
    margin-bottom: 3px;
  `,
  bubble: css`
    background: ${t.surface}; border-radius: 0 8px 8px 8px;
    padding: 12px 14px; box-shadow: ${t.elevation1};
    display: inline-block; max-width: 100%;
    min-height: 24px;
  `,
  typing: css`
    display: inline-flex; gap: 4px; padding: 4px 0;
    span {
      width: 6px; height: 6px; border-radius: 50%; background: ${t.textMuted};
      animation: blink 1.2s infinite ease-in-out both;
    }
    span:nth-child(2) { animation-delay: 0.2s; }
    span:nth-child(3) { animation-delay: 0.4s; }
    @keyframes blink { 0%, 80%, 100% { opacity: 0.2; } 40% { opacity: 1; } }
  `,
  error: css`
    display: flex; align-items: center; gap: 8px;
    color: ${t.danger}; font-size: 13px;
    background: rgba(220, 38, 38, 0.06);
    border-left: 2px solid ${t.danger};
    padding: 8px 12px; border-radius: 4px;
  `,
}))

interface StreamingMessageProps {
  parts: ContentPart[]
  phase: StreamPhase
  error?: string | null
}

export default function StreamingMessage({ parts, phase, error }: StreamingMessageProps) {
  const { styles } = useStyles()

  return (
    <div className={styles.row}>
      <div className={styles.avatar}>
        <Robot size={14} weight="bold" />
      </div>
      <div className={styles.content}>
        <div className={styles.role}>助手</div>
        <div className={styles.bubble}>
          {phase === 'error' ? (
            <div className={styles.error}>
              <Warning size={14} weight="bold" />
              <span>回复失败：{error || '未知错误'}</span>
            </div>
          ) : parts.length > 0 ? (
            <ContentParts parts={parts} />
          ) : (
            (phase === 'sending' || phase === 'streaming') ? (
              <div className={styles.typing}>
                <span></span><span></span><span></span>
              </div>
            ) : null
          )}
        </div>
      </div>
    </div>
  )
}
