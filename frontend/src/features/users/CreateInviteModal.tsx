import { useState } from 'react'
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
  { value: 'member', label: '成员（member，只读使用）' },
  { value: 'maintainer', label: '维护者（maintainer，可管资源）' },
  { value: 'admin', label: '管理员（admin，可邀请+管用户）' }
]

/**
 * Creates a one-time invite. The plaintext token is returned exactly once and
 * rendered as a copyable registration URL; closing the modal discards it.
 */
export default function CreateInviteModal({ open, onClose }: CreateInviteModalProps) {
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
      message.success('邀请链接已复制')
    } else if (result === 'failed') {
      message.error('复制失败，请手动选择复制')
    }
  }

  return (
    <Modal
      title={created ? '邀请创建成功' : '创建邀请链接'}
      open={open}
      onOk={created ? handleClose : handleSubmit}
      onCancel={handleClose}
      okText={created ? '完成' : '创建'}
      cancelText="关闭"
      okButtonProps={{ className: primaryBtnCls.root }}
      confirmLoading={loading}
      destroyOnHidden
    >
      {created ? (
        <>
          <Typography.Paragraph type="warning" style={{ marginBottom: 12 }}>
            链接仅显示这一次，请立即复制保存。关闭后无法再获取该链接（如丢失只能撤销重建）。
          </Typography.Paragraph>
          <Space.Compact style={{ width: '100%' }}>
            <Input value={inviteURL} readOnly />
            <Button icon={<CopyIcon size={16} />} onClick={() => void copyURL()}>
              复制
            </Button>
          </Space.Compact>
        </>
      ) : (
        <>
          <div style={{ marginBottom: 12 }}>
            <div style={{ marginBottom: 6 }}>角色</div>
            <Select
              style={{ width: '100%' }}
              value={role}
              onChange={(v) => { setRole(v); }}
              options={ROLE_OPTIONS}
            />
          </div>
          <div style={{ marginBottom: 12 }}>
            <div style={{ marginBottom: 6 }}>备注（可选）</div>
            <Input
              placeholder="例如：给张三"
              value={note}
              onChange={(e) => { setNote(e.target.value); }}
              maxLength={128}
            />
          </div>
          <div>
            <div style={{ marginBottom: 6 }}>有效期（天，1-30）</div>
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
