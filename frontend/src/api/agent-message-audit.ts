import apiClient from './client'
import type { AgentMessage } from './runs'

export interface AgentMessageAuditItem extends AgentMessage {
  sourceAgentId: number
  targetAgentId: number
  routeMode?: string
  routePlanned?: boolean
  routeDeviation?: string
  verified: boolean
  verification: string
}

export interface AgentMessageChain {
  conversationId: string
  rootMessageId: string
  messages: AgentMessageAuditItem[]
  messageCount: number
  verified: boolean
}

export type MessageChainSelector =
  | { kind: 'conversation'; value: string }
  | { kind: 'root'; value: string }

export const agentMessageAuditApi = {
  getChain: (selector: MessageChainSelector) => apiClient.get('/api/v1/admin/agent-message-chains', {
    params: selector.kind === 'conversation'
      ? { conversation_id: selector.value, limit: 100 }
      : { root_message_id: selector.value, limit: 100 },
  }),
}
