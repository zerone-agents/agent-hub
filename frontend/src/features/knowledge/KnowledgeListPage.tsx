import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Spin, Popconfirm, Tooltip, Empty } from 'antd'
import NameSearch from '@/components/NameSearch'
import type { ColumnsType } from 'antd/es/table'
import { PlusIcon, PencilSimpleIcon, TrashIcon, DatabaseIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useNavigate } from 'react-router'
import { useKnowledgeList, useDeleteKnowledge } from '@/queries/useKnowledge'
import { useCanWrite } from '@/hooks/useCanWrite'
import type { KnowledgeDataset } from '@/api/knowledge'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import BorderedTable from '@/components/BorderedTable'
import KnowledgeForm from './KnowledgeForm'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @keyframes pageIn {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  pageHead: css`
    display: flex; justify-content: space-between; align-items: flex-start;
    margin-bottom: 24px;
    @media (max-width: 768px) { flex-direction: column; gap: 16px; }
  `,
  pageTitle: css`
    font-size: ${tk.text3xl}; font-weight: 700; color: ${tk.text};
    letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`
    margin-top: 4px; font-size: ${tk.textBase}; color: ${tk.textTertiary};
  `,
  toolbar: css`
    display: flex; justify-content: space-between; align-items: center;
    gap: 12px; margin-bottom: 16px;
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 80px 0;
  `,
  nameLink: css`
    color: ${tk.ink}; font-weight: 600; cursor: pointer;
    &:hover { text-decoration: underline; }
  `,
  actBtn: css`
    width: 30px; height: 30px; display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: ${tk.radiusSm}px;
    color: ${tk.textMuted}; cursor: pointer; transition: all 0.15s;
    &:hover { background: ${tk.inkSubtle}; color: ${tk.ink}; }
  `,
  actBtnDanger: css`
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${tk.danger}; }
  `
}))

const PAGE_SIZE = 10

export default function KnowledgeListPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const navigate = useNavigate()

  const [page, setPage] = useState(1)
  const [keywords, setKeywords] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<KnowledgeDataset | null>(null)

  const { data, isLoading } = useKnowledgeList({
    page,
    page_size: PAGE_SIZE,
    keywords,
    orderby: 'update_time',
    desc: true
  })
  const deleteKnowledge = useDeleteKnowledge()
  const canWrite = useCanWrite()

  const datasets = useMemo(() => {
    return (data?.datasets ?? []).sort((a, b) => a.name.localeCompare(b.name))
  }, [data?.datasets])
  const total = data?.total ?? 0

  const columns: ColumnsType<KnowledgeDataset> = [
    {
      title: t('knowledge.list.name'),
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (_, record) => (
        <span className={styles.nameLink} onClick={async () => { await navigate(`/knowledge/${record.id}`); }}>
          {record.name || t('knowledge.list.unnamed')}
        </span>
      )
    },
    {
      title: t('knowledge.list.desc'),
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (value: string) => (
        <Tooltip title={value} placement="topLeft">
          <span style={{ color: tk.textTertiary }}>{value || '-'}</span>
        </Tooltip>
      )
    },
    { title: t('knowledge.list.docNum'), dataIndex: 'doc_num', key: 'doc_num', width: 80, align: 'right' },
    { title: t('knowledge.list.chunkNum'), dataIndex: 'chunk_num', key: 'chunk_num', width: 80, align: 'right' },
    { title: t('knowledge.list.parser'), dataIndex: 'parser_id', key: 'parser_id', width: 110 },
    {
      title: t('knowledge.list.updatedAt'),
      key: 'update_time',
      width: 140,
      render: (_, record) => formatTime(record.update_time ?? record.update_date)
    },
    {
      title: t('knowledge.list.actions'),
      key: 'action',
      width: 100,
      fixed: 'right',
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 2 }}>
          {canWrite && (
            <>
              <button
                type="button"
                className={styles.actBtn}
                title={t('common.edit')}
                onClick={() => {
                  setEditing(record)
                  setFormOpen(true)
                }}
              >
                <PencilSimpleIcon size={14} />
              </button>
              <Popconfirm
                title={t('scenes.deleteConfirmTitle')}
                description={t('knowledge.list.deleteConfirm', { name: record.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => deleteKnowledge.mutateAsync(record.id)}
              >
                <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title={t('common.delete')}>
                  <TrashIcon size={14} />
                </button>
              </Popconfirm>
            </>
          )}
        </div>
      )
    }
  ]

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>{t('knowledge.list.pageTitle')}</div>
          <div className={styles.pageSub}>{t('knowledge.list.pageSub')}</div>
        </div>
        {canWrite && (
          <PrimaryButton
            icon={<PlusIcon size={16} weight="bold" />}
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            {t('knowledge.list.create')}
          </PrimaryButton>
        )}
      </div>

      <div className={styles.toolbar}>
        <NameSearch
          placeholder={t('knowledge.list.searchPlaceholder')}
          onSearch={(value) => {
            setKeywords(value)
            setPage(1)
          }}
        />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin size="medium" />
        </div>
      ) : (
        <BorderedTable<KnowledgeDataset>
          columns={columns}
          dataSource={datasets}
          rowKey="id"
          size="middle"
          scroll={{ x: 900 }}
          locale={{
            emptyText: (
              <Empty
                image={<DatabaseIcon size={48} color={tk.textMuted} />}
                description={keywords ? t('knowledge.list.emptyNoMatch') : t('knowledge.list.emptyNone')}
              />
            )
          }}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            showTotal: (n) => t('common.totalItems', { total: n }),
            onChange: (next) => { setPage(next); }
          }}
        />
      )}

      <KnowledgeForm open={formOpen} editing={editing} onClose={() => { setFormOpen(false); }} />
    </div>
  )
}
