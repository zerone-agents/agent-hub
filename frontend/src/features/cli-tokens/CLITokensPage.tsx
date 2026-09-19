import { useState } from 'react'
import { Button, Popconfirm, Spin } from 'antd'
import { PlusIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useTranslation } from 'react-i18next'
import PrimaryButton from '@/components/PrimaryButton'
import type { ColumnsType } from 'antd/es/table'
import { useCLITokens, useRevokeCLIToken } from '@/queries/useCLITokens'
import type { CLIToken } from '@/api/cli-tokens'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import BorderedTable from '@/components/BorderedTable'
import CreateTokenModal from './CreateTokenModal'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @keyframes pageIn { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: translateY(0); } }
  `,
  pageHead: css`
    display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 32px;
    @media (max-width: 768px) { flex-direction: column; gap: 16px; }
  `,
  pageTitle: css`
    font-size: ${tk.text3xl}; font-weight: 700; color: ${tk.text}; letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`margin-top: 4px; font-size: ${tk.textBase}; color: ${tk.textTertiary};`,
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`
}))

export default function CLITokensPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()

  // columns 的列名走 t()（i18n），必须在组件内构造——模块级常量拿不到 hook。
  const columns: ColumnsType<CLIToken> = [
    {
      title: t('cliTokens.columns.name'),
      dataIndex: 'name',
      key: 'name'
    },
    {
      title: t('cliTokens.columns.createdAt'),
      dataIndex: 'createdAt',
      key: 'createdAt',
      render: (v: string) => formatTime(v)
    },
    {
      title: t('cliTokens.columns.lastUsed'),
      dataIndex: 'lastUsedAt',
      key: 'lastUsedAt',
      render: (v: string | null | undefined) => (v ? formatTime(v) : t('cliTokens.neverUsed'))
    },
    {
      title: t('cliTokens.columns.expiresAt'),
      dataIndex: 'expiresAt',
      key: 'expiresAt',
      render: (v: string) => formatTime(v)
    }
  ]
  const { data: tokens = [], isLoading } = useCLITokens()
  const revokeToken = useRevokeCLIToken()
  const [modalOpen, setModalOpen] = useState(false)

  const actionColumn: ColumnsType<CLIToken>[0] = {
    title: t('cliTokens.columns.actions'),
    key: 'action',
    width: 100,
    render: (_: unknown, record: CLIToken) => (
      <Popconfirm
        title={t('cliTokens.revokeTitle')}
        description={t('cliTokens.revokeConfirm', { name: record.name })}
        okText={t('cliTokens.revoke')}
        okButtonProps={{ danger: true }}
        cancelText={t('common.cancel')}
        onConfirm={() => { revokeToken.mutate(record.id); }}
      >
        <Button type="link" danger size="small">
          {t('cliTokens.revoke')}
        </Button>
      </Popconfirm>
    )
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>CLI Tokens</div>
          <div className={styles.pageSub}>
            {t('cliTokens.pageSub')}
          </div>
        </div>
        <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={() => { setModalOpen(true); }}>
          {t('cliTokens.create')}
        </PrimaryButton>
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin size="medium" />
        </div>
      ) : (
        <BorderedTable<CLIToken>
          columns={[...columns, actionColumn]}
          dataSource={tokens}
          rowKey="id"
          pagination={false}
          locale={{ emptyText: t('cliTokens.empty') }}
        />
      )}

      <CreateTokenModal open={modalOpen} onClose={() => { setModalOpen(false); }} />
    </div>
  )
}
