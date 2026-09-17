import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Tag, Select, Button, Popconfirm, message, Modal, Typography } from 'antd'
import { PlusIcon, SignInIcon } from '@phosphor-icons/react'
import type { ColumnsType } from 'antd/es/table'
import { usersApi, type AdminUser, type Invite, type UserRole } from '@/api/users'
import { authApi } from '@/api/auth'
import { parseApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import PageHeader from '@/components/PageHeader'
import PrimaryButton from '@/components/PrimaryButton'
import BorderedTable from '@/components/BorderedTable'
import CreateInviteModal from './CreateInviteModal'
import LoginLinkModal from './LoginLinkModal'

const ROLE_OPTIONS: { value: UserRole; label: string }[] = [
  { value: 'member', label: 'member' },
  { value: 'maintainer', label: 'maintainer' },
  { value: 'admin', label: 'admin' },
  { value: 'guest', label: 'guest' }
]

function roleColor(role: string): string {
  switch (role) {
    case 'admin': return 'red'
    case 'maintainer': return 'blue'
    case 'guest': return 'gold'
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
  const { t } = useTranslation()
  const qc = useQueryClient()
  const currentUserId = useAuthStore((s) => s.user?.id)
  const [inviteModalOpen, setInviteModalOpen] = useState(false)
  const [loginLinkModalOpen, setLoginLinkModalOpen] = useState(false)
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
  // 登录链接是一次性的（回调消费后失效）。不用 useQuery：全局 staleTime
  // 30s 会让 30s 内重开的弹窗复用缓存中的旧链接（已消费 → 回调 400）。
  // 用 mutation：每次打开显式 reset 清空旧值再重新生成。
  const loginUrlMutation = useMutation({
    mutationFn: usersApi.getLoginUrl
  })

  const openLoginLinkModal = () => {
    loginUrlMutation.reset() // 清空旧值：请求完成前不展示已消费的链接
    setLoginLinkModalOpen(true)
    loginUrlMutation.mutate()
  }

  const invalidateAll = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['admin', 'users'] }),
      qc.invalidateQueries({ queryKey: ['admin', 'invites'] })
    ])
  }

  const updateMutation = useMutation({
    mutationFn: (vars: { id: string | number; patch: { role?: UserRole; status?: 'active' | 'disabled' } }) =>
      usersApi.updateUser(vars.id, vars.patch),
    onSuccess: () => { void invalidateAll(); message.success(t('users.toast.updated')) },
    onError: (err) => message.error(parseApiError(err))
  })

  const resetMutation = useMutation({
    mutationFn: (id: string | number) => usersApi.resetPassword(id),
    onSuccess: (data) => { setResetTarget({ password: data.password }); },
    onError: (err) => message.error(parseApiError(err))
  })

  const revokeMutation = useMutation({
    mutationFn: (id: number) => usersApi.revokeInvite(id),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['admin', 'invites'] }); message.success(t('users.toast.revoked')) },
    onError: (err) => message.error(parseApiError(err))
  })

  const userColumns: ColumnsType<AdminUser> = [
    { title: t('users.columns.username'), dataIndex: 'username', key: 'username' },
    { title: t('users.columns.nickname'), dataIndex: 'displayName', key: 'displayName' },
    {
      title: t('users.columns.role'),
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
      title: t('users.columns.status'),
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (status: string) => {
        // pending = casdoor 待审批（本地成员表），分配角色后置 active。
        if (status === 'pending') return <Tag color="gold">{t('users.status.pending')}</Tag>
        return (
          <Tag color={status === 'active' ? 'green' : 'default'}>
            {status === 'active' ? t('users.status.active') : t('users.status.disabled')}
          </Tag>
        )
      }
    },
    {
      title: t('users.columns.createdAt'),
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 180,
      render: (v: string) => new Date(v).toLocaleString('zh-CN')
    },
    {
      title: t('users.columns.actions'),
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
                title={t('users.disableConfirmTitle')}
                description={t('users.disableConfirmDesc')}
                okText={t('users.disable')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() =>
                  { updateMutation.mutate({ id: record.id, patch: { status: 'disabled' } }); }
                }
                disabled={isSelf}
              >
                <Button size="small" disabled={isSelf}>{t('users.disable')}</Button>
              </Popconfirm>
            ) : (
              <Button
                size="small"
                onClick={() =>
                  { updateMutation.mutate({ id: record.id, patch: { status: 'active' } }); }
                }
              >
                {t('users.enable')}
              </Button>
            )}
            <Popconfirm
              title={t('users.resetConfirmTitle')}
              description={t('users.resetConfirmDesc', { name: record.username })}
              okText={t('users.reset')}
              okButtonProps={{ danger: true }}
              cancelText={t('common.cancel')}
              onConfirm={() => { resetMutation.mutate(record.id); }}
              disabled={isSelf}
            >
              <Button size="small" style={{ marginLeft: 8 }} disabled={isSelf}>
                {t('users.resetPassword')}
              </Button>
            </Popconfirm>
          </>
        )
      }
    }
  ]

  const inviteColumns: ColumnsType<Invite> = [
    {
      title: t('users.columns.role'),
      dataIndex: 'role',
      key: 'role',
      width: 110,
      render: (r: string) => <Tag color={roleColor(r)}>{r}</Tag>
    },
    { title: t('users.columns.note'), dataIndex: 'note', key: 'note' },
    {
      title: t('users.columns.status'),
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
      title: t('users.columns.actions'),
      key: 'actions',
      width: 100,
      render: (_, record) =>
        record.status === 'pending' ? (
          <Popconfirm
            title="确认撤销该邀请？"
            description="撤销后该邀请链接立即失效，无法用于注册。"
            okText={t('users.revoke')}
            okButtonProps={{ danger: true }}
            cancelText={t('common.cancel')}
            onConfirm={() => { revokeMutation.mutate(record.id); }}
          >
            <Button size="small" danger>{t('users.revoke')}</Button>
          </Popconfirm>
        ) : null
    }
  ]

  return (
    <div>
      <PageHeader
        title={t('users.pageTitle')}
        subtitle={t('users.pageSub')}
        extra={
          isCasdoor ? (
            <PrimaryButton icon={<SignInIcon size={16} weight="bold" />} onClick={openLoginLinkModal}>
              {t('users.loginLink')}
            </PrimaryButton>
          ) : (
            <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={() => { setInviteModalOpen(true); }}>
              {t('users.createInvite')}
            </PrimaryButton>
          )
        }
      />

      <Typography.Title level={5} style={{ marginTop: 24 }}>{t('users.sectionUsers')}</Typography.Title>
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
          <Typography.Title level={5} style={{ marginTop: 32 }}>{t('users.sectionInvites')}</Typography.Title>
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
        <LoginLinkModal
          open={loginLinkModalOpen}
          loginUrl={loginUrlMutation.data?.loginUrl}
          loading={loginUrlMutation.isPending}
          onClose={() => { setLoginLinkModalOpen(false); }}
        />
      )}

      <Modal
        title={t('users.resetSuccessTitle')}
        open={!!resetTarget}
        onCancel={() => { setResetTarget(null); }}
        footer={
          <PrimaryButton onClick={() => { setResetTarget(null); }}>{t('users.invite.close')}</PrimaryButton>
        }
      >
        <Typography.Paragraph type="warning">
          {t('users.resetSuccessHint')}
        </Typography.Paragraph>
        <Typography.Paragraph copyable code>
          {resetTarget?.password ?? ''}
        </Typography.Paragraph>
      </Modal>
    </div>
  )
}
