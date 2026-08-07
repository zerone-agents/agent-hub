import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { mcpApi, type Mcp, type McpDetail, type McpInput, type McpProbeInput, type McpProbeResult } from '@/api/mcps'
import { parseApiError } from '@/api/client'

export function useMcps() {
  return useQuery<Mcp[]>({
    queryKey: ['mcps'],
    queryFn: async () => {
      const res = await mcpApi.list()
      if (!res.data.success) throw new Error(res.data.message)
      return res.data.data ?? []
    }
  })
}

export function useMcp(name: string | null) {
  return useQuery<McpDetail>({
    queryKey: ['mcp', name],
    queryFn: async () => {
      const res = await mcpApi.get(name!)
      if (!res.data.success) throw new Error(res.data.message)
      return res.data.data
    },
    enabled: !!name
  })
}

export function useCreateMcp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: McpInput) => mcpApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success('MCP 已创建')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateMcp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: McpInput }) =>
      mcpApi.update(name, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success('MCP 已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteMcp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => mcpApi.delete(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success('MCP 已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useAgentMcps(agentName: string | null) {
  return useQuery<string[]>({
    queryKey: ['agent-mcps', agentName],
    queryFn: async () => {
      const res = await mcpApi.getAgentMcps(agentName!)
      if (!res.data.success) throw new Error(res.data.message)
      return res.data.data ?? []
    },
    enabled: !!agentName
  })
}

export function useUpdateAgentMcps() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentName, mcpNames }: { agentName: string; mcpNames: string[] }) =>
      mcpApi.updateAgentMcps(agentName, mcpNames),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent-mcps'] })
      message.success('Agent MCP 关系已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useProbeMcp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ name, config }: { name?: string; config?: McpProbeInput }) => {
      const res = name ? await mcpApi.probeByName(name) : await mcpApi.probeByConfig(config!)
      return res.data.data as McpProbeResult
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['mcps'] })
      if (data.status === 'success') {
        message.success('探测完成')
      } else {
        message.error(data.error ?? '探测失败')
      }
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
