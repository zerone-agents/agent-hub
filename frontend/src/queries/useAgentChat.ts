import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { agentChatApi, type AgentChatListParams } from '@/api/agent-chat'
import { parseApiError } from '@/api/client'

export function useAgentChatSessions(agentName: string, params: AgentChatListParams = {}) {
  const queryKey = ['agent-chat-sessions', agentName, { source: 'agent_chat_page', ...params }]
  return useQuery({
    queryKey,
    queryFn: async () => {
      const res = await agentChatApi.listSessions(agentName, { source: 'agent_chat_page', ...params })
      const payload = res.data?.data ?? { items: [], total: 0 }
      return payload as { items: any[]; total: number }
    },
    enabled: !!agentName,
  })
}

export function useAgentChatMessages(agentName: string, sessionId: string | null) {
  return useQuery({
    queryKey: ['agent-chat-messages', agentName, sessionId],
    queryFn: async () => {
      if (!sessionId) return { items: [], total: 0 }
      const res = await agentChatApi.listMessages(agentName, sessionId)
      return (res.data?.data ?? { items: [], total: 0 }) as { items: any[]; total: number }
    },
    enabled: !!sessionId,
  })
}

export function useCreateAgentChatSession(agentName: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (title?: string) => agentChatApi.createSession(agentName, title),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent-chat-sessions', agentName] }),
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useDeleteAgentChatSession(agentName: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (sessionId: string) => agentChatApi.deleteSession(agentName, sessionId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent-chat-sessions', agentName] }),
    onError: (err) => message.error(parseApiError(err)),
  })
}
