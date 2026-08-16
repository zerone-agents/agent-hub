import apiClient, { unwrapResponse } from './client'
import type { ApiResponse } from '@/types/api'

export type UserRole = 'admin' | 'maintainer' | 'member'
export type UserStatus = 'active' | 'disabled'

export interface AdminUser {
  // builtin 模式为 number；casdoor 模式为 casdoor 用户 Id 字符串。
  id: string | number
  username: string
  displayName: string
  email: string
  role: UserRole
  status: UserStatus
  createdAt: string
}

export type InviteStatus = 'pending' | 'used' | 'expired'

export interface Invite {
  id: number
  role: string
  note: string
  status: InviteStatus
  expiresAt: string
  usedAt?: string | null
  createdAt: string
}

export interface CreatedInvite {
  token: string
  expiresAt: string
}

export const usersApi = {
  listUsers: () =>
    apiClient.get<ApiResponse<AdminUser[]>>('/api/v1/admin/users').then((res) => unwrapResponse<AdminUser[]>(res)),
  updateUser: (id: string | number, patch: { role?: UserRole; status?: UserStatus }) =>
    apiClient.patch(`/api/v1/admin/users/${id}`, patch).then((res) => unwrapResponse<unknown>(res)),
  resetPassword: (id: string | number) =>
    apiClient
      .post<ApiResponse<{ password: string }>>(`/api/v1/admin/users/${id}/reset-password`)
      .then((res) => unwrapResponse<{ password: string }>(res)),
  listInvites: () =>
    apiClient.get<ApiResponse<Invite[]>>('/api/v1/admin/invites').then((res) => unwrapResponse<Invite[]>(res)),
  createInvite: (input: { role: UserRole; note?: string; expiresInDays?: number }) =>
    apiClient
      .post<ApiResponse<CreatedInvite>>('/api/v1/admin/invites', input)
      .then((res) => unwrapResponse<CreatedInvite>(res)),
  revokeInvite: (id: number) =>
    apiClient.delete(`/api/v1/admin/invites/${id}`).then((res) => unwrapResponse<unknown>(res)),
  /** Casdoor 模式：获取组织注册页链接（admin 引导新用户自助注册）。 */
  getSignupUrl: () =>
    apiClient.get<ApiResponse<{ signupUrl: string }>>('/api/v1/admin/users/signup-url').then((res) => unwrapResponse<{ signupUrl: string }>(res))
}
