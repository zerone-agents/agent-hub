// H7.5 用量与运维的 react-query hooks。
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { unwrapResponse } from '@/api/client'
import {
  usageApi,
  type DimensionPage,
  type HealthReport,
  type StorageRow,
  type UsageAlertEvent,
  type UsageAlertRule,
  type UsageBudget,
  type UsageErrorRow,
  type UsageRange,
  type UsageSummary
} from '@/api/usage'

export function useUsageSummary(range: UsageRange) {
  return useQuery<UsageSummary>({
    queryKey: ['usage', 'summary', range],
    queryFn: async () => unwrapResponse<UsageSummary>(await usageApi.summary(range))
  })
}

export function useUsageByExtension(range: UsageRange) {
  return useQuery<DimensionPage>({
    queryKey: ['usage', 'by-extension', range],
    queryFn: async () => unwrapResponse<DimensionPage>(await usageApi.byExtension(range))
  })
}

export function useUsageByAgent(range: UsageRange) {
  return useQuery<DimensionPage>({
    queryKey: ['usage', 'by-agent', range],
    queryFn: async () => unwrapResponse<DimensionPage>(await usageApi.byAgent(range))
  })
}

export function useUsageByRun(range: UsageRange) {
  return useQuery<DimensionPage>({
    queryKey: ['usage', 'by-run', range],
    queryFn: async () => unwrapResponse<DimensionPage>(await usageApi.byRun(range))
  })
}

export function useUsageByModel(range: UsageRange) {
  return useQuery<DimensionPage>({
    queryKey: ['usage', 'by-model', range],
    queryFn: async () => unwrapResponse<DimensionPage>(await usageApi.byModel(range))
  })
}

export function useUsageErrors(range: UsageRange) {
  return useQuery<{ items: UsageErrorRow[] }>({
    queryKey: ['usage', 'errors', range],
    queryFn: async () => unwrapResponse<{ items: UsageErrorRow[] }>(await usageApi.errors(range))
  })
}

export function useUsageStorage() {
  return useQuery<{ items: StorageRow[] }>({
    queryKey: ['usage', 'storage'],
    queryFn: async () => unwrapResponse<{ items: StorageRow[] }>(await usageApi.storage())
  })
}

export function useUsageHealth() {
  return useQuery<HealthReport>({
    queryKey: ['usage', 'health'],
    queryFn: async () => unwrapResponse<HealthReport>(await usageApi.health()),
    refetchInterval: 30000
  })
}

export function useUsageBudgets() {
  return useQuery<{ items: UsageBudget[] }>({
    queryKey: ['usage', 'budgets'],
    queryFn: async () => unwrapResponse<{ items: UsageBudget[] }>(await usageApi.budgets())
  })
}

export function useUpsertUsageBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: {
      kind: string
      limitValue: number
      period: string
      alertThresholdPct: number
    }) => unwrapResponse(await usageApi.upsertBudget(body)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['usage', 'budgets'] })
    }
  })
}

export function useDeleteUsageBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => unwrapResponse(await usageApi.deleteBudget(id)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['usage', 'budgets'] })
    }
  })
}

export function useUsageAlerts() {
  return useQuery<{ items: UsageAlertRule[] }>({
    queryKey: ['usage', 'alerts'],
    queryFn: async () => unwrapResponse<{ items: UsageAlertRule[] }>(await usageApi.alerts())
  })
}

export function useUpsertUsageAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: {
      rule: string
      channel: string
      webhookUrl?: string
      threshold: number
    }) => unwrapResponse(await usageApi.upsertAlert(body)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['usage', 'alerts'] })
    }
  })
}

export function useDeleteUsageAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => unwrapResponse(await usageApi.deleteAlert(id)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['usage', 'alerts'] })
    }
  })
}

export function useUsageAlertEvents(limit = 50) {
  return useQuery<{ items: UsageAlertEvent[] }>({
    queryKey: ['usage', 'alert-events', limit],
    queryFn: async () =>
      unwrapResponse<{ items: UsageAlertEvent[] }>(await usageApi.alertEvents(limit))
  })
}
