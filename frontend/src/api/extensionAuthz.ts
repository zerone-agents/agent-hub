// H7.4 扩展权限与审计的管理 API 客户端类型与请求封装。
import apiClient from './client'

export interface ExtensionGrant {
  id: number
  extensionName: string
  permission: string
  scope: string
  actions: string
  resource: string
  status: string // active / pending
  mode: string // auto / approval
  isActive: boolean
  sourceVersion: string
  grantedBy: string
  grantedAt: string
  revokedAt?: string
  lastSyncAt: string
}

export interface ExtensionGrantListResult {
  items: ExtensionGrant[]
}

export interface ExtensionAccessAuditItem {
  id: number
  extensionName: string
  permission: string
  scope: string
  action: string
  resource: string
  allowed: boolean
  deniedReason: string
  ip: string
  createdAt: string
}

export interface ExtensionAuditListResult {
  items: ExtensionAccessAuditItem[]
  total: number
  page: number
  pageSize: number
}

export const extensionAuthzApi = {
  grants: (id: number) =>
    apiClient.get<ExtensionGrantListResult>(`/api/v1/admin/extensions/${id}/grants`),
  revokeGrant: (id: number, grantId: number) =>
    apiClient.delete(`/api/v1/admin/extensions/${id}/grants/${grantId}`),
  approveGrant: (id: number, grantId: number) =>
    apiClient.post(`/api/v1/admin/extensions/${id}/grants/${grantId}/approve`),
  audit: (id: number, params: { page: number; pageSize: number; allowed?: boolean }) =>
    apiClient.get<ExtensionAuditListResult>(`/api/v1/admin/extensions/${id}/audit`, { params })
}
