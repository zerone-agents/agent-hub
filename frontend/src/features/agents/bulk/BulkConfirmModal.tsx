import { Modal, Tag } from 'antd'
import { useTranslation } from 'react-i18next'
import { createStyles } from 'antd-style'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { BULK_OPERATION_LABEL } from './classifyBulkOperation'
import type { BulkOperation, ClassifiedItem, Classification } from './classifyBulkOperation'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  group: css`
    margin-bottom: 12px;
    &:last-child { margin-bottom: 0; }
  `,
  groupTitle: css`
    font-size: 12px;
    font-weight: 600;
    color: ${tk.text};
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
    color: ${tk.textTertiary};
  `,
}))

const GROUPS: { key: Classification; label: string; color: string }[] = [
  { key: 'executable', label: 'agents.bulk.clsExecutable', color: 'green' },
  { key: 'skipped', label: 'agents.bulk.clsSkipped', color: 'default' },
  { key: 'blocked', label: 'agents.bulk.clsBlocked', color: 'red' },
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
  const { t } = useTranslation()
  const { styles } = useStyles()
  const primaryStyles = usePrimaryButtonStyle()
  const executableCount = items.filter((i) => i.classification === 'executable').length
  const danger = operation === 'delete'

  return (
    <Modal
      title={t('agents.bulk.confirmTitle', { op: t(BULK_OPERATION_LABEL[operation]) })}
      open={open}
      onCancel={onCancel}
      cancelText={t('common.cancel')}
      okText={t('agents.bulk.confirmOk', { op: t(BULK_OPERATION_LABEL[operation]), n: executableCount })}
      okButtonProps={{
        disabled: executableCount === 0,
        // 统一注入共享主按钮样式（AGENTS.md）；删除操作叠加 danger（review S2：两者都保留）
        className: primaryStyles.root,
        ...(danger ? { danger: true } : {}),
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
              <Tag color={color} className={styles.nameTag}>{t(label)} · {group.length}</Tag>
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
