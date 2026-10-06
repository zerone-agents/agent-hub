import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi, beforeEach } from 'vitest'
import { message } from 'antd'
import { knowledgeApi, type KnowledgeDataset, type KnowledgeWriteOwner } from '@/api/knowledge'
import { knowledgeKeys, useCreateKnowledge, useUpdateKnowledge } from './useKnowledge'

vi.mock('@/api/knowledge', () => ({ knowledgeApi: { datasets: { update: vi.fn(), create: vi.fn() } } }))
// A notification can remain visible after the request has already failed.
vi.mock('antd', () => ({ message: { success: vi.fn(), error: vi.fn(() => new Promise<never>(() => undefined)) } }))

beforeEach(() => { vi.clearAllMocks(); vi.mocked(knowledgeApi.datasets.update).mockReset(); vi.mocked(knowledgeApi.datasets.create).mockReset() })

it('settles a failed configuration request without waiting for the error notification', async () => {
  vi.mocked(knowledgeApi.datasets.update).mockRejectedValueOnce(new Error('configuration rejected'))
  const qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={qc}>{children}</QueryClientProvider> }
  const { result } = renderHook(() => useUpdateKnowledge(), { wrapper: Wrapper })
  await act(async () => { await expect(result.current.mutateAsync({ id: 'existing-kb', data: { name: 'draft' } })).rejects.toThrow('configuration rejected') })
  expect(result.current.isPending).toBe(false)
  await waitFor(() => { expect(result.current.isError).toBe(true) })
  expect(message.error).toHaveBeenCalledTimes(1)
})


const saved: KnowledgeDataset = { id: 'owner-a-kb', name: 'A draft', display_name: 'A draft', collection_name: 'kb_a', description: '', permission: 'me', doc_num: 0, chunk_num: 0, parser_id: 'naive', embd_id: '', parser_config: {} }
function owner(isCurrent: () => boolean): KnowledgeWriteOwner { return { isCurrent, assertCurrent: () => undefined, signal: new AbortController().signal } }
function setup() {
  const qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const invalidate = vi.spyOn(qc, 'invalidateQueries')
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={qc}>{children}</QueryClientProvider> }
  return { qc, invalidate, Wrapper }
}
it('does not notify or invalidate B for a late creation result from A', async () => {
  let belongsToA = true
  let captured = owner(() => belongsToA)
  const { Wrapper, invalidate } = setup()
  let release!: (value: KnowledgeDataset) => void
  vi.mocked(knowledgeApi.datasets.create).mockImplementation(() => new Promise((resolve) => { release = resolve }))
  const { result } = renderHook(() => useCreateKnowledge({ getOwner: () => captured }), { wrapper: Wrapper })
  let pending!: Promise<KnowledgeDataset>
  act(() => { pending = result.current.mutateAsync({ name: 'A draft' }) })
  await waitFor(() => { expect(knowledgeApi.datasets.create).toHaveBeenCalledTimes(1) })
  belongsToA = false; captured = owner(() => true)
  await act(async () => { release(saved); await pending })
  expect(message.success).not.toHaveBeenCalled()
  expect(invalidate).not.toHaveBeenCalled()
})
it('does not put A saved configuration into B query cache or notify B', async () => {
  let belongsToA = true
  let captured = owner(() => belongsToA)
  const { qc, Wrapper, invalidate } = setup()
  let release!: (value: KnowledgeDataset) => void
  vi.mocked(knowledgeApi.datasets.update).mockImplementation(() => new Promise((resolve) => { release = resolve }))
  const { result } = renderHook(() => useUpdateKnowledge({ getOwner: () => captured }), { wrapper: Wrapper })
  let pending!: Promise<KnowledgeDataset>
  act(() => { pending = result.current.mutateAsync({ id: 'owner-a-kb', data: { name: 'A draft' } }) })
  await waitFor(() => { expect(knowledgeApi.datasets.update).toHaveBeenCalledTimes(1) })
  belongsToA = false; captured = owner(() => true)
  await act(async () => { release(saved); await pending })
  expect(qc.getQueryData(knowledgeKeys.datasetDetail('owner-a-kb'))).toBeUndefined()
  expect(message.success).not.toHaveBeenCalled()
  expect(invalidate).not.toHaveBeenCalled()
})
it('does not show B a late mutation error from A', async () => {
  let belongsToA = true
  let captured = owner(() => belongsToA)
  const { Wrapper } = setup()
  let reject!: (error: Error) => void
  vi.mocked(knowledgeApi.datasets.update).mockImplementation(() => new Promise((_resolve, rejectPromise) => { reject = rejectPromise }))
  const { result } = renderHook(() => useUpdateKnowledge({ getOwner: () => captured }), { wrapper: Wrapper })
  let pending!: Promise<KnowledgeDataset>
  act(() => { pending = result.current.mutateAsync({ id: 'owner-a-kb', data: { name: 'A draft' } }) })
  await waitFor(() => { expect(knowledgeApi.datasets.update).toHaveBeenCalledTimes(1) })
  belongsToA = false; captured = owner(() => true)
  await act(async () => { reject(new Error('A private error')); await expect(pending).rejects.toThrow('A private error') })
  expect(message.error).not.toHaveBeenCalled()
})
