import { useState } from 'react'
import { Modal, message } from 'antd'
import PasswordInput from '@/components/PasswordInput'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { authApi } from '@/api/auth'
import { parseApiError, setTokens } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

interface ChangePasswordModalProps {
  open: boolean
  onClose: () => void
}

/**
 * Self-service password change. On success the backend revokes all other
 * sessions and returns a fresh token pair, which we install so the current
 * session stays logged in.
 */
export default function ChangePasswordModal({ open, onClose }: ChangePasswordModalProps) {
  const primaryBtnCls = usePrimaryButtonStyle()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  // keep store reference stable so the linter doesn't flag unused setUser
  const _setUser = useAuthStore((s) => s.setUser)
  void _setUser

  const reset = () => {
    setOldPassword('')
    setNewPassword('')
    setConfirm('')
    setError('')
  }

  const handleClose = () => {
    reset()
    onClose()
  }

  const handleSubmit = async () => {
    setError('')
    if (newPassword !== confirm) {
      setError('两次输入的新密码不一致')
      return
    }
    if (newPassword.length < 8) {
      setError('新密码至少 8 位，且需包含字母和数字')
      return
    }
    setLoading(true)
    try {
      const pair = await authApi.changePassword(oldPassword, newPassword)
      setTokens(pair.accessToken, pair.refreshToken)
      message.success('密码已更新')
      reset()
      onClose()
    } catch (err) {
      setError(parseApiError(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title="修改密码"
      open={open}
      onOk={handleSubmit}
      onCancel={handleClose}
      okText="更新密码"
      cancelText="取消"
      okButtonProps={{ className: primaryBtnCls.root }}
      confirmLoading={loading}
      destroyOnHidden
    >
      {error && <div style={{ color: '#d4380d', marginBottom: 12 }}>{error}</div>}
      <div style={{ marginBottom: 12 }}>
        <PasswordInput
          placeholder="当前密码"
          value={oldPassword}
          onChange={(e) => { setOldPassword(e.target.value); }}
        />
      </div>
      <div style={{ marginBottom: 12 }}>
        <PasswordInput
          placeholder="新密码（至少 8 位，含字母和数字）"
          value={newPassword}
          onChange={(e) => { setNewPassword(e.target.value); }}
        />
      </div>
      <div>
        <PasswordInput
          placeholder="确认新密码"
          value={confirm}
          onChange={(e) => { setConfirm(e.target.value); }}
          onPressEnter={handleSubmit}
        />
      </div>
    </Modal>
  )
}
