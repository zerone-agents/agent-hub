import { create } from 'zustand'
import type { User } from '@/api/auth'
import { authApi } from '@/api/auth'
import { clearTokens, setTokens } from '@/api/client'

interface AuthState {
  user: User | null
  setUser: (user: User) => void
  /** builtin username+password login. */
  loginWithPassword: (username: string, password: string) => Promise<void>
  /** casdoor SSO redirect (no-op for builtin mode). */
  login: () => void
  logout: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  setUser: (user) => { set({ user }) },
  loginWithPassword: async (username, password) => {
    const pair = await authApi.loginWithPassword(username, password)
    setTokens(pair.accessToken, pair.refreshToken)
  },
  login: () => { authApi.login() },
  logout: async () => {
    try {
      await authApi.logout()
    } finally {
      clearTokens()
      set({ user: null })
    }
  }
}))
