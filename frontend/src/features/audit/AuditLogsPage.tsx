import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, DatePicker, Input, Select, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react'
import { auditApi, type AuditCategory, type AuditLog } from '@/api/audit'
import PageHeader from '@/components/PageHeader'
import BorderedTable from '@/components/BorderedTable'

const CATEGORY_OPTIONS: { value: AuditCategory; label: string }[] = [
  { value: 'auth', label: 'auth' },
  { value: 'user', label: 'user' },
  { value: 'invite', label: 'invite' },
  { value: 'provider', label: 'provider' },
  { value: 'agent', label: 'agent' },
  { value: 'token', label: 'token' },
  { value: 'aigc', label: 'aigc' }
]

const CATEGORY_COLORS: Record<string, string> = {
  auth: 'geekblue', user: 'orange', invite: 'cyan', provider: 'purple',
  agent: 'green', token: 'gold', aigc: 'magenta'
}

const STATUS_COLORS: Record<string, string> = { success: 'green', failure: 'red', partial: 'orange' }
const STATUS_LABELS: Record<string, string> = { success: 'audit.status.success', failure: 'audit.status.failure', partial: 'audit.status.partial' }

const PAGE_SIZE = 20

export default function AuditLogsPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [category, setCategory] = useState<string | undefined>()
  const [user, setUser] = useState('')
  const [range, setRange] = useState<[string, string] | null>(null)
  const [page, setPage] = useState(1)
  // 快照界来自服务端（spec §5.3/§6.2）：首屏响应的 snapshotId（string）存 state，
  // 会话内翻页/筛选按字符串原样回传；null = 首屏；刷新按钮重开快照。不使用客户端时钟。
  const [snapshotId, setSnapshotId] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'audit-logs', { category, user, range, page, snapshotId }],
    queryFn: () =>
      auditApi.listLogs({
        page,
        page_size: PAGE_SIZE,
        category: category ?? undefined,
        user: user || undefined,
        from: range?.[0],
        to: range?.[1],
        snapshotId: snapshotId ?? undefined
      })
  })

  useEffect(() => {
    if (data?.snapshotId && snapshotId === null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- capture the server snapshot boundary from the first response; in-session paging/filtering replays it verbatim (spec §5.3/§6.2)
      setSnapshotId(data.snapshotId)
    }
  }, [data, snapshotId])

  const handleRefresh = () => {
    // 用户主动刷新 → 重开快照。必须 removeQueries 而非 invalidateQueries：
    // snapshotId=null 对应的 query key 若留有首屏旧缓存，useQuery 会在重置后
    // 立即回放旧 data，effect 又把旧 snapshotId 写回——刷新沦为空转、新记录
    // 不可见（PR #150 二轮审查 P2）。removeQueries 返回 void（v5）同步清缓存。
    setSnapshotId(null)
    setPage(1)
    qc.removeQueries({ queryKey: ['admin', 'audit-logs'] })
  }

  const columns: ColumnsType<AuditLog> = [
    { title: t('audit.columns.time'), dataIndex: 'createdAt', key: 'createdAt', width: 180, render: (v: string) => new Date(v).toLocaleString() },
    { title: t('audit.columns.user'), key: 'user', render: (_, r) => r.userName || r.userId || '-' },
    { title: t('audit.columns.action'), dataIndex: 'action', key: 'action', width: 200, render: (v: string, r) => <Tag color={CATEGORY_COLORS[r.category]}>{v}</Tag> },
    { title: t('audit.columns.target'), key: 'target', render: (_, r) => <span title={r.targetId}>{r.targetName || r.targetId || '-'}</span> },
    { title: t('audit.columns.result'), dataIndex: 'status', key: 'status', width: 100, render: (v: string) => <Tag color={STATUS_COLORS[v]}>{t(STATUS_LABELS[v] ?? v)}</Tag> },
    { title: 'IP', dataIndex: 'remoteIp', key: 'remoteIp', width: 140 }
  ]

  return (
    <div>
      <PageHeader title={t('audit.pageTitle')} subtitle={t('audit.pageSub')} />
      <div style={{ display: 'flex', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
        <Select
          allowClear
          placeholder="Category"
          style={{ width: 140 }}
          value={category}
          options={CATEGORY_OPTIONS}
          onChange={(v) => { setCategory(v); setPage(1) }}
        />
        <Input.Search
          placeholder={t('audit.searchPlaceholder')}
          allowClear
          style={{ width: 200 }}
          onSearch={(v) => { setUser(v); setPage(1) }}
        />
        <DatePicker.RangePicker
          showTime
          onChange={(dates) => {
            setRange(dates?.[0] && dates[1] ? [dates[0].toISOString(), dates[1].toISOString()] : null)
            setPage(1)
          }}
        />
        <Button icon={<ArrowsClockwiseIcon size={14} />} onClick={handleRefresh}>
          {t('audit.refresh')}
        </Button>
      </div>
      <BorderedTable<AuditLog>
        rowKey="id"
        columns={columns}
        dataSource={data?.items ?? []}
        loading={isLoading}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: (p) => { setPage(p) }
        }}
        expandable={{
          expandedRowRender: (r) =>
            r.detail ? (
              <pre style={{ margin: 0 }}>{JSON.stringify(r.detail, null, 2)}</pre>
            ) : (
              <Typography.Text type="secondary">{t('audit.noDetail')}</Typography.Text>
            )
        }}
      />
    </div>
  )
}
