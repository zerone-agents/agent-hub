import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { mcpApi, type Mcp, type McpDetail, type McpInput, type McpProbeInput, type McpProbeResult } from '@/api/mcps'
import { parseApiError, unwrapResponse } from '@/api/client'
import { useTranslation } from 'react-i18next'

export function useMcps() {
  return useQuery<Mcp[]>({
    queryKey: ['mcps'],
    queryFn: async () => unwrapResponse<Mcp[]>(await mcpApi.list())
  })
}

export function useMcp(name: string | null) {
  return useQuery<McpDetail>({
    queryKey: ['mcp', name],
    queryFn: async () => {
      // enabled gate guarantees name is non-null at call time
      if (name === null) throw new Error('useMcp: name is null despite enabled gate')
      return unwrapResponse<McpDetail>(await mcpApi.get(name))
    },
    enabled: !!name
  })
}

export function useCreateMcp() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: McpInput) => mcpApi.create(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success(t('mcps.toast.created'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateMcp() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: McpInput }) =>
      mcpApi.update(name, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success(t('mcps.toast.updated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteMcp() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => mcpApi.delete(name),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success(t('mcps.toast.deleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useAgentMcps(agentName: string | null) {
  return useQuery<string[]>({
    queryKey: ['agent-mcps', agentName],
    queryFn: async () => {
      // enabled gate guarantees agentName is non-null at call time
      if (agentName === null) throw new Error('useAgentMcps: agentName is null despite enabled gate')
      return unwrapResponse<string[]>(await mcpApi.getAgentMcps(agentName))
    },
    enabled: !!agentName
  })
}

export function useUpdateAgentMcps() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentName, mcpNames }: { agentName: string; mcpNames: string[] }) =>
      mcpApi.updateAgentMcps(agentName, mcpNames),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agent-mcps'] })
      // AgentCard 的 "N MCP" 计数来自 ['agents']（useAgents）。v5 invalidate
      // 默认前缀匹配：['agents'] 同时覆盖 ['agents', name, 'detail']（chat 页
      // AgentDetailBar）——不必再显式失效 detail，否则触发重复 refetch。
      void qc.invalidateQueries({ queryKey: ['agents'] })
      message.success(t('mcps.toast.agentsUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useProbeMcp() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ name, config }: { name?: string; config?: McpProbeInput }) => {
      // Caller must supply exactly one of name/config; throw otherwise.
      if (name) return unwrapResponse<McpProbeResult>(await mcpApi.probeByName(name))
      if (!config) throw new Error('useProbeMcp: name or config is required')
      return unwrapResponse<McpProbeResult>(await mcpApi.probeByConfig(config))
    },
    onSuccess: (data) => {
      void qc.invalidateQueries({ queryKey: ['mcps'] })
      if (data.status === 'success') {
        message.success(t('mcps.toast.probeDone'))
      } else {
        message.error(data.error ?? t('mcps.toast.probeFailed'))
      }
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
