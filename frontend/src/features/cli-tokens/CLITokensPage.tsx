import { useState } from 'react'
import { Button, Popconfirm, Spin } from 'antd'
import { Plus } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { ColumnsType } from 'antd/es/table'
import { useCLITokens, useRevokeCLIToken } from '@/queries/useCLITokens'
import type { CLIToken } from '@/api/cli-tokens'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
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
    font-size: ${t.text3xl}; font-weight: 700; color: ${t.text}; letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`margin-top: 4px; font-size: ${t.textBase}; color: ${t.textTertiary};`,
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`
}))

const columns: ColumnsType<CLIToken> = [
  {
    title: '名称',
    dataIndex: 'name',
    key: 'name'
  },
  {
    title: '创建时间',
    dataIndex: 'createdAt',
    key: 'createdAt',
    render: (v: string) => formatTime(v)
  },
  {
    title: '最后使用',
    dataIndex: 'lastUsedAt',
    key: 'lastUsedAt',
    render: (v: string | null | undefined) => (v ? formatTime(v) : '从未使用')
  },
  {
    title: '过期时间',
    dataIndex: 'expiresAt',
    key: 'expiresAt',
    render: (v: string) => formatTime(v)
  }
]

export default function CLITokensPage() {
  const { styles } = useStyles()
  const { data: tokens = [], isLoading } = useCLITokens()
  const revokeToken = useRevokeCLIToken()
  const [modalOpen, setModalOpen] = useState(false)

  const actionColumn: ColumnsType<CLIToken>[0] = {
    title: '操作',
    key: 'action',
    width: 100,
    render: (_: unknown, record: CLIToken) => (
      <Popconfirm
        title="确认撤销？"
        description={`撤销 "${record.name}" 后，使用该 Token 的 CLI 将无法继续认证。`}
        okText="撤销"
        okButtonProps={{ danger: true }}
        cancelText="取消"
        onConfirm={() => { revokeToken.mutate(record.id); }}
      >
        <Button type="link" danger size="small">
          撤销
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
            管理 CLI 认证 Token，用于 zhub CLI 工具的长期身份验证
          </div>
        </div>
        <PrimaryButton icon={<Plus size={16} weight="bold" />} onClick={() => { setModalOpen(true); }}>
          创建 CLI Token
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
          locale={{ emptyText: '暂无 Token，点击上方按钮创建' }}
        />
      )}

      <CreateTokenModal open={modalOpen} onClose={() => { setModalOpen(false); }} />
    </div>
  )
}
