import { Modal, Tag } from 'antd'
import { createStyles } from 'antd-style'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import type { BulkOperation, ClassifiedItem, Classification } from './classifyBulkOperation'
import { tokens as t } from '@/styles/tokens'

const OP_LABEL: Record<BulkOperation, string> = {
  deploy: '部署',
  redeploy: '重新部署',
  stop: '停止',
  delete: '删除',
}

const useStyles = createStyles(({ css }) => ({
  group: css`
    margin-bottom: 12px;
    &:last-child { margin-bottom: 0; }
  `,
  groupTitle: css`
    font-size: 12px;
    font-weight: 600;
    color: ${t.text};
    margin-bottom: 6px;
  `,
  names: css`
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  `,
  nameTag: css`
    font-size: 11px;
    margin-inline-end: 0;
  `,
  reason: css`
    font-size: 11px;
    color: ${t.textTertiary};
  `,
}))

const GROUPS: { key: Classification; label: string; color: string }[] = [
  { key: 'executable', label: '可执行', color: 'green' },
  { key: 'skipped', label: '跳过', color: 'default' },
  { key: 'blocked', label: '受限', color: 'red' },
]

export interface BulkConfirmModalProps {
  open: boolean
  operation: BulkOperation
  /** 完整分类结果；确认后由父级整体传入任务（spec §6） */
  items: ClassifiedItem[]
  onCancel: () => void
  onConfirm: () => void
}

export default function BulkConfirmModal({ open, operation, items, onCancel, onConfirm }: BulkConfirmModalProps) {
  const { styles } = useStyles()
  const primaryStyles = usePrimaryButtonStyle()
  const executableCount = items.filter((i) => i.classification === 'executable').length
  const danger = operation === 'delete'

  return (
    <Modal
      title={`批量${OP_LABEL[operation]}`}
      open={open}
      onCancel={onCancel}
      cancelText="取消"
      okText={`${OP_LABEL[operation]} ${executableCount} 个`}
      okButtonProps={{
        disabled: executableCount === 0,
        // 删除操作用 antd danger 主按钮；其余操作注入统一主按钮样式（AGENTS.md）
        ...(danger ? { danger: true } : { className: primaryStyles.root }),
      }}
      onOk={onConfirm}
      width={480}
    >
      {GROUPS.map(({ key, label, color }) => {
        const group = items.filter((i) => i.classification === key)
        if (group.length === 0) return null
        return (
          <div key={key} className={styles.group}>
            <div className={styles.groupTitle}>
              <Tag color={color} className={styles.nameTag}>{label} · {group.length}</Tag>
            </div>
            <div className={styles.names}>
              {group.map((item) => (
                <Tag
                  key={item.agent.name}
                  className={styles.nameTag}
                  title={item.reason}
                >
                  {item.agent.config.title?.zh ?? item.agent.name}
                  {item.reason ? <span className={styles.reason}>（{item.reason}）</span> : null}
                </Tag>
              ))}
            </div>
          </div>
        )
      })}
    </Modal>
  )
}
