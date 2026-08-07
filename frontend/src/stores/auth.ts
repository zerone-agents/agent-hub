import { create } from 'zustand'
import type { User } from '@/api/auth'
import { authApi } from '@/api/auth'
import { clearTokens } from '@/api/client'

interface AuthState {
  user: User | null
  setUser: (user: User) => void
  login: () => void
  logout: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  setUser: (user) => { set({ user }); },
  login: () => { authApi.login(); },
  logout: async () => {
    try {
      await authApi.logout()
    } finally {
      clearTokens()
      set({ user: null })
    }
  }
}))
