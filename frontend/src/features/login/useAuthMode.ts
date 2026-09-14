import { useQuery } from '@tanstack/react-query'
import { authApi } from '@/api/auth'

/**
 * Reports the active auth backend so the login page can render a password
 * form (builtin) or an SSO button (casdoor), and redirect to /setup when the
 * system is not yet initialized.
 * Accepts an optional `enabled` flag to gate the query (defaults to true).
 */
export function useAuthMode(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ['auth-mode'],
    queryFn: () => authApi.getAuthMode(),
    staleTime: 60_000,
    retry: 1,
    enabled: options?.enabled ?? true
  })
}
