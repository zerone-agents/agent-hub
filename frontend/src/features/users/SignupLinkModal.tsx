import { Modal, Input, Typography, message, Space, Button } from 'antd'
import { CopyIcon } from '@phosphor-icons/react'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { copyOrManual } from '@/utils/clipboard'

interface SignupLinkModalProps {
  open: boolean
  signupUrl?: string
  onClose: () => void
}

/**
 * casdoor 模式下的注册入口：展示组织注册页链接供管理员复制分发。
 * 链接是静态的（由后端 /admin/users/signup-url 提供），无创建步骤，
 * 打开即显示，可重复获取——与 builtin 的一次性邀请链接不同。
 */
export default function SignupLinkModal({ open, signupUrl, onClose }: SignupLinkModalProps) {
  const primaryBtnCls = usePrimaryButtonStyle()
  const copyURL = async () => {
    if (!signupUrl) return
    const result = await copyOrManual(signupUrl)
    if (result === 'copied') {
      message.success('注册链接已复制')
    } else if (result === 'failed') {
      message.error('复制失败，请手动选择复制')
    }
  }

  return (
    <Modal
      title="注册链接"
      open={open}
      onOk={onClose}
      onCancel={onClose}
      okText="完成"
      cancelText="关闭"
      okButtonProps={{ className: primaryBtnCls.root }}
      destroyOnHidden
    >
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        将此链接发给新用户，在 Casdoor 完成注册后，回到本页即可看到该用户。
      </Typography.Paragraph>
      <Space.Compact style={{ width: '100%' }}>
        <Input value={signupUrl ?? ''} readOnly />
        <Button icon={<CopyIcon size={16} />} onClick={() => void copyURL()}>
          复制
        </Button>
      </Space.Compact>
    </Modal>
  )
}
