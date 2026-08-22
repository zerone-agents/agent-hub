import apiClient, { getRefreshToken, unwrapResponse } from './client'
import type { ApiResponse } from '@/types/api'

export interface User {
  id: string
  name: string
  email: string
  avatar?: string
  role?: string
}

export interface UserInfoResponse {
  user_id?: string
  id?: string
  email?: string
  display_name?: string
  username?: string
  avatar?: string
  roles?: string[]
}

export interface RefreshResponse {
  accessToken: string
  refreshToken: string
}

export interface TokenPair {
  accessToken: string
  refreshToken: string
  expiresIn: number
}

export interface AuthMode {
  mode: 'builtin' | 'casdoor'
  initialized: boolean
  /** casdoor 模式下 tenant_oauth_clients 有行即 true——前端据此渲染组织选择入口。 */
  multiOrg?: boolean
}

export const authApi = {
  /** Casdoor SSO redirect entry. Only used when auth.mode = casdoor. */
  login: (org?: string) => {
    const target = org ?? new URLSearchParams(window.location.search).get('org') ?? ''
    window.location.href = target ? `/auth/login?org=${encodeURIComponent(target)}` : '/auth/login'
  },
  /** 组织预检：登录跳转前确认组织已注册（不存在就地报错，不整页跳 404）。 */
  checkOrg: (org: string) =>
    apiClient
      .get<ApiResponse<{ exists: boolean }>>(`/auth/org-check`, { params: { org } })
      .then((res) => unwrapResponse<{ exists: boolean }>(res)),
  /** Reports the active auth backend so the login page can render the right UI. */
  getAuthMode: () =>
    apiClient.get<ApiResponse<AuthMode>>('/auth/mode').then((res) => unwrapResponse<AuthMode>(res)),
  /** builtin login with username + password. */
  loginWithPassword: (username: string, password: string) =>
    apiClient
      .post<ApiResponse<TokenPair>>('/auth/login', { username, password })
      .then((res) => unwrapResponse<TokenPair>(res)),
  /** First-run setup: create the fixed-username `admin` account. */
  setup: (password: string, confirmPassword: string) =>
    apiClient
      .post<ApiResponse<TokenPair>>('/auth/setup', { password, confirmPassword })
      .then((res) => unwrapResponse<TokenPair>(res)),
  /** Consume a one-time invite and create the account (auto-login). */
  register: (inviteToken: string, username: string, password: string, displayName?: string) =>
    apiClient
      .post<ApiResponse<TokenPair>>('/auth/register', { inviteToken, username, password, displayName })
      .then((res) => unwrapResponse<TokenPair>(res)),
  /** Validate an invite token before rendering the register form. */
  precheckInvite: (token: string) =>
    apiClient
      .get<ApiResponse<{ valid: boolean; note: string }>>(`/auth/invite/${encodeURIComponent(token)}`)
      .then((res) => unwrapResponse<{ valid: boolean; note: string }>(res)),
  /** Self-service password change; revokes all other sessions. */
  changePassword: (oldPassword: string, newPassword: string) =>
    apiClient
      .post<ApiResponse<TokenPair>>('/auth/change-password', { oldPassword, newPassword })
      .then((res) => unwrapResponse<TokenPair>(res)),
  getUserInfo: () => apiClient.get<ApiResponse<UserInfoResponse>>('/auth/userinfo'),
  logout: async () => {
    try {
      await apiClient.post('/auth/logout', { refreshToken: getRefreshToken() })
    } finally {
      // tokens are cleared by the caller (auth store)
    }
  }
}
