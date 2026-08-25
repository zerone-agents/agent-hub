import { useAgents } from './useAgents'
import { useTools } from './useTools'
import { useSkills } from './useSkills'
import { useScenes } from './useScenes'
import { useProviders } from './useProviders'
import { useMcps } from './useMcps'
import { useKnowledgeList } from './useKnowledge'
import { useChatSessions } from './useChat'
import type { Agent } from '@/api/agents'
import type { Tool } from '@/api/tools'
import type { Skill } from '@/api/skills'
import type { Scene } from '@/api/scenes'
import type { Provider } from '@/api/providers'
import type { Mcp } from '@/api/mcps'
import type { KnowledgeDataset } from '@/api/knowledge'
import type { ChatSession } from '@/api/chat'

export interface DashboardStats {
  agents: Agent[]
  tools: Tool[]
  skills: Skill[]
  scenes: Scene[]
  providers: Provider[]
  mcps: Mcp[]
  knowledgeDatasets: KnowledgeDataset[]
  chatSessions: ChatSession[]
  chatSessionTotal: number
  isLoading: boolean
  isError: boolean
  refetch: () => Promise<void>
}

/**
 * Aggregate the dashboard resource and operational queries.
 * Each underlying query shares its TanStack Query cache with the individual
 * useXxx hooks, so navigating to a list page later will reuse the same data.
 */
export function useDashboardStats(): DashboardStats {
  const agents = useAgents()
  const tools = useTools()
  const skills = useSkills()
  const scenes = useScenes()
  const providers = useProviders()
  const mcps = useMcps()
  const knowledge = useKnowledgeList({ page: 1, page_size: 1000 })
  const chat = useChatSessions()

  // The knowledge base is an optional module: when MultiRAG is not configured
  // the API returns 503, and a KB outage must not take down the whole
  // dashboard. Exclude it from the fatal loading/error gates so the page
  // degrades to zero knowledge stats instead of failing entirely.
  const isLoading =
    agents.isLoading ||
    tools.isLoading ||
    skills.isLoading ||
    scenes.isLoading ||
    providers.isLoading ||
    mcps.isLoading ||
    chat.isLoading
  const isError =
    agents.isError ||
    tools.isError ||
    skills.isError ||
    scenes.isError ||
    providers.isError ||
    mcps.isError ||
    chat.isError

  const refetch = async () => {
    await Promise.allSettled([
      agents.refetch(),
      tools.refetch(),
      skills.refetch(),
      scenes.refetch(),
      providers.refetch(),
      mcps.refetch(),
      knowledge.refetch(),
      chat.refetch()
    ])
  }

  return {
    agents: agents.data ?? [],
    tools: tools.data ?? [],
    skills: skills.data ?? [],
    scenes: scenes.data ?? [],
    providers: providers.data ?? [],
    mcps: mcps.data ?? [],
    knowledgeDatasets: knowledge.data?.datasets ?? [],
    chatSessions: chat.data?.items ?? [],
    chatSessionTotal: chat.data?.total ?? 0,
    isLoading,
    isError,
    refetch
  }
}

// Re-export the domain types so the dashboard can consume them without
// reaching into multiple api/* modules.
export type { Agent, Tool, Skill, Scene, Provider, Mcp, KnowledgeDataset, ChatSession }
