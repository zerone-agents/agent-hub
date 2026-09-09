import { Modal, Input, Typography, message, Space, Button } from 'antd'
import { CopyIcon } from '@phosphor-icons/react'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { copyOrManual } from '@/utils/clipboard'

interface LoginLinkModalProps {
  open: boolean
  loginUrl?: string
  /** 正在向后端请求新链接：清空旧值、禁用复制，避免分发已消费的旧链接。 */
  loading?: boolean
  onClose: () => void
}

/**
 * casdoor 模式下的登录入口：展示本组织的一次性 OAuth 授权登录链接供管理员
 * 复制分发。链接由后端 /admin/users/login-url 生成（携带 client_id、PKCE、
 * redirect_uri），新用户打开后走 Casdoor 的登录/注册流，落在本组织的
 * Application 上——与 builtin 的一次性邀请链接不同，这里无本地记录。
 */
export default function LoginLinkModal({ open, loginUrl, loading = false, onClose }: LoginLinkModalProps) {
  const primaryBtnCls = usePrimaryButtonStyle()
  const copyURL = async () => {
    if (!loginUrl) return
    const result = await copyOrManual(loginUrl)
    if (result === 'copied') {
      message.success('登录链接已复制')
    } else if (result === 'failed') {
      message.error('复制失败，请手动选择复制')
    }
  }

  return (
    <Modal
      title="登录链接"
      open={open}
      onOk={onClose}
      onCancel={onClose}
      okText="完成"
      cancelText="关闭"
      okButtonProps={{ className: primaryBtnCls.root }}
      destroyOnHidden
    >
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        将此链接发给新用户，通过 Casdoor 完成登录（未注册的账号可在登录页注册）后回到本页即可看到该用户。
      </Typography.Paragraph>
      <Space.Compact style={{ width: '100%' }}>
        <Input value={loginUrl ?? ''} readOnly placeholder={loading ? '生成中…' : undefined} />
        <Button icon={<CopyIcon size={16} />} onClick={() => void copyURL()} disabled={loading || !loginUrl}>
          复制
        </Button>
      </Space.Compact>
    </Modal>
  )
}