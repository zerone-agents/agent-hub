import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, Tag, Select, Button, Popconfirm, message, Modal, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { usersApi, type AdminUser, type Invite, type UserRole } from '@/api/users'
import { parseApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import PageHeader from '@/components/PageHeader'
import CreateInviteModal from './CreateInviteModal'

const ROLE_OPTIONS: { value: UserRole; label: string }[] = [
  { value: 'member', label: 'member' },
  { value: 'maintainer', label: 'maintainer' },
  { value: 'admin', label: 'admin' }
]

function roleColor(role: string): string {
  switch (role) {
    case 'admin': return 'red'
    case 'maintainer': return 'blue'
    default: return 'default'
  }
}

function inviteStatusColor(status: string): string {
  switch (status) {
    case 'used': return 'green'
    case 'expired': return 'default'
    default: return 'gold'
  }
}

export default function UsersPage() {
  const qc = useQueryClient()
  const currentUserId = useAuthStore((s) => s.user?.id)
  const [inviteModalOpen, setInviteModalOpen] = useState(false)
  const [resetTarget, setResetTarget] = useState<{ password: string } | null>(null)

  const { data: users = [], isLoading: usersLoading } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: usersApi.listUsers
  })
  const { data: invites = [], isLoading: invitesLoading } = useQuery({
    queryKey: ['admin', 'invites'],
    queryFn: usersApi.listInvites
  })

  const invalidateAll = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['admin', 'users'] }),
      qc.invalidateQueries({ queryKey: ['admin', 'invites'] })
    ])
  }

  const updateMutation = useMutation({
    mutationFn: (vars: { id: number; patch: { role?: UserRole; status?: 'active' | 'disabled' } }) =>
      usersApi.updateUser(vars.id, vars.patch),
    onSuccess: () => { void invalidateAll(); message.success('已更新') },
    onError: (err) => message.error(parseApiError(err))
  })

  const resetMutation = useMutation({
    mutationFn: (id: number) => usersApi.resetPassword(id),
    onSuccess: async (data) => {
      await invalidateAll()
      setResetTarget({ password: data.password })
    },
    onError: (err) => message.error(parseApiError(err))
  })

  const revokeMutation = useMutation({
    mutationFn: (id: number) => usersApi.revokeInvite(id),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['admin', 'invites'] }); message.success('已撤销') },
    onError: (err) => message.error(parseApiError(err))
  })

  const userColumns: ColumnsType<AdminUser> = [
    { title: '用户名', dataIndex: 'username', key: 'username' },
    { title: '昵称', dataIndex: 'displayName', key: 'displayName' },
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      width: 160,
      render: (_, record) => (
        <Select
          size="small"
          value={record.role}
          style={{ width: 130 }}
          options={ROLE_OPTIONS}
          disabled={String(record.id) === currentUserId}
          onChange={(role: UserRole) =>
            updateMutation.mutate({ id: record.id, patch: { role } })
          }
        />
      )
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (status: string) => (
        <Tag color={status === 'active' ? 'green' : 'default'}>
          {status === 'active' ? '启用' : '禁用'}
        </Tag>
      )
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 180,
      render: (v: string) => new Date(v).toLocaleString('zh-CN')
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, record) => {
        const isSelf = String(record.id) === currentUserId
        return (
          <>
            {record.status === 'active' ? (
              <Popconfirm
                title="确认禁用该用户？"
                description="用户将立即下线。"
                onConfirm={() =>
                  updateMutation.mutate({ id: record.id, patch: { status: 'disabled' } })
                }
                disabled={isSelf}
              >
                <Button size="small" disabled={isSelf}>禁用</Button>
              </Popconfirm>
            ) : (
              <Button
                size="small"
                onClick={() =>
                  updateMutation.mutate({ id: record.id, patch: { status: 'active' } })
                }
              >
                启用
              </Button>
            )}
            <Button
              size="small"
              style={{ marginLeft: 8 }}
              onClick={() => resetMutation.mutate(record.id)}
            >
              重置密码
            </Button>
          </>
        )
      }
    }
  ]

  const inviteColumns: ColumnsType<Invite> = [
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      width: 110,
      render: (r: string) => <Tag color={roleColor(r)}>{r}</Tag>
    },
    { title: '备注', dataIndex: 'note', key: 'note' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (s: string) => <Tag color={inviteStatusColor(s)}>{s}</Tag>
    },
    {
      title: '过期时间',
      dataIndex: 'expiresAt',
      key: 'expiresAt',
      width: 180,
      render: (v: string) => new Date(v).toLocaleString('zh-CN')
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, record) =>
        record.status === 'pending' ? (
          <Popconfirm
            title="撤销该邀请？"
            onConfirm={() => revokeMutation.mutate(record.id)}
          >
            <Button size="small" danger>撤销</Button>
          </Popconfirm>
        ) : null
    }
  ]

  return (
    <div>
      <PageHeader
        title="用户管理"
        subtitle="邀请用户、管理角色与账号状态。仅管理员可见。"
        extra={
          <Button type="primary" onClick={() => setInviteModalOpen(true)}>
            创建邀请
          </Button>
        }
      />

      <Typography.Title level={5} style={{ marginTop: 24 }}>用户</Typography.Title>
      <Table<AdminUser>
        rowKey="id"
        loading={usersLoading}
        dataSource={users}
        columns={userColumns}
        pagination={false}
        size="middle"
      />

      <Typography.Title level={5} style={{ marginTop: 32 }}>邀请记录</Typography.Title>
      <Table<Invite>
        rowKey="id"
        loading={invitesLoading}
        dataSource={invites}
        columns={inviteColumns}
        pagination={false}
        size="middle"
      />

      <CreateInviteModal open={inviteModalOpen} onClose={() => setInviteModalOpen(false)} />

      <Modal
        title="重置密码成功"
        open={!!resetTarget}
        onOk={() => setResetTarget(null)}
        onCancel={() => setResetTarget(null)}
        okText="已复制/关闭"
        cancelText="关闭"
      >
        <Typography.Paragraph type="warning">
          新密码仅显示这一次，请立即复制并安全送达被重置的用户：
        </Typography.Paragraph>
        <Typography.Paragraph copyable code>
          {resetTarget?.password ?? ''}
        </Typography.Paragraph>
      </Modal>
    </div>
  )
}
