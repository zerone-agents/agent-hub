import { Button, Modal, Progress, Tag } from 'antd'
import { createStyles } from 'antd-style'
import type { BulkOperation } from './classifyBulkOperation'
import type { BulkItemStatus, BulkTaskItem, BulkTaskSummary, TaskPhase } from './useBulkAgentTask'
import { tokens as t } from '@/styles/tokens'

const OP_LABEL: Record<BulkOperation, string> = {
  deploy: '部署',
  redeploy: '重新部署',
  stop: '停止',
  delete: '删除',
}

const STATUS_META: Record<BulkItemStatus, { label: string; color: string }> = {
  pending: { label: '等待', color: 'default' },
  running: { label: '执行中', color: 'processing' },
  succeeded: { label: '成功', color: 'success' },
  failed: { label: '失败', color: 'error' },
  skipped: { label: '跳过', color: 'warning' },
  blocked: { label: '受限', color: 'error' },
}

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
    font-size: 13px;
    color: ${t.text};
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
    border-radius: ${t.radiusSm}px;
    font-size: 12px;
  `,
  title: css`
    font-weight: 600;
    color: ${t.text};
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  name: css`
    font-family: ${t.fontMono};
    font-size: 11px;
    color: ${t.textTertiary};
  `,
  reason: css`
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: ${t.danger};
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
  const { styles } = useStyles()
  const settled = summary.succeeded + summary.failed + summary.skipped + summary.blocked
  const running = phase === 'running'
  const percent = summary.total === 0 ? 0 : Math.round((settled / summary.total) * 100)

  return (
    <Modal
      title={`批量${operation ? OP_LABEL[operation] : ''}进度`}
      open={open}
      onCancel={running ? onCollapse : onClose}
      closable
      mask={{ closable: false }}
      keyboard={false}
      width={560}
      footer={
        <Button
          type={running ? 'default' : 'primary'}
          onClick={running ? onCollapse : onClose}
        >
          {running ? '收起' : '关闭'}
        </Button>
      }
    >
      <div className={styles.head}>
        <span>{settled}/{summary.total}</span>
        <Progress percent={percent} size="small" style={{ flex: 1, margin: 0 }} />
        <span>
          成功 {summary.succeeded} · 失败 {summary.failed} · 跳过 {summary.skipped} · 受限 {summary.blocked}
        </span>
      </div>
      <div className={styles.list}>
        {items.map((item) => (
          <div key={item.name} className={styles.row}>
            <span className={styles.title}>{item.title}</span>
            <span className={styles.name}>{item.name}</span>
            <span className={styles.spacer} />
            {item.reason && <span className={styles.reason}>{item.reason}</span>}
            <Tag color={STATUS_META[item.status].color}>{STATUS_META[item.status].label}</Tag>
          </div>
        ))}
      </div>
    </Modal>
  )
}
