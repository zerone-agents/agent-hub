import apiClient, { unwrapResponse } from './client'
import type { ApiResponse } from '@/types/api'

export type AuditCategory = 'auth' | 'user' | 'invite' | 'provider' | 'agent' | 'token' | 'aigc'
export type AuditStatus = 'success' | 'failure' | 'partial'

// id 为十进制字符串（JS number 超 2^53-1 丢精度；rowKey 依赖 id 身份）——
// 全程 string，禁止转 number。
export interface AuditLog {
  id: string
  tenantId: string
  userId: string
  userName: string
  category: AuditCategory
  action: string
  targetType: string
  targetId: string
  targetName: string
  status: AuditStatus
  detail: Record<string, unknown> | null
  remoteIp: string
  userAgent: string
  createdAt: string
}

// page_size 为后端契约 snake_case，其余 camelCase。
export interface AuditLogQuery {
  page?: number
  page_size?: number
  category?: string
  action?: string
  user?: string
  from?: string
  to?: string
  snapshotId?: string
}

export interface AuditLogListResult {
  items: AuditLog[]
  total: number
  snapshotId: string
}

export const auditApi = {
  listLogs: (params: AuditLogQuery) =>
    apiClient
      .get<ApiResponse<AuditLogListResult>>('/api/v1/admin/audit-logs', { params })
      .then((res) => unwrapResponse<AuditLogListResult>(res))
}
