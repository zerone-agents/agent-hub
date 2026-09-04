import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useKnowledgeListAll } from './useKnowledge'

vi.mock('@/api/knowledge', () => ({
  knowledgeApi: {
    datasets: {
      list: vi.fn(),
    },
  },
}))

import { knowledgeApi } from '@/api/knowledge'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

// issue #122 review P2：ghost 判定依赖完整目录——useKnowledgeListAll 必须
// 分页取全；任一页失败即整体失败（liveness 未知，消费方禁用 ghost）。
describe('useKnowledgeListAll', () => {
  beforeEach(() => vi.clearAllMocks())

  it('returns the first page as-is when total fits one page', async () => {
    const datasets = Array.from({ length: 3 }, (_, i) => ({ id: `kb-${i}`, name: `库${i}` }))
    ;(knowledgeApi.datasets.list as any).mockResolvedValue({ total: 3, datasets })

    const { result } = renderHook(() => useKnowledgeListAll(), { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
    expect(result.current.data?.datasets).toHaveLength(3)
    expect(result.current.data?.total).toBe(3)
    expect(knowledgeApi.datasets.list).toHaveBeenCalledTimes(1)
    expect(knowledgeApi.datasets.list).toHaveBeenCalledWith({ page: 1, page_size: 1000 })
  })

  it('fetches every page until the catalog is complete', async () => {
    const page1 = Array.from({ length: 1000 }, (_, i) => ({ id: `kb-${i}` }))
    const page2 = Array.from({ length: 500 }, (_, i) => ({ id: `kb-${1000 + i}` }))
    ;(knowledgeApi.datasets.list as any)
      .mockResolvedValueOnce({ total: 1500, datasets: page1 })
      .mockResolvedValueOnce({ total: 1500, datasets: page2 })

    const { result } = renderHook(() => useKnowledgeListAll(), { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
    expect(result.current.data?.datasets).toHaveLength(1500)
    expect(result.current.data?.total).toBe(1500)
    expect(knowledgeApi.datasets.list).toHaveBeenCalledTimes(2)
    expect(knowledgeApi.datasets.list).toHaveBeenCalledWith({ page: 2, page_size: 1000 })
  })

  it('surfaces an error when any page fails', async () => {
    const page1 = Array.from({ length: 1000 }, (_, i) => ({ id: `kb-${i}` }))
    ;(knowledgeApi.datasets.list as any)
      .mockResolvedValueOnce({ total: 1500, datasets: page1 })
      .mockRejectedValueOnce(new Error('boom'))

    const { result } = renderHook(() => useKnowledgeListAll(), { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
  })
})
