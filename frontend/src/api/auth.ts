import apiClient from './client'
import type { ApiResponse } from '@/types/api'

export interface User {
  id: string
  name: string
  email: string
  avatar?: string
}

export interface UserInfoResponse {
  user_id?: string
  id?: string
  email?: string
  display_name?: string
  username?: string
  avatar?: string
}

export interface RefreshResponse {
  accessToken: string
  refreshToken: string
}

export const authApi = {
  login: () => {
    window.location.href = '/auth/login'
  },
  getUserInfo: () => apiClient.get<ApiResponse<UserInfoResponse>>('/auth/userinfo'),
  logout: async () => {
    try {
      await apiClient.post('/auth/logout', '', {
        headers: { 'Content-Type': 'text/plain' }
      })
    } finally {
      // tokens are cleared by the caller (auth store)
    }
  }
}
