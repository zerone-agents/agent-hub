import { Button, Modal, Progress, Tag } from 'antd'
import { useTranslation } from 'react-i18next'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { BULK_OPERATION_LABEL } from './classifyBulkOperation'
import type { BulkOperation } from './classifyBulkOperation'
import type { BulkItemStatus, BulkTaskItem, BulkTaskSummary, TaskPhase } from './useBulkAgentTask'
import { tokens as tk } from '@/styles/tokens'

const STATUS_META: Record<BulkItemStatus, { label: string; color: string }> = {
  pending: { label: 'agents.bulk.taskPending', color: 'default' },
  running: { label: 'agents.bulk.taskRunning', color: 'processing' },
  succeeded: { label: 'agents.bulk.taskSucceeded', color: 'success' },
  failed: { label: 'agents.bulk.taskFailed', color: 'error' },
  skipped: { label: 'agents.bulk.taskSkipped', color: 'warning' },
  blocked: { label: 'agents.bulk.taskBlocked', color: 'error' },
}

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
    font-size: 13px;
    color: ${tk.text};
  `,
  list: css`
    max-height: 320px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
  `,
  row: css`
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${tk.radiusSm}px;
    font-size: 12px;
  `,
  title: css`
    font-weight: 600;
    color: ${tk.text};
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  name: css`
    font-family: ${tk.fontMono};
    font-size: 11px;
    color: ${tk.textTertiary};
  `,
  reason: css`
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: ${tk.danger};
    font-size: 11px;
    text-align: right;
  `,
  spacer: css`
    flex: 1;
  `,
}))

export interface BulkTaskModalProps {
  open: boolean
  phase: TaskPhase
  operation: BulkOperation | null
  items: BulkTaskItem[]
  summary: BulkTaskSummary
  onCollapse: () => void
  onClose: () => void
}

export default function BulkTaskModal({
  open, phase, operation, items, summary, onCollapse, onClose,
}: BulkTaskModalProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const settled = summary.succeeded + summary.failed + summary.skipped + summary.blocked
  const running = phase === 'running'
  const percent = summary.total === 0 ? 0 : Math.round((settled / summary.total) * 100)

  return (
    <Modal
      title={t('agents.bulk.progressTitle', { op: operation ? t(BULK_OPERATION_LABEL[operation]) : '' })}
      open={open}
      onCancel={running ? onCollapse : onClose}
      closable
      mask={{ closable: false }}
      keyboard={false}
      width={560}
      footer={
        running ? (
          <Button onClick={onCollapse}>{t('agents.bulk.collapse')}</Button>
        ) : (
          <PrimaryButton onClick={onClose}>{t('agents.bulk.close')}</PrimaryButton>
        )
      }
    >
      <div className={styles.head}>
        <span>{settled}/{summary.total}</span>
        <Progress percent={percent} size="small" style={{ flex: 1, margin: 0 }} />
        <span>
          {t('agents.bulk.summary', { succeeded: summary.succeeded, failed: summary.failed, skipped: summary.skipped, blocked: summary.blocked })}
        </span>
      </div>
      <div className={styles.list}>
        {items.map((item) => (
          <div key={item.name} className={styles.row}>
            <span className={styles.title}>{item.title}</span>
            <span className={styles.name}>{item.name}</span>
            <span className={styles.spacer} />
            {item.reason && <span className={styles.reason}>{item.reason}</span>}
            <Tag color={STATUS_META[item.status].color}>{t(STATUS_META[item.status].label)}</Tag>
          </div>
        ))}
      </div>
    </Modal>
  )
}
