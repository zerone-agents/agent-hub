import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useUpdateAgentMcps } from './useMcps'
import { useAgentDetail } from './useAgentDetail'

vi.mock('@/api/mcps', () => ({
  mcpApi: { updateAgentMcps: vi.fn() },
}))
vi.mock('@/api/agents', () => ({
  agentApi: {
    updateTools: vi.fn(),
    updateSkills: vi.fn(),
    updateSubagents: vi.fn(),
    updateKnowledgeDatasets: vi.fn(),
    getDetail: vi.fn(),
    list: vi.fn(),
  },
}))

import { mcpApi } from '@/api/mcps'
import { agentApi } from '@/api/agents'

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useUpdateAgentMcps invalidation', () => {
  beforeEach(() => vi.clearAllMocks())

  it('invalidates the ["agents"] list so AgentCard counts refresh immediately (P3 keep)', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const spy = vi.spyOn(qc, 'invalidateQueries')
    vi.mocked(mcpApi.updateAgentMcps).mockResolvedValue({ success: true } as never)

    const { result } = renderHook(() => useUpdateAgentMcps(), { wrapper: makeWrapper(qc) })
    await result.current.mutateAsync({ agentName: 'test-agent', mcpNames: ['web-search-prime'] })

    // AgentCard 的 "N MCP" 计数来自 ['agents']（useAgents）：必须失效
    expect(spy).toHaveBeenCalledWith({ queryKey: ['agents'] })
    // 原有 agent-mcp 列表失效保留
    expect(spy).toHaveBeenCalledWith({ queryKey: ['agent-mcps'] })
  })

  it('refetches the detail query exactly once after mutation (P3: no duplicate invalidation)', async () => {
    vi.mocked(mcpApi.updateAgentMcps).mockResolvedValue({ success: true } as never)
    // getDetail 计数：初始挂载 1 次 + 失效触发的 refetch 必须恰好 1 次
    vi.mocked(agentApi.getDetail).mockResolvedValue({
      data: { id: 'test-agent', name: 'test-agent', status: 'ready', maxTurns: 50, hasSystemPrompt: true },
    } as never)

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => ({ update: useUpdateAgentMcps(), detail: useAgentDetail('test-agent') }),
      { wrapper: makeWrapper(qc) },
    )
    await waitFor(() => {
      expect(result.current.detail.isSuccess).toBe(true)
    })
    const callsAfterMount = vi.mocked(agentApi.getDetail).mock.calls.length
    expect(callsAfterMount).toBe(1)

    await act(async () => { await result.current.update.mutateAsync({ agentName: 'test-agent', mcpNames: ['web-search-prime'] }) })

    // invalidateQueries(['agents']) 前缀匹配已覆盖 ['agents','test-agent','detail']；
    // 若再显式失效 detail 会触发第二次 refetch —— 回归锁：必须恰好 +1
    await waitFor(() => {
      expect(vi.mocked(agentApi.getDetail).mock.calls.length).toBe(callsAfterMount + 1)
    })
  })
})