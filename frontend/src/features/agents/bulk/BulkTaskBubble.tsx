import { RocketIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { TaskPhase } from './useBulkAgentTask'
import { tokens as t } from '@/styles/tokens'

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
    background: ${t.surface};
    border: 1px solid var(--border);
    box-shadow: ${t.elevation2};
    cursor: pointer;
    font-size: 13px;
    font-weight: 600;
    color: ${t.text};
  `,
  dot: css`
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  `,
  dotGreen: css`
    background: ${t.success};
  `,
  dotRed: css`
    background: ${t.danger};
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
  const { styles } = useStyles()
  if (!visible) return null

  return (
    <div
      className={styles.bubble}
      data-testid="bulk-task-bubble"
      role="button"
      aria-label={`批量任务${phase === 'done' ? '已完成' : '进行中'}，点击查看`}
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
