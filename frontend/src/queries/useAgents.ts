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
      void qc.invalidateQueries({ queryKey: ['agents'] })
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
      void qc.invalidateQueries({ queryKey: ['agents'] })
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
      void qc.invalidateQueries({ queryKey: ['agents'] })
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
    onSuccess: (_res, variables) => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      // chat 页 AgentDetailBar 计数来自 ['agents', name, 'detail']：绑定类
      // mutation 必须一并失效，否则保存后详情条不刷新（issue #131 同源）。
      void qc.invalidateQueries({ queryKey: ['agents', variables.name, 'detail'] })
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
    onSuccess: (_res, variables) => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      void qc.invalidateQueries({ queryKey: ['agents', variables.name, 'detail'] })
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
    onSuccess: (_res, variables) => {
      void qc.invalidateQueries({ queryKey: ['agents'] })
      void qc.invalidateQueries({ queryKey: ['agents', variables.name, 'detail'] })
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
      void qc.invalidateQueries({ queryKey: ['agents', variables.name, 'knowledge'] })
      void qc.invalidateQueries({ queryKey: ['agents'] })
      void qc.invalidateQueries({ queryKey: ['agents', variables.name, 'detail'] })
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
