import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import { knowledgeApi, type KnowledgeChunk } from '@/api/knowledge'
import { knowledgeKeys, useCreateChunk, useUpdateChunk } from './useKnowledge'

vi.mock('@/api/knowledge', () => ({ knowledgeApi: { chunks: { create: vi.fn(), update: vi.fn() } } }))
vi.mock('antd', () => ({ message: { success: vi.fn(), error: vi.fn() } }))

const chunk: KnowledgeChunk = { id: 'c1', content: 'saved', document_id: 'd1', important_keywords: [], questions: [], available: true, positions: [], tag_kwd: [], tag_feas: {} }
function setup() {
  const qc = new QueryClient({ defaultOptions: { mutations: { retry: false, onError: () => { message.error('global error') } } } })
  const invalidate = vi.spyOn(qc, 'invalidateQueries')
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={qc}>{children}</QueryClientProvider> }
  return { Wrapper, invalidate }
}

describe('chunk persistence notification ownership', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(knowledgeApi.chunks.create).mockReset()
    vi.mocked(knowledgeApi.chunks.update).mockReset()
  })
  it.each([true, false])('create retains cache invalidation and honors notify=%s for both outcomes', async (notify) => {
    const { Wrapper, invalidate } = setup()
    const { result } = renderHook(() => useCreateChunk('kb1', 'd1', notify ? undefined : { notify: false }), { wrapper: Wrapper })
    vi.mocked(knowledgeApi.chunks.create).mockResolvedValueOnce(chunk).mockRejectedValueOnce(new Error('write rejected'))
    await act(async () => { await result.current.mutateAsync({ content: 'saved' }) })
    expect(message.success).toHaveBeenCalledTimes(notify ? 1 : 0)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: knowledgeKeys.chunks('kb1', 'd1') })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: knowledgeKeys.documents('kb1') })
    await act(async () => { await expect(result.current.mutateAsync({ content: 'saved' })).rejects.toThrow('write rejected') })
    expect(message.error).toHaveBeenCalledTimes(notify ? 1 : 0)
  })
  it.each([true, false])('update retains cache invalidation and honors notify=%s for both outcomes', async (notify) => {
    const { Wrapper, invalidate } = setup()
    const { result } = renderHook(() => useUpdateChunk('kb1', 'd1', notify ? undefined : { notify: false }), { wrapper: Wrapper })
    vi.mocked(knowledgeApi.chunks.update).mockResolvedValueOnce(chunk).mockRejectedValueOnce(new Error('write rejected'))
    const update = { chunkId: 'c1', input: { content: 'saved' } }
    await act(async () => { await result.current.mutateAsync(update) })
    expect(message.success).toHaveBeenCalledTimes(notify ? 1 : 0)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: knowledgeKeys.chunks('kb1', 'd1') })
    await act(async () => { await expect(result.current.mutateAsync(update)).rejects.toThrow('write rejected') })
    expect(message.error).toHaveBeenCalledTimes(notify ? 1 : 0)
  })
})
