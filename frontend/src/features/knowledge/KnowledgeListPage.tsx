import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Spin, Popconfirm, Tooltip, Empty, Select, Space, Modal } from 'antd'
import NameSearch from '@/components/NameSearch'
import type { ColumnsType } from 'antd/es/table'
import { PlusIcon, PencilSimpleIcon, TrashIcon, DatabaseIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { Link, useNavigate } from 'react-router'
import { useKnowledgeList, useDeleteKnowledgeSelection } from '@/queries/useKnowledge'
import { useCanWrite } from '@/hooks/useCanWrite'
import type { KnowledgeDataset, KnowledgeWriteOwner } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { getAccessToken, parseApiError } from '@/api/client'
import { queryClient } from '@/lib/query-client'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import BorderedTable from '@/components/BorderedTable'
import KnowledgeForm from './KnowledgeForm'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @media (prefers-reduced-motion: reduce) { animation: none; }
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
    gap: 12px; margin-bottom: 16px; flex-wrap: wrap;
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 80px 0;
  `,
  nameLink: css`
    color: ${tk.ink}; font-weight: 600; cursor: pointer;
    &:hover { text-decoration: underline; }
    &:focus-visible { outline: 2px solid ${tk.ink}; outline-offset: 3px; }
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
  const [sort, setSort] = useState('updated')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<KnowledgeDataset | null>(null)
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [deleteError, setDeleteError] = useState('')
  const [confirmationIds, setConfirmationIds] = useState<string[]>([])
  const [deleting, setDeleting] = useState(false)
  const operation = useRef<AbortController | null>(null)
  const selectionOrigin = useRef<{ id: string; token: string | null } | null>(null)
  const confirmationOrigin = useRef<{ id: string; token: string | null } | null>(null)
  const rowOrigin = useRef<{ id: string; token: string | null } | null>(null)
  const accountId = useAuthStore((s) => s.user?.id)

  useEffect(() => {
    const unsubscribe = useAuthStore.subscribe((state, previous) => {
      if (state.user?.id !== previous.user?.id || state.user?.role !== previous.user?.role) {
        operation.current?.abort()
        selectionOrigin.current = null
        confirmationOrigin.current = null
        rowOrigin.current = null
        setSelectedIds([])
        setDeleteError('')
        setConfirmationIds([])
      }
    })
    return () => { unsubscribe(); operation.current?.abort() }
  }, [])

  const { data, origin: listOrigin, isLoading, isError, refetch } = useKnowledgeList({
    page,
    page_size: PAGE_SIZE,
    keywords,
    orderby: 'update_time',
    desc: sort === 'updated'
  }, { owned: true })
  const deleteKnowledge = useDeleteKnowledgeSelection()
  const canWrite = useCanWrite()

  const datasets = data?.datasets ?? []
  const total = data?.total ?? 0
  const selected = datasets.filter((dataset) => selectedIds.includes(dataset.id))
  const busy = deleting || deleteKnowledge.isPending
  const clearSelection = () => { setSelectedIds([]); setDeleteError(''); setConfirmationIds([]); selectionOrigin.current = null; confirmationOrigin.current = null }
  const originIsCurrent = (origin: { id: string; token: string | null } | null) => Boolean(origin && origin.id === accountId && origin.id === useAuthStore.getState().user?.id && origin.token === getAccessToken())
  const listIsCurrent = () => Boolean(listOrigin && originIsCurrent(listOrigin) && ['admin', 'maintainer'].includes(listOrigin.role ?? ''))
  const rejectOldSelection = () => { clearSelection(); setDeleteError(t('knowledge.list.deleteOwnerChanged')) }

  const removeSelected = async (ids: string[], origin: { id: string; token: string | null } | null) => {
    if (!canWrite || busy || operation.current || !ids.length || ids.some((id) => !datasets.some((dataset) => dataset.id === id))) return
    if (!listIsCurrent() || !origin || !originIsCurrent(origin)) { rejectOldSelection(); return }
    const user = useAuthStore.getState().user
    if (!user || user.id !== accountId || !['admin', 'maintainer'].includes(user.role ?? '')) return
    const controller = new AbortController()
    operation.current = controller
    let token = origin.token
    const current = () => !controller.signal.aborted && useAuthStore.getState().user?.id === user.id
      && ['admin', 'maintainer'].includes(useAuthStore.getState().user?.role ?? '')
    const owner: KnowledgeWriteOwner = {
      signal: controller.signal,
      isCurrent: () => current() && getAccessToken() === token,
      assertCurrent: (refresh) => {
        if (refresh && current()) token = getAccessToken()
        if (!current() || getAccessToken() !== token) throw new Error(t('knowledge.list.deleteOwnerChanged'))
      },
    }
    const unsubscribeAuth = useAuthStore.subscribe((state) => {
      if (state.user?.id !== user.id || !['admin', 'maintainer'].includes(state.user.role ?? '')) controller.abort()
    })
    const unsubscribeCache = queryClient.getQueryCache().subscribe((event) => {
      const key = event.query.queryKey as readonly unknown[]
      if (event.type === 'removed' && key[0] === 'userinfo') controller.abort()
    })
    setDeleting(true)
    setDeleteError('')
    try {
      const result = await deleteKnowledge.mutateAsync({ ids: [...ids], owner })
      owner.assertCurrent()
      setConfirmationIds([])
      setSelectedIds((previous) => previous.filter((id) => !result.deletedIds.includes(id)))
      if (result.failed.length) setDeleteError(t('knowledge.list.batchFailed', { count: result.failed.length }) + ' ' + result.failed.map(({ id, error }) => `${datasets.find((dataset) => dataset.id === id)?.name ?? id}: ${parseApiError(error)}`).join('; '))
      if (result.deletedIds.length && result.deletedIds.length === datasets.length && page > 1) { setPage(page - 1); clearSelection() }
    } catch (error) {
      if (owner.isCurrent()) setDeleteError(parseApiError(error))
    } finally {
      unsubscribeAuth(); unsubscribeCache()
      if (operation.current === controller) { operation.current = null; setDeleting(false) }
    }
  }

  const columns: ColumnsType<KnowledgeDataset> = [
    {
      title: t('knowledge.list.name'),
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (_, record) => (
        <Link className={styles.nameLink} to={`/knowledge/${record.id}`}>
          {record.name || t('knowledge.list.unnamed')}
        </Link>
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
    { title: t('knowledge.list.parser'), dataIndex: 'parser_id', key: 'parser_id', width: 140, render: (value: string) => t(`knowledge.parsers.${value}`, { defaultValue: value }) },
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
                disabled={busy}
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
                onOpenChange={(open) => {
                  if (!open) return
                  if (!listIsCurrent()) { rowOrigin.current = null; rejectOldSelection(); return }
                  rowOrigin.current = listOrigin ?? null
                }}
                onConfirm={() => removeSelected([record.id], rowOrigin.current)}
              >
                <button type="button" disabled={busy} className={`${styles.actBtn} ${styles.actBtnDanger}`} title={t('common.delete')}>
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
            disabled={busy}
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
        <fieldset disabled={busy} style={{ border: 0, margin: 0, padding: 0 }}><NameSearch
          placeholder={t('knowledge.list.searchPlaceholder')}
          onSearch={(value) => {
            if (busy) return
            clearSelection()
            setKeywords(value)
            setPage(1)
          }}
        /></fieldset>
        <Select disabled={busy} aria-label={t('knowledge.list.sort')} value={sort} onChange={(value) => { clearSelection(); setSort(value); setPage(1) }} options={[
          { value: 'updated', label: t('knowledge.list.sortUpdated') },
          { value: 'oldest', label: t('knowledge.list.sortOldest') },
        ]} style={{ minWidth: 150 }} />
      </div>

      {canWrite && selected.length > 0 && (
        <Space wrap style={{ marginBottom: 16 }}>
          <span>{t('knowledge.list.selected', { count: selected.length })}</span>
          <Button disabled={busy} onClick={clearSelection}>{t('knowledge.list.clearSelection')}</Button>
          <Button danger icon={<TrashIcon size={16} />} loading={busy} onClick={() => {
            if (!listIsCurrent() || !originIsCurrent(selectionOrigin.current)) { rejectOldSelection(); return }
            confirmationOrigin.current = selectionOrigin.current
            setConfirmationIds(selected.map((dataset) => dataset.id))
          }}>{t('knowledge.list.batchDelete')}</Button>
        </Space>
      )}
      <Modal open={canWrite && confirmationIds.length > 0} title={t('knowledge.list.batchDeleteTitle', { count: confirmationIds.length })}
        okText={t('common.delete')} cancelText={t('common.cancel')} okButtonProps={{ type: 'default', danger: true }}
        confirmLoading={busy} cancelButtonProps={{ disabled: busy }} closable={!busy} keyboard={!busy} mask={{ closable: !busy }}
        onCancel={() => { if (!busy) setConfirmationIds([]) }} onOk={() => removeSelected(confirmationIds, confirmationOrigin.current)}>
        <p>{t('knowledge.list.batchDeleteHint')}</p>
        <ul style={{ maxHeight: 240, overflowY: 'auto', overflowWrap: 'anywhere', paddingLeft: 20 }}>{confirmationIds.map((id) => <li key={id}>{datasets.find((dataset) => dataset.id === id)?.name ?? id}</li>)}</ul>
      </Modal>
      {deleteError && <Alert type="error" showIcon title={t('knowledge.list.batchUnconfirmed')} description={deleteError}
        action={<Button disabled={busy} onClick={() => { clearSelection(); void queryClient.invalidateQueries({ queryKey: ['userinfo'] }); void refetch() }}>{t('knowledge.states.retry')}</Button>}
        style={{ marginBottom: 16 }} />}

      {isError ? (
        <Alert type="error" showIcon title={t('knowledge.states.listFailed')} description={t('knowledge.states.listFailedHint')} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} />
      ) : isLoading ? (
        <div className={styles.loadingWrap} role="status" aria-label={t('common.loading')}>
          <Spin size="medium" />
        </div>
      ) : (
        <BorderedTable<KnowledgeDataset>
          columns={columns}
          dataSource={datasets}
          rowKey="id"
          rowSelection={canWrite ? {
            selectedRowKeys: selected.map((dataset) => dataset.id),
            preserveSelectedRowKeys: false,
            onChange: (keys) => {
              if (!listIsCurrent()) { rejectOldSelection(); return }
              if (selectedIds.length && !originIsCurrent(selectionOrigin.current)) { rejectOldSelection(); return }
              if (!selectedIds.length) selectionOrigin.current = listOrigin ?? null
              setSelectedIds(keys.map(String)); setDeleteError('')
            },
            getCheckboxProps: (dataset) => ({ disabled: busy, 'aria-label': t('knowledge.list.selectDataset', { name: dataset.name || dataset.id }) }),
          } : undefined}
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
            disabled: busy,
            onChange: (next) => { clearSelection(); setPage(next); }
          }}
        />
      )}

      <KnowledgeForm onResume={() => { setEditing(null); setFormOpen(true); }} onCreated={(id) => { void navigate(`/knowledge/${id}/documents`) }} open={formOpen} editing={editing} onClose={() => { setFormOpen(false); }} />
    </div>
  )
}
