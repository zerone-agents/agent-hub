import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Modal, Select, Input, InputNumber, Typography, message, Space, Button } from 'antd'
import { CopyIcon } from '@phosphor-icons/react'
import { usersApi, type UserRole, type CreatedInvite } from '@/api/users'
import { parseApiError } from '@/api/client'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { useQueryClient } from '@tanstack/react-query'
import { copyOrManual } from '@/utils/clipboard'

interface CreateInviteModalProps {
  open: boolean
  onClose: () => void
}

const ROLE_OPTIONS: { value: UserRole; label: string }[] = [
  { value: 'member', label: 'users.invite.roleMember' },
  { value: 'maintainer', label: 'users.invite.roleMaintainer' },
  { value: 'admin', label: 'users.invite.roleAdmin' }
]

/**
 * Creates a one-time invite. The plaintext token is returned exactly once and
 * rendered as a copyable registration URL; closing the modal discards it.
 */
export default function CreateInviteModal({ open, onClose }: CreateInviteModalProps) {
  const { t } = useTranslation()
  const primaryBtnCls = usePrimaryButtonStyle()
  const [role, setRole] = useState<UserRole>('member')
  const [note, setNote] = useState('')
  const [days, setDays] = useState<number>(7)
  const [loading, setLoading] = useState(false)
  const [created, setCreated] = useState<CreatedInvite | null>(null)
  const qc = useQueryClient()

  const inviteURL = created
    ? `${window.location.origin}/static/register?token=${created.token}`
    : ''

  const reset = () => {
    setRole('member')
    setNote('')
    setDays(7)
    setCreated(null)
  }

  const handleClose = () => {
    reset()
    onClose()
  }

  const handleSubmit = async () => {
    setLoading(true)
    try {
      const res = await usersApi.createInvite({ role, note: note || undefined, expiresInDays: days })
      setCreated(res)
      void qc.invalidateQueries({ queryKey: ['admin', 'invites'] })
    } catch (err) {
      message.error(parseApiError(err))
    } finally {
      setLoading(false)
    }
  }

  const copyURL = async () => {
    if (!inviteURL) return
    const result = await copyOrManual(inviteURL)
    if (result === 'copied') {
      message.success(t('users.invite.copied'))
    } else if (result === 'failed') {
      message.error(t('users.copyFail'))
    }
  }

  return (
    <Modal
      title={created ? t('users.invite.createdTitle') : t('users.invite.createTitle')}
      open={open}
      onOk={created ? handleClose : handleSubmit}
      onCancel={handleClose}
      okText={created ? t('users.invite.done') : t('users.invite.create')}
      cancelText={t('users.invite.close')}
      okButtonProps={{ className: primaryBtnCls.root }}
      confirmLoading={loading}
      destroyOnHidden
    >
      {created ? (
        <>
          <Typography.Paragraph type="warning" style={{ marginBottom: 12 }}>
            {t('users.invite.onceHint')}
          </Typography.Paragraph>
          <Space.Compact style={{ width: '100%' }}>
            <Input value={inviteURL} readOnly />
            <Button icon={<CopyIcon size={16} />} onClick={() => void copyURL()}>
              {t('users.copy')}
            </Button>
          </Space.Compact>
        </>
      ) : (
        <>
          <div style={{ marginBottom: 12 }}>
            <div style={{ marginBottom: 6 }}>{t('users.invite.roleLabel')}</div>
            <Select
              style={{ width: '100%' }}
              value={role}
              onChange={(v) => { setRole(v); }}
              options={ROLE_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
            />
          </div>
          <div style={{ marginBottom: 12 }}>
            <div style={{ marginBottom: 6 }}>{t('users.invite.noteLabel')}</div>
            <Input
              placeholder={t('users.invite.notePlaceholder')}
              value={note}
              onChange={(e) => { setNote(e.target.value); }}
              maxLength={128}
            />
          </div>
          <div>
            <div style={{ marginBottom: 6 }}>{t('users.invite.ttlLabel')}</div>
            <InputNumber
              min={1}
              max={30}
              value={days}
              onChange={(v) => { setDays(Number(v) || 7); }}
              style={{ width: '100%' }}
            />
          </div>
        </>
      )}
    </Modal>
  )
}
