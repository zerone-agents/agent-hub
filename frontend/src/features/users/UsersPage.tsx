import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Tag, Select, Button, Popconfirm, message, Modal, Typography } from 'antd'
import { PlusIcon } from '@phosphor-icons/react'
import type { ColumnsType } from 'antd/es/table'
import { usersApi, type AdminUser, type Invite, type UserRole } from '@/api/users'
import { authApi } from '@/api/auth'
import { parseApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import PageHeader from '@/components/PageHeader'
import PrimaryButton from '@/components/PrimaryButton'
import BorderedTable from '@/components/BorderedTable'
import CreateInviteModal from './CreateInviteModal'
import SignupLinkModal from './SignupLinkModal'

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
  const [signupLinkModalOpen, setSignupLinkModalOpen] = useState(false)
  const [resetTarget, setResetTarget] = useState<{ password: string } | null>(null)

  const { data: authMode } = useQuery({
    queryKey: ['auth', 'mode'],
    queryFn: authApi.getAuthMode
  })
  const isCasdoor = authMode?.mode === 'casdoor'

  const { data: users = [], isLoading: usersLoading } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: usersApi.listUsers
  })
  const { data: invites = [], isLoading: invitesLoading } = useQuery({
    queryKey: ['admin', 'invites'],
    queryFn: usersApi.listInvites,
    enabled: !isCasdoor
  })
  const { data: signupUrlData } = useQuery({
    queryKey: ['admin', 'signup-url'],
    queryFn: usersApi.getSignupUrl,
    enabled: isCasdoor
  })

  const invalidateAll = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['admin', 'users'] }),
      qc.invalidateQueries({ queryKey: ['admin', 'invites'] })
    ])
  }

  const updateMutation = useMutation({
    mutationFn: (vars: { id: string | number; patch: { role?: UserRole; status?: 'active' | 'disabled' } }) =>
      usersApi.updateUser(vars.id, vars.patch),
    onSuccess: () => { void invalidateAll(); message.success('已更新') },
    onError: (err) => message.error(parseApiError(err))
  })

  const resetMutation = useMutation({
    mutationFn: (id: string | number) => usersApi.resetPassword(id),
    onSuccess: (data) => { setResetTarget({ password: data.password }); },
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
          // casdoor 模式下未映射角色的用户 role 为 ""，归一为 undefined 以显示 placeholder。
          value={record.role || undefined}
          placeholder="-"
          style={{ width: 130 }}
          options={ROLE_OPTIONS}
          disabled={String(record.id) === currentUserId}
          onChange={(role: UserRole) =>
            { updateMutation.mutate({ id: record.id, patch: { role } }); }
          }
        />
      )
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (status: string) => {
        // pending = casdoor 待审批（本地成员表），分配角色后置 active。
        if (status === 'pending') return <Tag color="gold">待审批</Tag>
        return (
          <Tag color={status === 'active' ? 'green' : 'default'}>
            {status === 'active' ? '启用' : '禁用'}
          </Tag>
        )
      }
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
        // 待审批用户：只保留角色 Select（分配角色即审批），隐藏禁用/重置密码。
        if (record.status === 'pending') return null
        return (
          <>
            {record.status === 'active' ? (
              <Popconfirm
                title="确认禁用该用户？"
                description="用户将立即下线。"
                okText="禁用"
                okButtonProps={{ danger: true }}
                cancelText="取消"
                onConfirm={() =>
                  { updateMutation.mutate({ id: record.id, patch: { status: 'disabled' } }); }
                }
                disabled={isSelf}
              >
                <Button size="small" disabled={isSelf}>禁用</Button>
              </Popconfirm>
            ) : (
              <Button
                size="small"
                onClick={() =>
                  { updateMutation.mutate({ id: record.id, patch: { status: 'active' } }); }
                }
              >
                启用
              </Button>
            )}
            <Popconfirm
              title="确认重置密码？"
              description={`将为 "${record.username}" 生成随机新密码，原密码立即失效，所有会话下线。`}
              okText="重置"
              okButtonProps={{ danger: true }}
              cancelText="取消"
              onConfirm={() => { resetMutation.mutate(record.id); }}
              disabled={isSelf}
            >
              <Button size="small" style={{ marginLeft: 8 }} disabled={isSelf}>
                重置密码
              </Button>
            </Popconfirm>
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
            title="确认撤销该邀请？"
            description="撤销后该邀请链接立即失效，无法用于注册。"
            okText="撤销"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => { revokeMutation.mutate(record.id); }}
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
          isCasdoor ? (
            <PrimaryButton onClick={() => { setSignupLinkModalOpen(true); }}>
              注册链接
            </PrimaryButton>
          ) : (
            <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={() => { setInviteModalOpen(true); }}>
              创建邀请
            </PrimaryButton>
          )
        }
      />

      <Typography.Title level={5} style={{ marginTop: 24 }}>用户</Typography.Title>
      <BorderedTable<AdminUser>
        rowKey="id"
        loading={usersLoading}
        dataSource={users}
        columns={userColumns}
        pagination={false}
        size="middle"
      />

      {!isCasdoor && (
        <>
          <Typography.Title level={5} style={{ marginTop: 32 }}>邀请记录</Typography.Title>
          <BorderedTable<Invite>
            rowKey="id"
            loading={invitesLoading}
            dataSource={invites}
            columns={inviteColumns}
            pagination={false}
            size="middle"
          />

          <CreateInviteModal open={inviteModalOpen} onClose={() => { setInviteModalOpen(false); }} />
        </>
      )}

      {isCasdoor && (
        <SignupLinkModal
          open={signupLinkModalOpen}
          signupUrl={signupUrlData?.signupUrl}
          onClose={() => { setSignupLinkModalOpen(false); }}
        />
      )}

      <Modal
        title="重置密码成功"
        open={!!resetTarget}
        onCancel={() => { setResetTarget(null); }}
        footer={
          <PrimaryButton onClick={() => { setResetTarget(null); }}>关闭</PrimaryButton>
        }
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
