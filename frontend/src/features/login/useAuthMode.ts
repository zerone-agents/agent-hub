import { useQuery } from '@tanstack/react-query'
import { authApi } from '@/api/auth'

/**
 * Reports the active auth backend so the login page can render a password
 * form (builtin) or an SSO button (casdoor), and redirect to /setup when the
 * system is not yet initialized.
 */
export function useAuthMode() {
  return useQuery({
    queryKey: ['auth-mode'],
    queryFn: () => authApi.getAuthMode(),
    staleTime: 60_000,
    retry: 1
  })
}
