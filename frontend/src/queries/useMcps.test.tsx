import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useUpdateAgentMcps } from './useMcps'

vi.mock('@/api/mcps', () => ({
  mcpApi: {
    updateAgentMcps: vi.fn(),
  },
}))

import { mcpApi } from '@/api/mcps'

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useUpdateAgentMcps', () => {
  beforeEach(() => vi.clearAllMocks())

  it('invalidates the agent list after updating MCP bindings so AgentCard counts refresh immediately', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const spy = vi.spyOn(qc, 'invalidateQueries')
    vi.mocked(mcpApi.updateAgentMcps).mockResolvedValue({ success: true } as never)

    const { result } = renderHook(() => useUpdateAgentMcps(), { wrapper: makeWrapper(qc) })
    // 不 await：onSuccess 的 invalidate 同步执行于 mutate 完成后
    await result.current.mutateAsync({ agentName: 'test-agent', mcpNames: ['web-search-prime'] })

    // AgentCard 的 "N MCP" 计数来自 ['agents']（useAgents）：先前漏掉导致保存后不刷新
    expect(spy).toHaveBeenCalledWith({ queryKey: ['agents'] })
    // chat 页 AgentDetailBar 计数来自 ['agents', name, 'detail']：所有绑定类 mutation 均漏
    expect(spy).toHaveBeenCalledWith({ queryKey: ['agents', 'test-agent', 'detail'] })
    // 原有 agent-mcp 列表失效保留
    expect(spy).toHaveBeenCalledWith({ queryKey: ['agent-mcps'] })
  })
})