import { RocketIcon } from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'
import { createStyles } from 'antd-style'
import type { TaskPhase } from './useBulkAgentTask'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  bubble: css`
    position: fixed;
    right: 24px;
    bottom: 24px;
    z-index: 1000;
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 10px 16px;
    border-radius: 999px;
    background: ${tk.surface};
    border: 1px solid var(--border);
    box-shadow: ${tk.elevation2};
    cursor: pointer;
    font-size: 13px;
    font-weight: 600;
    color: ${tk.text};
  `,
  dot: css`
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  `,
  dotGreen: css`
    background: ${tk.success};
  `,
  dotRed: css`
    background: ${tk.danger};
  `,
}))

export interface BulkTaskBubbleProps {
  /** presentation === 'collapsed' 时可见 */
  visible: boolean
  phase: TaskPhase
  /** 执行中显示 `${settled}/${total}` */
  progressText: string
  /** 完成态：红点 = failed/blocked 任一非零；绿点 = 其余（spec §4.4） */
  dot: 'green' | 'red' | null
  onClick: () => void
}

export default function BulkTaskBubble({ visible, phase, progressText, dot, onClick }: BulkTaskBubbleProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  if (!visible) return null

  return (
    <div
      className={styles.bubble}
      data-testid="bulk-task-bubble"
      role="button"
      aria-label={t('agents.bulk.bubbleAria', { state: phase === 'done' ? t('agents.bulk.bubbleDone') : t('agents.bulk.bubbleRunning') })}
      onClick={onClick}
    >
      {phase === 'running' ? (
        <>
          <RocketIcon size={16} weight="fill" />
          <span>{progressText}</span>
        </>
      ) : dot === 'red' ? (
        <span className={`${styles.dot} ${styles.dotRed}`} data-testid="bulk-bubble-dot-red" />
      ) : (
        <span className={`${styles.dot} ${styles.dotGreen}`} data-testid="bulk-bubble-dot-green" />
      )}
    </div>
  )
}
