// H7.4 扩展详情页「权限」标签区：授权列表（scope/action/resource/状态）、
// 撤销/批准操作与调用审计日志表（时间/动作/结果/原因）。
import { useState } from 'react'
import { Alert, App, Button, Empty, Popconfirm, Space, Table, Tabs, Tag } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { parseApiError, unwrapResponse } from '@/api/client'
import {
  extensionAuthzApi,
  type ExtensionAccessAuditItem,
  type ExtensionGrant
} from '@/api/extensionAuthz'

function parseActions(raw: string): string[] {
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.map(String) : []
  } catch {
    return []
  }
}

function grantStatus(g: ExtensionGrant): { label: string; color: string } {
  if (g.revokedAt) return { label: '已撤销', color: 'red' }
  if (g.status === 'pending') return { label: '待批准', color: 'orange' }
  if (!g.isActive) return { label: '已停用', color: 'default' }
  return { label: '生效中', color: 'green' }
}

export default function ExtensionGrantsPanel({ extensionId }: { extensionId: number }) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [auditPage, setAuditPage] = useState(1)
  const [auditAllowed, setAuditAllowed] = useState<boolean | undefined>()

  const grantsQuery = useQuery({
    queryKey: ['extensions', extensionId, 'grants'],
    queryFn: async () =>
      unwrapResponse<{ items: ExtensionGrant[] }>(await extensionAuthzApi.grants(extensionId))
  })
  const grants = grantsQuery.data?.items

  const auditQuery = useQuery({
    queryKey: ['extensions', extensionId, 'audit', auditPage, auditAllowed],
    queryFn: async () =>
      unwrapResponse<{ items: ExtensionAccessAuditItem[]; total: number }>(
        await extensionAuthzApi.audit(extensionId, {
          page: auditPage,
          pageSize: 10,
          allowed: auditAllowed
        })
      )
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['extensions', extensionId, 'grants'] })
    queryClient.invalidateQueries({ queryKey: ['extensions', extensionId, 'audit'] })
  }

  // `success:false` 的响应 HTTP 状态仍是 200，axios 不会判为错误 —— 必须
  // unwrapResponse 才能发现失败，否则后端拒绝也会弹"撤销成功"。
  const revokeMutation = useMutation({
    mutationFn: async (grantId: number) =>
      unwrapResponse(await extensionAuthzApi.revokeGrant(extensionId, grantId)),
    onSuccess: () => {
      message.success('授权已撤销，即时生效')
      invalidate()
    },
    // 用 parseApiError 而不是 e.message：403 时 axios 给的是英文原文
    onError: (e: unknown) => message.error(parseApiError(e))
  })

  const approveMutation = useMutation({
    mutationFn: async (grantId: number) =>
      unwrapResponse(await extensionAuthzApi.approveGrant(extensionId, grantId)),
    onSuccess: () => {
      message.success('授权已批准')
      invalidate()
    },
    onError: (e: unknown) => message.error(parseApiError(e))
  })

  return (
    <>
      {grantsQuery.error && (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 12 }}
          message="加载授权列表失败"
          description={parseApiError(grantsQuery.error)}
        />
      )}
      <Tabs
        items={[
          {
            key: 'grants',
            label: '授权列表',
            children: (
              <Table<ExtensionGrant>
                rowKey="id"
                size="small"
                loading={grantsQuery.isLoading}
                dataSource={grants ?? []}
                pagination={false}
                locale={{ emptyText: <Empty description="安装扩展后按 manifest 声明生成授权" /> }}
                columns={[
                  { title: '权限类', dataIndex: 'permission', width: 110 },
                  { title: '命名空间', dataIndex: 'scope', render: (v: string) => v || '*' },
                  {
                    title: '操作',
                    dataIndex: 'actions',
                    render: (v: string) =>
                      parseActions(v).map((a) => (
                        <Tag key={a} style={{ marginInlineEnd: 4 }}>
                          {a}
                        </Tag>
                      ))
                  },
                  { title: '资源', dataIndex: 'resource', render: (v: string) => v || '—' },
                  {
                    title: '状态',
                    key: 'status',
                    width: 100,
                    render: (_, g) => {
                      const s = grantStatus(g)
                      return <Tag color={s.color}>{s.label}</Tag>
                    }
                  },
                  {
                    title: '模式',
                    dataIndex: 'mode',
                    width: 90,
                    render: (v: string) => (v === 'approval' ? '需审批' : '自动')
                  },
                  { title: '来源版本', dataIndex: 'sourceVersion', width: 100 },
                  {
                    title: '操作',
                    key: 'actions-col',
                    width: 130,
                    render: (_, g) => (
                      <Space>
                        {g.status === 'pending' && !g.revokedAt && (
                          <Button
                            size="small"
                            type="primary"
                            loading={approveMutation.isPending}
                            onClick={() => approveMutation.mutate(g.id)}
                          >
                            批准
                          </Button>
                        )}
                        {!g.revokedAt && (
                          <Popconfirm
                            title="撤销该授权？"
                            description="撤销后扩展立即失去对应权限。"
                            onConfirm={() => revokeMutation.mutate(g.id)}
                          >
                            <Button size="small" danger loading={revokeMutation.isPending}>
                              撤销
                            </Button>
                          </Popconfirm>
                        )}
                      </Space>
                    )
                  }
                ]}
              />
            )
          },
          {
            key: 'audit',
            label: '调用审计',
            children: (
              <Table<ExtensionAccessAuditItem>
                rowKey="id"
                size="small"
                loading={auditQuery.isLoading}
                dataSource={auditQuery.data?.items ?? []}
                locale={{ emptyText: <Empty description="扩展身份发起调用后此处记录允许/拒绝" /> }}
                pagination={{
                  current: auditPage,
                  pageSize: 10,
                  total: auditQuery.data?.total ?? 0,
                  showSizeChanger: false,
                  onChange: setAuditPage
                }}
                columns={[
                  {
                    title: '时间',
                    dataIndex: 'createdAt',
                    width: 180,
                    render: (v: string) => (v ? new Date(v).toLocaleString() : '—')
                  },
                  { title: '权限类', dataIndex: 'permission', width: 100 },
                  {
                    title: '动作',
                    key: 'action',
                    width: 160,
                    render: (_, r) => (
                      <span>
                        {r.action}
                        {r.resource ? ` · ${r.resource}` : ''}
                      </span>
                    )
                  },
                  {
                    title: '结果',
                    dataIndex: 'allowed',
                    width: 90,
                    filters: [
                      { text: '允许', value: true },
                      { text: '拒绝', value: false }
                    ],
                    filteredValue: auditAllowed === undefined ? null : [auditAllowed],
                    onFilter: () => true,
                    render: (v: boolean) =>
                      v ? <Tag color="green">允许</Tag> : <Tag color="red">拒绝</Tag>
                  },
                  {
                    title: '原因 / IP',
                    key: 'reason',
                    render: (_, r) => (
                      <span>
                        {r.deniedReason || '—'}
                        {r.ip ? `（${r.ip}）` : ''}
                      </span>
                    )
                  }
                ]}
                onChange={(pagination, filters) => {
                  const v = filters.allowed
                  setAuditAllowed(v === null || v === undefined ? undefined : (v[0] as boolean))
                  setAuditPage(pagination.current ?? 1)
                }}
              />
            )
          }
        ]}
      />
    </>
  )
}
