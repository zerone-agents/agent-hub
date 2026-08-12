import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { authApi, type User } from '@/api/auth'
import { getAccessToken } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

interface UseUserInfoOptions {
  enabled?: boolean
}

const BYPASS_AUTH = import.meta.env.VITE_BYPASS_AUTH === 'true'

const MOCK_USER: User = {
  id: 'mock-admin',
  email: 'admin@local',
  name: 'Admin (Local Preview)',
  avatar: ''
}

/**
 * Fetch the current user's info via /auth/userinfo and sync it into the auth store.
 * The query is the single source of truth for user data; the store is a projection.
 */
export function useUserInfo({ enabled = true }: UseUserInfoOptions = {}) {
  const setUser = useAuthStore((s) => s.setUser)

  const query = useQuery<User | null>({
    queryKey: ['userinfo'],
    queryFn: async () => {
      if (BYPASS_AUTH) return MOCK_USER
      const res = await authApi.getUserInfo()
      if (!res.data.success) return null
      const d = res.data.data
      const roles = d.roles ?? []
      return {
        id: d.user_id ?? d.id ?? '',
        email: d.email ?? '',
        name: d.display_name ?? d.username ?? d.email?.split('@')[0] ?? '',
        avatar: d.avatar,
        role: roles[0]
      }
    },
    enabled: BYPASS_AUTH || (enabled && !!getAccessToken()),
    staleTime: 5 * 60_000,
    retry: 1
  })

  useEffect(() => {
    if (query.data) setUser(query.data)
  }, [query.data, setUser])

  return query
}
