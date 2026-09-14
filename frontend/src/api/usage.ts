// H7.5 用量与运维 · 前端 API 层。
import apiClient from '@/api/client'

export type UsageKind =
  | 'token'
  | 'model_call'
  | 'message'
  | 'workflow_step'
  | 'extension_call'
  | 'storage'

export interface KindSummary {
  kind: string
  totalCalls: number
  totalTokens: number
  totalCostMicros: number
  totalErrors: number
  avgLatencyMs: number
  deniedCount: number
}

export interface DailyTrend {
  date: string
  kind: string
  calls: number
  tokens: number
  costMicros: number
  errors: number
}

export interface UsageSummary {
  from: string
  to: string
  kinds: KindSummary[]
  trends: DailyTrend[]
}

export interface DimensionRow {
  key: string
  totalCalls: number
  totalTokens: number
  totalCostMicros: number
  totalErrors: number
  avgLatencyMs: number
}

export interface DimensionPage {
  items: DimensionRow[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface UsageErrorRow {
  kind: string
  error: string
  count: number
  lastAt: string
}

export interface StorageRow {
  tableName: string
  rowEstimate: number
  dataLengthBytes: number
}

export interface HealthReport {
  status: 'green' | 'yellow' | 'red'
  db: string
  queueBacklog: number
  queueDropped: number
  todayErrorRatePct: number
  extensionsTotal: number
  extensionsOn: number
  extensionsOff: number
  uptimeSec: number
  checkedAt: string
}

export interface UsageBudget {
  id: number
  kind: string
  limitValue: number
  period: 'daily' | 'monthly'
  alertThresholdPct: number
  createdAt: string
  updatedAt: string
  usedValue: number
  usedPct: number
}

export interface UsageAlertRule {
  id: number
  rule: 'budget_pct' | 'error_rate_spike' | 'health_red'
  channel: 'log' | 'webhook'
  webhookUrl?: string
  threshold: number
  lastFiredAt?: string
  createdAt: string
}

export interface UsageAlertEvent {
  id: number
  alertId: number
  rule: string
  message: string
  createdAt: string
}

export interface UsageRange {
  from?: string
  to?: string
}

export const usageApi = {
  summary: (range: UsageRange) =>
    apiClient.get('/api/v1/admin/usage/summary', { params: range }),
  byExtension: (range: UsageRange & { page?: number; pageSize?: number }) =>
    apiClient.get('/api/v1/admin/usage/by-extension', { params: range }),
  byAgent: (range: UsageRange & { page?: number; pageSize?: number }) =>
    apiClient.get('/api/v1/admin/usage/by-agent', { params: range }),
  byRun: (range: UsageRange & { page?: number; pageSize?: number }) =>
    apiClient.get('/api/v1/admin/usage/by-run', { params: range }),
  byModel: (range: UsageRange & { page?: number; pageSize?: number }) =>
    apiClient.get('/api/v1/admin/usage/by-model', { params: range }),
  errors: (range: UsageRange) => apiClient.get('/api/v1/admin/usage/errors', { params: range }),
  storage: () => apiClient.get('/api/v1/admin/usage/storage'),
  health: () => apiClient.get('/api/v1/admin/usage/health'),
  exportUrl: (range: UsageRange & { kind?: string }) => {
    const params = new URLSearchParams()
    if (range.from) params.set('from', range.from)
    if (range.to) params.set('to', range.to)
    if (range.kind) params.set('kind', range.kind)
    const qs = params.toString()
    return `/api/v1/admin/usage/export${qs ? `?${qs}` : ''}`
  },
  budgets: () => apiClient.get('/api/v1/admin/usage/budgets'),
  upsertBudget: (body: {
    kind: string
    limitValue: number
    period: string
    alertThresholdPct: number
  }) => apiClient.put('/api/v1/admin/usage/budgets', body),
  deleteBudget: (id: number) => apiClient.delete(`/api/v1/admin/usage/budgets/${id}`),
  alerts: () => apiClient.get('/api/v1/admin/usage/alerts'),
  upsertAlert: (body: { rule: string; channel: string; webhookUrl?: string; threshold: number }) =>
    apiClient.put('/api/v1/admin/usage/alerts', body),
  deleteAlert: (id: number) => apiClient.delete(`/api/v1/admin/usage/alerts/${id}`),
  alertEvents: (limit = 50) =>
    apiClient.get('/api/v1/admin/usage/alerts/events', { params: { limit } }),
  getPricing: () => apiClient.get('/api/v1/admin/usage/pricing'),
  putPricing: (value: string) => apiClient.put('/api/v1/admin/usage/pricing', { value })
}

export const USAGE_KIND_LABELS: Record<string, string> = {
  token: 'Token',
  model_call: '模型调用',
  message: '组织消息',
  workflow_step: '工作流步骤',
  extension_call: '扩展调用',
  storage: '存储'
}

// 微分 → 可读金额（分）。
export function formatMicros(micros: number): string {
  if (!micros) return '0'
  const cents = micros / 10000
  if (Math.abs(cents) >= 100) return cents.toFixed(0)
  if (Math.abs(cents) >= 1) return cents.toFixed(2)
  return cents.toFixed(4)
}
