import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import {
  agentRelationApi,
  type AgentRelation,
  type AgentRelationCreatePayload,
  type AgentRelationEvent,
  type AgentRelationUpdatePayload,
  type RecordAgentRelationEventPayload,
} from '@/api/agent-relations'
import { parseApiError, unwrapResponse } from '@/api/client'

export function useAgentRelations() {
  return useQuery<AgentRelation[]>({
    queryKey: ['agent-relations'],
    queryFn: async () => unwrapResponse<AgentRelation[]>(await agentRelationApi.list()),
  })
}

export function useCreateAgentRelation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: AgentRelationCreatePayload) => agentRelationApi.create(data),
    onSuccess: (_response, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['agent-relations'] })
      message.success(variables.bidirectional ? '双向关系已创建' : '关系已创建')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useUpdateAgentRelation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: AgentRelationUpdatePayload }) => agentRelationApi.update(id, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['agent-relations'] })
      message.success('关系已更新')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useDeleteAgentRelation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => agentRelationApi.delete(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['agent-relations'] })
      message.success('关系已删除')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useAgentRelationEvents(relationId: number | null) {
  return useQuery<AgentRelationEvent[]>({
    queryKey: ['agent-relation-events', relationId],
    queryFn: async () => {
      if (relationId === null) return []
      return unwrapResponse<AgentRelationEvent[]>(await agentRelationApi.events(relationId))
    },
    enabled: relationId !== null,
  })
}

export function useRecordAgentRelationEvent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: RecordAgentRelationEventPayload }) =>
      agentRelationApi.recordEvent(id, data),
    onSuccess: (_response, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['agent-relations'] })
      void queryClient.invalidateQueries({
        queryKey: ['agent-relation-events', variables.id],
      })
      message.success('关系事件已记录')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}
