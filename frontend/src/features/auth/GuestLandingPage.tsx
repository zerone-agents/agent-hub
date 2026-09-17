import { Button, Card, Typography } from 'antd'
import { ChatCircleDotsIcon, SignOutIcon } from '@phosphor-icons/react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import PrimaryButton from '@/components/PrimaryButton'
import { useAuthStore } from '@/stores/auth'

/**
 * Guest 落地页（管理路由守卫拦截 guest 时渲染，spec 6.3）。
 * 保留待审核语义：账号尚未开通管理权限，可先去 Agent 聊天页继续体验。
 */
export default function GuestLandingPage() {
  const { t } = useTranslation()
  const logout = useAuthStore((s) => s.logout)
  const navigate = useNavigate()

  // 与 UserDropdown/AppHeader 同款显式跳登录：仅依赖 RequireAuth 重渲染的
  // 隐式跳转在退出后不会自动刷新（用户需二次点击或手动刷新才到登录页）。
  const handleLogout = async () => {
    await logout()
    await navigate('/login')
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
      <Card style={{ maxWidth: 480, width: '100%', textAlign: 'center' }}>
        <Typography.Title level={3}>{t('auth.guestTitle')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('auth.guestHint')}
        </Typography.Paragraph>
        <PrimaryButton
          icon={<ChatCircleDotsIcon size={16} weight="bold" />}
          onClick={() => { void Promise.resolve(navigate('/agents/chat')) }}
          style={{ marginRight: 12 }}
        >
          {t('auth.goChat')}
        </PrimaryButton>
        {/* 退出是次要动作：普通描边按钮（PrimaryButton ghost 为 accent 底色设计，浅色卡片上文字不可见） */}
        <Button
          icon={<SignOutIcon size={16} weight="bold" />}
          onClick={() => { void handleLogout() }}
        >
          {t('common.logout')}
        </Button>
      </Card>
    </div>
  )
}
