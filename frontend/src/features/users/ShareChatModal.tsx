import { Modal, Input, Typography, message, Space, Button } from 'antd'
import { CopyIcon } from '@phosphor-icons/react'
import { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { copyOrManual } from '@/utils/clipboard'

interface ShareChatModalProps {
  open: boolean
  onClose: () => void
}

/**
 * 分享对话页：展示 Agent 聊天总览（/agents/chat）的完整链接供管理员复制
 * 分发（形态参考 LoginLinkModal）。链接为前端本地构造（router basename
 * /static/），无需后端参与；打开的用户登录后即落聊天总览。
 */
export default function ShareChatModal({ open, onClose }: ShareChatModalProps) {
  const primaryBtnCls = usePrimaryButtonStyle()
  const chatUrl = `${window.location.origin}/static/agents/chat`

  const copyURL = async () => {
    const result = await copyOrManual(chatUrl)
    if (result === 'copied') {
      message.success('对话页链接已复制')
    } else if (result === 'failed') {
      message.error('复制失败，请手动选择复制')
    }
  }

  return (
    <Modal
      title="分享对话页"
      open={open}
      onOk={onClose}
      onCancel={onClose}
      okText="完成"
      cancelText="关闭"
      okButtonProps={{ className: primaryBtnCls.root }}
      destroyOnHidden
    >
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        将此链接发给用户，登录后即可进入 Agent 聊天页体验（仅开放给体验用户的 Agent 可见）。
      </Typography.Paragraph>
      <Space.Compact style={{ width: '100%' }}>
        <Input value={chatUrl} readOnly />
        <Button icon={<CopyIcon size={16} />} onClick={() => { void copyURL(); }}>
          复制
        </Button>
      </Space.Compact>
    </Modal>
  )
}
