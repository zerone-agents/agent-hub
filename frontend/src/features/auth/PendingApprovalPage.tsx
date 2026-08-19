import { Card, Typography } from 'antd'
import { SignOutIcon } from '@phosphor-icons/react'
import PrimaryButton from '@/components/PrimaryButton'
import { useAuthStore } from '@/stores/auth'

/**
 * 待审批用户专属页面（casdoor 模式）。
 *
 * 新登录用户在管理员分配角色前 roles 为空，后端除白名单接口外一律返回
 * PENDING_APPROVAL 403。此页面替代主框架渲染，告知用户联系管理员，
 * 并提供退出登录入口。
 */
export default function PendingApprovalPage() {
  const logout = useAuthStore((s) => s.logout)

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24
      }}
    >
      <Card style={{ maxWidth: 480, width: '100%', textAlign: 'center' }}>
        <Typography.Title level={3}>账号待审批</Typography.Title>
        <Typography.Paragraph type="secondary">
          账号待审批：你已成功登录，但账号尚未被租户管理员分配角色。请联系管理员开通。
        </Typography.Paragraph>
        <PrimaryButton
          icon={<SignOutIcon size={16} weight="bold" />}
          onClick={() => {
            void logout()
          }}
        >
          退出登录
        </PrimaryButton>
      </Card>
    </div>
  )
}
