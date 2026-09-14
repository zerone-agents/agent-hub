import { type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { getAccessToken } from '@/api/client'
import { useAuthMode } from '@/features/login/useAuthMode'
import { useUserInfo } from '@/queries/useUserInfo'
import { isGuestUser } from '@/lib/auth-guest'
import LoadingState from '@/components/LoadingState'
import GuestLandingPage from '@/features/auth/GuestLandingPage'

/**
 * Auth guard. Renders children only when a valid access token exists and
 * /auth/userinfo succeeds. Otherwise redirects to /login?redirect=<origin>.
 *
 * guest（体验用户，spec 5.2）：allowGuest（聊天路由）放行；'/' 分流到
 * /agents/chat（登录落地无来源时的角色兜底）；其余管理路径渲染
 * GuestLandingPage（待审核 + 前往聊天入口）。
 */
const BYPASS_AUTH = import.meta.env.VITE_BYPASS_AUTH === 'true'

export default function RequireAuth({
  allowGuest = false,
  children
}: {
  allowGuest?: boolean
  children: ReactNode
}) {
  const location = useLocation()
  const token = getAccessToken()
  const { data: user, isLoading, isError } = useUserInfo({ enabled: !BYPASS_AUTH && !!token })
  const { data: authMode, isLoading: modeLoading } = useAuthMode({ enabled: !BYPASS_AUTH && !!token })

  if (BYPASS_AUTH) return <>{children}</>
  if (!token) {
    return <Navigate to={`/login?redirect=${encodeURIComponent(location.pathname + location.search + location.hash)}`} replace />
  }
  if (isLoading || modeLoading) return <LoadingState />
  if (isError || !user) {
    return <Navigate to={`/login?redirect=${encodeURIComponent(location.pathname + location.search + location.hash)}`} replace />
  }
  if (isGuestUser(user, authMode?.mode)) {
    if (allowGuest) return <>{children}</>
    if (location.pathname === '/') return <Navigate to="/agents/chat" replace />
    return <GuestLandingPage />
  }
  return <>{children}</>
}
