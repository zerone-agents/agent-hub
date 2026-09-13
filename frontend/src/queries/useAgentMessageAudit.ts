import { useQuery } from '@tanstack/react-query'
import { unwrapResponse } from '@/api/client'
import { agentMessageAuditApi, type AgentMessageChain, type MessageChainSelector } from '@/api/agent-message-audit'

export function useAgentMessageChain(selector?: MessageChainSelector) {
  return useQuery<AgentMessageChain>({
    queryKey: ['agent-message-chain', selector?.kind, selector?.value],
    queryFn: async () => unwrapResponse<AgentMessageChain>(await agentMessageAuditApi.getChain(selector as MessageChainSelector)),
    enabled: Boolean(selector?.value),
    refetchInterval: ({ state }) => state.data?.messages.some((item) => item.status === 'queued' || item.status === 'running') ? 1500 : false,
  })
}
