import { Button } from 'antd'
import { RocketIcon, ArrowClockwiseIcon, StopIcon, TrashIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { BulkOperation } from './classifyBulkOperation'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  // bulkBar 样式沿用知识库文档页既有多选栏视觉
  bulkBar: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid color-mix(in srgb, var(--foreground) 12%, transparent);
    border-radius: ${t.radius}px;
    background: linear-gradient(
      90deg,
      color-mix(in srgb, var(--foreground) 6%, transparent),
      rgba(5, 150, 105, 0.06)
    );
    @media (max-width: 768px) {
      flex-direction: column;
      align-items: stretch;
    }
  `,
  left: css`
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  `,
  count: css`
    font-size: 13px;
    font-weight: 600;
    color: ${t.text};
  `,
  right: css`
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
  `,
}))

const OPERATIONS: { op: BulkOperation; label: string; icon: React.ReactNode; danger?: boolean }[] = [
  { op: 'deploy', label: '部署', icon: <RocketIcon size={14} /> },
  { op: 'redeploy', label: '重新部署', icon: <ArrowClockwiseIcon size={14} /> },
  { op: 'stop', label: '停止', icon: <StopIcon size={14} /> },
  { op: 'delete', label: '删除', icon: <TrashIcon size={14} />, danger: true },
]

export interface BulkActionBarProps {
  selectedCount: number
  /** 待更新 agent 数（0 时禁用「全选待更新」） */
  pendingUpdateCount: number
  onSelectAll: () => void
  onSelectPendingUpdates: () => void
  onClear: () => void
  onOperation: (op: BulkOperation) => void
  onExit: () => void
  /** 批次执行中：禁用预检与批量操作（单批次约束，spec §6） */
  operationsDisabled: boolean
  /** 正在预检的操作（对应按钮 loading） */
  prechecking: BulkOperation | null
}

export default function BulkActionBar({
  selectedCount, pendingUpdateCount,
  onSelectAll, onSelectPendingUpdates, onClear, onOperation, onExit,
  operationsDisabled, prechecking,
}: BulkActionBarProps) {
  const { styles } = useStyles()
  const noSelection = selectedCount === 0

  return (
    <div className={styles.bulkBar} data-testid="bulk-action-bar">
      <div className={styles.left}>
        <span className={styles.count}>已选 {selectedCount} 个</span>
        <Button type="link" size="small" onClick={onSelectAll}>全选</Button>
        <Button type="link" size="small" disabled={pendingUpdateCount === 0} onClick={onSelectPendingUpdates}>
          全选待更新
        </Button>
        <Button type="link" size="small" disabled={noSelection} onClick={onClear}>清空</Button>
      </div>
      <div className={styles.right}>
        {OPERATIONS.map(({ op, label, icon, danger }) => (
          <Button
            key={op}
            size="small"
            danger={danger}
            icon={icon}
            // 任一预检进行中禁用全部操作，防止多个预检竞争覆盖确认内容（review P1）
            disabled={noSelection || operationsDisabled || prechecking !== null}
            loading={prechecking === op}
            onClick={() => { onOperation(op); }}
          >
            {label}
          </Button>
        ))}
        <Button size="small" onClick={onExit}>退出</Button>
      </div>
    </div>
  )
}
