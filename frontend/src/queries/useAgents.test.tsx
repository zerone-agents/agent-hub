import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  useUpdateAgentTools,
  useUpdateAgentSkills,
  useUpdateSubagents,
  useUpdateAgentKnowledgeDatasets,
} from './useAgents'

vi.mock('@/api/agents', () => ({
  agentApi: {
    updateTools: vi.fn(),
    updateSkills: vi.fn(),
    updateSubagents: vi.fn(),
    updateKnowledgeDatasets: vi.fn(),
  },
}))

import { agentApi } from '@/api/agents'

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('binding-update mutations invalidate agent detail (issue #131 同源)', () => {
  beforeEach(() => vi.clearAllMocks())

  const cases: {
    name: string
    useHook: () => { mutateAsync: (v: never) => Promise<unknown> }
    apiMock: () => unknown
    variables: never
  }[] = [
    {
      name: 'useUpdateAgentTools',
      useHook: () => useUpdateAgentTools() as never,
      apiMock: () => vi.mocked(agentApi.updateTools).mockResolvedValue({ success: true } as never),
      variables: { name: 'test-agent', toolNames: ['Bash'] } as never,
    },
    {
      name: 'useUpdateAgentSkills',
      useHook: () => useUpdateAgentSkills() as never,
      apiMock: () => vi.mocked(agentApi.updateSkills).mockResolvedValue({ success: true } as never),
      variables: { name: 'test-agent', skillNames: ['s1'] } as never,
    },
    {
      name: 'useUpdateSubagents',
      useHook: () => useUpdateSubagents() as never,
      apiMock: () => vi.mocked(agentApi.updateSubagents).mockResolvedValue({ success: true } as never),
      variables: { name: 'test-agent', subagents: ['writer'] } as never,
    },
    {
      name: 'useUpdateAgentKnowledgeDatasets',
      useHook: () => useUpdateAgentKnowledgeDatasets() as never,
      apiMock: () => vi.mocked(agentApi.updateKnowledgeDatasets).mockResolvedValue({ success: true } as never),
      variables: { name: 'test-agent', datasetIds: ['kb-1'] } as never,
    },
  ]

  for (const c of cases) {
    it(`${c.name} invalidates ['agents', name, 'detail'] in addition to ['agents']`, async () => {
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const spy = vi.spyOn(qc, 'invalidateQueries')
      c.apiMock()

      const { result } = renderHook(c.useHook, { wrapper: makeWrapper(qc) })
      await (result.current as { mutateAsync: (v: unknown) => Promise<unknown> }).mutateAsync(c.variables)

      // chat 页 AgentDetailBar 计数来自 ['agents', name, 'detail']：绑定类 mutation 必须失效
      expect(spy).toHaveBeenCalledWith({ queryKey: ['agents', 'test-agent', 'detail'] })
      // 列表卡片计数来源保持失效
      expect(spy).toHaveBeenCalledWith({ queryKey: ['agents'] })
    })
  }
})