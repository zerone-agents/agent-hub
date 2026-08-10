import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { agentChatApi, type AgentChatListParams, type AgentChatSession } from '@/api/agent-chat'
import type { ChatMessage } from '@/api/chat'
import { parseApiError, unwrapResponse } from '@/api/client'

interface SessionListResult {
  items: AgentChatSession[]
  total: number
}
interface MessageListResult {
  items: ChatMessage[]
  total: number
}
const EMPTY_SESSIONS: SessionListResult = { items: [], total: 0 }
const EMPTY_MESSAGES: MessageListResult = { items: [], total: 0 }

export function useAgentChatSessions(agentName: string, params: AgentChatListParams = {}) {
  const queryKey = ['agent-chat-sessions', agentName, { source: 'agent_chat_page', ...params }]
  return useQuery({
    queryKey,
    queryFn: async () => {
      const res = await agentChatApi.listSessions(agentName, { source: 'agent_chat_page', ...params })
      return unwrapResponse<SessionListResult | null>(res) ?? EMPTY_SESSIONS
    },
    enabled: !!agentName,
  })
}

export function useAgentChatMessages(agentName: string, sessionId: string | null) {
  return useQuery({
    queryKey: ['agent-chat-messages', agentName, sessionId],
    queryFn: async () => {
      if (!sessionId) return EMPTY_MESSAGES
      const res = await agentChatApi.listMessages(agentName, sessionId)
      return unwrapResponse<MessageListResult | null>(res) ?? EMPTY_MESSAGES
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
