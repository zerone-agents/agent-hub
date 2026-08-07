import { type ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { getAccessToken } from '@/api/client'
import { useUserInfo } from '@/queries/useUserInfo'
import LoadingState from '@/components/LoadingState'

/**
 * Auth guard. Renders children only when a valid access token exists and
 * /auth/userinfo succeeds. Otherwise redirects to /login.
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

  if (BYPASS_AUTH) return <>{children}</>
  if (!token) return <Navigate to="/login" replace />
  if (isLoading) return <LoadingState />
  if (isError || !user) return <Navigate to="/login" replace />
  return <>{children}</>
}
