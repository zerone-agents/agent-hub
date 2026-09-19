import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { agentApi, type Agent } from '@/api/agents'
import { parseApiError, unwrapResponse } from '@/api/client'
import { useTranslation } from 'react-i18next'

export function useAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: async () => {
      return unwrapResponse<{ agents?: Agent[] }>(await agentApi.list()).agents ?? []
    }
  })
}

/**
 * 聊天视图公开 Agent 列表（/api/v1/agents?view=chat）。
 * 独立 query key 与管理端 ['agents'] 隔离，避免 admin/public 缓存互串。
 * guest 用户拿到的即服务端 guestEnabled 过滤后列表；不能复用 useAgents()
 * （admin 端点，guest 必 403，spec 6.1）。
 */
export function usePublicAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents', 'public'],
    queryFn: async () =>
      unwrapResponse<{ agents?: Agent[] }>(await agentApi.publicList()).agents ?? []
  })
}

export function useCreateAgent() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Agent>) => agentApi.create(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      message.success(t('agents.toast.agentCreated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateAgent() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: Partial<Agent> }) =>
      agentApi.update(name, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      message.success(t('agents.toast.agentUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteAgent() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => agentApi.delete(name),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      message.success(t('agents.toast.agentDeleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateSubagents() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, subagents }: { name: string; subagents: string[] }) =>
      agentApi.updateSubagents(name, subagents),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      // v5 前缀匹配：['agents'] 已覆盖 ['agents', name, 'detail']，
      // 无需显式失效 detail（避免重复 refetch）。
      message.success(t('agents.toast.subagentUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateAgentTools() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, toolNames }: { name: string; toolNames: string[] }) =>
      agentApi.updateTools(name, toolNames),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      // v5 前缀匹配：['agents'] 已覆盖 ['agents', name, 'detail']，无需显式失效。
      message.success(t('agents.toast.toolsUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateAgentSkills() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, skillNames }: { name: string; skillNames: string[] }) =>
      agentApi.updateSkills(name, skillNames),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      // v5 前缀匹配：['agents'] 已覆盖 ['agents', name, 'detail']，无需显式失效。
      message.success(t('agents.toast.skillsUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useAgentKnowledgeDatasets(name: string) {
  return useQuery<string[]>({
    queryKey: ['agents', name, 'knowledge'],
    queryFn: async () => {
      return unwrapResponse<{ dataset_ids?: string[] }>(await agentApi.getKnowledgeDatasets(name)).dataset_ids ?? []
    },
    enabled: !!name
  })
}

export function useUpdateAgentKnowledgeDatasets() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, datasetIds }: { name: string; datasetIds: string[] }) =>
      agentApi.updateKnowledgeDatasets(name, datasetIds),
    onSuccess: (_res, variables) => {
      void qc.invalidateQueries({ queryKey: ['agents', variables.name, 'knowledge'] })
      void qc.invalidateQueries({ queryKey: ['agents'] })
      // v5 前缀匹配：['agents'] 已覆盖 ['agents', name, 'detail']，无需显式失效。
      message.success(t('agents.toast.knowledgeUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useProbeAgent() {
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: { providerId?: number; apiKey: string; baseUrl: string } }) =>
      agentApi.probe(name, data),
    onError: (err) => message.error(parseApiError(err))
  })
}
