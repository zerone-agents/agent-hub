import { type ReactNode } from 'react'
import { Navigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { getAccessToken } from '@/api/client'
import { authApi } from '@/api/auth'
import { useUserInfo } from '@/queries/useUserInfo'
import LoadingState from '@/components/LoadingState'
import PendingApprovalPage from '@/features/auth/PendingApprovalPage'

/**
 * Auth guard. Renders children only when a valid access token exists and
 * /auth/userinfo succeeds. Otherwise redirects to /login.
 *
 * casdoor 模式下 roles 为空的用户处于待审批状态（后端对业务接口返回
 * PENDING_APPROVAL 403），此时渲染待审批专属页替代主框架。
 *
 * Set `VITE_BYPASS_AUTH=true` in `.env.local` to skip auth for local preview
 * (renders a mock admin user without calling the backend).
 */
const BYPASS_AUTH = import.meta.env.VITE_BYPASS_AUTH === 'true'

export default function RequireAuth({ children }: { children: ReactNode }) {
  const token = getAccessToken()
  const { data: user, isLoading, isError } = useUserInfo({
    enabled: !BYPASS_AUTH && !!token
  })
  const { data: authMode, isLoading: modeLoading } = useQuery({
    queryKey: ['auth', 'mode'],
    queryFn: authApi.getAuthMode,
    enabled: !BYPASS_AUTH && !!token
  })

  if (BYPASS_AUTH) return <>{children}</>
  if (!token) return <Navigate to="/login" replace />
  if (isLoading || modeLoading) return <LoadingState />
  if (isError || !user) return <Navigate to="/login" replace />
  // casdoor 待审批：userinfo 成功但未分配任何角色 → 专属页面，不进主框架。
  if (authMode?.mode === 'casdoor' && !user.role) return <PendingApprovalPage />
  return <>{children}</>
}
