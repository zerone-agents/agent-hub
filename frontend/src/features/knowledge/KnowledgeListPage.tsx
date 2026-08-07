import { useState, useMemo } from 'react'
import { Spin, Popconfirm, Tooltip, Empty } from 'antd'
import NameSearch from '@/components/NameSearch'
import type { ColumnsType } from 'antd/es/table'
import { PlusIcon, PencilSimpleIcon, TrashIcon, DatabaseIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useNavigate } from 'react-router-dom'
import { useKnowledgeList, useDeleteKnowledge } from '@/queries/useKnowledge'
import type { KnowledgeDataset } from '@/api/knowledge'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
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
    font-size: ${t.text3xl}; font-weight: 700; color: ${t.text};
    letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`
    margin-top: 4px; font-size: ${t.textBase}; color: ${t.textTertiary};
  `,
  toolbar: css`
    display: flex; justify-content: space-between; align-items: center;
    gap: 12px; margin-bottom: 16px;
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 80px 0;
  `,
  nameLink: css`
    color: ${t.ink}; font-weight: 600; cursor: pointer;
    &:hover { text-decoration: underline; }
  `,
  actBtn: css`
    width: 30px; height: 30px; display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: ${t.radiusSm}px;
    color: ${t.textMuted}; cursor: pointer; transition: all 0.15s;
    &:hover { background: ${t.inkSubtle}; color: ${t.ink}; }
  `,
  actBtnDanger: css`
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${t.danger}; }
  `
}))

const PAGE_SIZE = 10

export default function KnowledgeListPage() {
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

  const datasets = useMemo(() => {
    return (data?.datasets ?? []).sort((a, b) => a.name.localeCompare(b.name))
  }, [data?.datasets])
  const total = data?.total ?? 0

  const columns: ColumnsType<KnowledgeDataset> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (_, record) => (
        <span className={styles.nameLink} onClick={() => { navigate(`/knowledge/${record.id}`); }}>
          {record.name || '未命名'}
        </span>
      )
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (value: string) => (
        <Tooltip title={value} placement="topLeft">
          <span style={{ color: t.textTertiary }}>{value || '-'}</span>
        </Tooltip>
      )
    },
    { title: '文档数', dataIndex: 'doc_num', key: 'doc_num', width: 80, align: 'right' },
    { title: '分块数', dataIndex: 'chunk_num', key: 'chunk_num', width: 80, align: 'right' },
    { title: '解析方法', dataIndex: 'parser_id', key: 'parser_id', width: 110 },
    {
      title: '更新时间',
      key: 'update_time',
      width: 140,
      render: (_, record) => formatTime(record.update_time ?? record.update_date)
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      fixed: 'right',
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 2 }}>
          <button
            type="button"
            className={styles.actBtn}
            title="编辑"
            onClick={() => {
              setEditing(record)
              setFormOpen(true)
            }}
          >
            <PencilSimpleIcon size={14} />
          </button>
          <Popconfirm
            title="确认删除？"
            description={`删除知识库 "${record.name}"？此操作不可撤销。`}
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => { deleteKnowledge.mutate(record.id); }}
          >
            <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title="删除">
              <TrashIcon size={14} />
            </button>
          </Popconfirm>
        </div>
      )
    }
  ]

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>知识库管理</div>
          <div className={styles.pageSub}>管理知识库、文档、分块与检索测试</div>
        </div>
        <PrimaryButton
          icon={<PlusIcon size={16} weight="bold" />}
          onClick={() => {
            setEditing(null)
            setFormOpen(true)
          }}
        >
          新建知识库
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
        <NameSearch
          placeholder="搜索知识库名称"
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
                image={<DatabaseIcon size={48} color={t.textMuted} />}
                description={keywords ? '未找到匹配的知识库' : '还没有知识库，点击右上角新建'}
              />
            )
          }}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            showTotal: (n) => `共 ${n} 条`,
            onChange: (next) => { setPage(next); }
          }}
        />
      )}

      <KnowledgeForm open={formOpen} editing={editing} onClose={() => { setFormOpen(false); }} />
    </div>
  )
}
