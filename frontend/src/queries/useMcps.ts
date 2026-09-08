import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { mcpApi, type Mcp, type McpDetail, type McpInput, type McpProbeInput, type McpProbeResult } from '@/api/mcps'
import { parseApiError, unwrapResponse } from '@/api/client'

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
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: McpInput) => mcpApi.create(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['mcps'] })
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
      void qc.invalidateQueries({ queryKey: ['mcps'] })
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
      void qc.invalidateQueries({ queryKey: ['mcps'] })
      message.success('MCP 已删除')
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
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentName, mcpNames }: { agentName: string; mcpNames: string[] }) =>
      mcpApi.updateAgentMcps(agentName, mcpNames),
    onSuccess: (_res, variables) => {
      void qc.invalidateQueries({ queryKey: ['agent-mcps'] })
      // AgentCard 的 "N MCP" 计数来自 ['agents']（useAgents）；chat 页
      // AgentDetailBar 计数来自 ['agents', name, 'detail']。两者都必须
      // 立即失效，否则绑定修改后数量不刷新（issue #131 同源可观测性）。
      void qc.invalidateQueries({ queryKey: ['agents'] })
      void qc.invalidateQueries({ queryKey: ['agents', variables.agentName, 'detail'] })
      message.success('Agent MCP 关系已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useProbeMcp() {
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
        message.success('探测完成')
      } else {
        message.error(data.error ?? '探测失败')
      }
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
