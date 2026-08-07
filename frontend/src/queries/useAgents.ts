import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { agentApi, type Agent } from '@/api/agents'
import { parseApiError, unwrapResponse } from '@/api/client'

export function useAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: async () => {
      return unwrapResponse<{ agents?: Agent[] }>(await agentApi.list()).agents ?? []
    }
  })
}

export function useCreateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Agent>) => agentApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('代理已创建')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: Partial<Agent> }) =>
      agentApi.update(name, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('代理已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => agentApi.delete(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('代理已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateSubagents() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, subagents }: { name: string; subagents: string[] }) =>
      agentApi.updateSubagents(name, subagents),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('子代理已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateAgentTools() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, toolNames }: { name: string; toolNames: string[] }) =>
      agentApi.updateTools(name, toolNames),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('工具已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateAgentSkills() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, skillNames }: { name: string; skillNames: string[] }) =>
      agentApi.updateSkills(name, skillNames),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('技能已更新')
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
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, datasetIds }: { name: string; datasetIds: string[] }) =>
      agentApi.updateKnowledgeDatasets(name, datasetIds),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ['agents', variables.name, 'knowledge'] })
      qc.invalidateQueries({ queryKey: ['agents'] })
      message.success('知识库已更新')
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
