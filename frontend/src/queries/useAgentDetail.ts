import { useQuery } from '@tanstack/react-query'
import { agentApi, type AgentDetail } from '@/api/agents'

/**
 * Fetches agent detail (model, tools, MCP, skills, subagents, datasets)
 * from the control-panel proxy of runtime GET /v1/agents/:agentId.
 *
 * Stale time is 5 minutes because agent config rarely changes during a
 * chat session. On error the consuming component silently hides, so we
 * retry once to absorb transient runtime hiccups before giving up.
 */
export function useAgentDetail(name: string) {
  return useQuery<AgentDetail>({
    queryKey: ['agents', name, 'detail'],
    queryFn: async () => {
      // Backend naked-passes runtime JSON (no {success, data} wrapping).
      const res = await agentApi.getDetail(name)
      return res.data as AgentDetail
    },
    enabled: !!name,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })
}
