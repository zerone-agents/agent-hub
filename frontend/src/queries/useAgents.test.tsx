import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  useUpdateAgentTools,
  useUpdateAgentSkills,
  useUpdateSubagents,
  useUpdateAgentKnowledgeDatasets,
} from './useAgents'
import { useAgentDetail } from './useAgentDetail'

vi.mock('@/api/agents', () => ({
  agentApi: {
    updateTools: vi.fn(),
    updateSkills: vi.fn(),
    updateSubagents: vi.fn(),
    updateKnowledgeDatasets: vi.fn(),
    getDetail: vi.fn(),
  },
}))

import { agentApi } from '@/api/agents'

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

const detailPayload = {
  data: { id: 'test-agent', name: 'test-agent', status: 'ready', maxTurns: 50, hasSystemPrompt: true },
} as never

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

describe('binding-update mutations refetch agent detail exactly once (P3)', () => {
  beforeEach(() => vi.clearAllMocks())

  for (const c of cases) {
    it(`${c.name}: detail refetched exactly once after mutation (no duplicate invalidate)`, async () => {
      vi.mocked(agentApi.getDetail).mockResolvedValue(detailPayload)
      c.apiMock()

      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const { result } = renderHook(
        () => ({ update: c.useHook(), detail: useAgentDetail('test-agent') }),
        { wrapper: makeWrapper(qc) },
      )
      await waitFor(() => {
        expect(result.current.detail.isSuccess).toBe(true)
      })
      const afterMount = vi.mocked(agentApi.getDetail).mock.calls.length
      expect(afterMount).toBe(1)

      await act(async () => {
        await (result.current.update as { mutateAsync: (v: unknown) => Promise<unknown> }).mutateAsync(c.variables)
      })

      // ['agents'] 前缀匹配已覆盖 detail；显式失效会导致第二次 refetch。
      await waitFor(() => {
        expect(vi.mocked(agentApi.getDetail).mock.calls.length).toBe(afterMount + 1)
      })
    })
  }
})