import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Modal, message } from 'antd'
import PasswordInput from '@/components/PasswordInput'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { authApi } from '@/api/auth'
import { parseApiError, setTokens } from '@/api/client'

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
  const { t } = useTranslation()
  const primaryBtnCls = usePrimaryButtonStyle()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

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
      setError(t('users.changePassword.mismatch'))
      return
    }
    if (newPassword.length < 8) {
      setError(t('users.changePassword.rule'))
      return
    }
    setLoading(true)
    try {
      const pair = await authApi.changePassword(oldPassword, newPassword)
      setTokens(pair.accessToken, pair.refreshToken)
      message.success(t('users.changePassword.done'))
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
      title={t('users.changePassword.title')}
      open={open}
      onOk={handleSubmit}
      onCancel={handleClose}
      okText={t('users.changePassword.ok')}
      cancelText={t('common.cancel')}
      okButtonProps={{ className: primaryBtnCls.root }}
      confirmLoading={loading}
      destroyOnHidden
    >
      {error && <div style={{ color: '#d4380d', marginBottom: 12 }}>{error}</div>}
      <form noValidate autoComplete="off" onSubmit={(e) => { e.preventDefault(); void handleSubmit(); }}>
        <div style={{ marginBottom: 12 }}>
          <PasswordInput
            placeholder={t('users.changePassword.current')}
            name="currentPassword"
            value={oldPassword}
            onChange={(e) => { setOldPassword(e.target.value); }}
            autoComplete="off"
          />
        </div>
        <div style={{ marginBottom: 12 }}>
          <PasswordInput
            placeholder={t('users.changePassword.next')}
            name="newPassword"
            value={newPassword}
            onChange={(e) => { setNewPassword(e.target.value); }}
            autoComplete="off"
          />
        </div>
        <div>
          <PasswordInput
            placeholder={t('users.changePassword.confirm')}
            name="confirmPassword"
            value={confirm}
            onChange={(e) => { setConfirm(e.target.value); }}
            autoComplete="off"
          />
        </div>
      </form>
    </Modal>
  )
}
