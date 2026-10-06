import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import apiClient from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { multiragKeys, readOwnedMultiragModels, useMultiragModels } from './useMultirag'

const user = (id: string) => ({ id, name: id, email: `${id}@example.invalid`, role: 'admin' })
const identity = (id: string) => ({ data: { success: true, data: { id } } })
const modelResponse = (name: string) => ({ data: { success: true, data: [{ name, factory: 'OpenAI', type: 'image2text', status: '1', fullId: `${name}@OpenAI` }] } })
beforeEach(() => { useAuthStore.setState({ user: user('a') }); localStorage.setItem('access_token', 'token-a') })
afterEach(() => { vi.restoreAllMocks(); useAuthStore.setState({ user: null }); localStorage.removeItem('access_token') })

function setupHook(type: 'image2text' | 'ocr' = 'image2text') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={client}>{children}</QueryClientProvider> }
  const hook = renderHook(() => useMultiragModels(type), { wrapper: Wrapper })
  return { ...hook, client }
}

it('verifies the current identity before reading model candidates', async () => {
  localStorage.setItem('access_token', 'token-b')
  const get = vi.spyOn(apiClient, 'get').mockResolvedValue(identity('b'))
  await expect(readOwnedMultiragModels('image2text', new AbortController().signal)).rejects.toThrow('账号或登录凭据已变化')
  expect(get).toHaveBeenCalledTimes(1)
  expect(get.mock.calls[0][0]).toBe('/auth/userinfo')
})

it('rejects late candidates after a credential change and passes abort ownership to the API', async () => {
  const get = vi.spyOn(apiClient, 'get').mockResolvedValueOnce(identity('a')).mockImplementationOnce(async () => {
    localStorage.setItem('access_token', 'token-b')
    return modelResponse('old-vision')
  })
  const signal = new AbortController().signal
  await expect(readOwnedMultiragModels('image2text', signal)).rejects.toThrow('账号或登录凭据已变化')
  expect(get.mock.calls[1][1]).toMatchObject({ params: { type: 'image2text' }, signal })
})

it('isolates account caches and hides cached candidates on a token-only transition', async () => {
  vi.spyOn(apiClient, 'get').mockImplementation(async (url) => {
    const id = localStorage.getItem('access_token') === 'token-a' ? 'a' : 'b'
    return url === '/auth/userinfo' ? identity(id) : modelResponse(`vision-${id}`)
  })
  const { result, rerender, unmount, client } = setupHook()
  await waitFor(() => { expect(result.current.data?.[0].name).toBe('vision-a') })
  localStorage.setItem('access_token', 'token-b')
  rerender()
  expect(result.current.data).toBeUndefined()
  await act(async () => { await result.current.refetch() })
  await waitFor(() => { expect(result.current.isError).toBe(true) })
  expect(result.current.data).toBeUndefined()
  act(() => { useAuthStore.setState({ user: user('b') }) })
  await waitFor(() => { expect(result.current.data?.[0].name).toBe('vision-b') })
  expect(client.getQueryCache().getAll().map((query) => query.queryKey)).toEqual([
    [...multiragKeys.models('image2text'), 'a'],
    [...multiragKeys.models('image2text'), 'b'],
  ])
  expect(JSON.stringify(client.getQueryCache().getAll().map((query) => [query.queryKey, query.state.data]))).not.toMatch(/token-a|token-b/)
  unmount(); client.clear()
})

it('keeps visual loading failures separate from OCR and permits explicit retry', async () => {
  let failing = true
  vi.spyOn(apiClient, 'get').mockImplementation(async (url, config) => {
    if (url === '/auth/userinfo') return identity('a')
    const params = config?.params as { type?: string } | undefined
    if (params?.type === 'image2text' && failing) throw new Error('unavailable')
    return modelResponse(params?.type === 'ocr' ? 'ocr-a' : 'vision-a')
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={client}>{children}</QueryClientProvider> }
  const { result, unmount } = renderHook(() => ({ vision: useMultiragModels('image2text'), ocr: useMultiragModels('ocr') }), { wrapper: Wrapper })
  await waitFor(() => { expect(result.current.vision.isError).toBe(true); expect(result.current.ocr.data?.[0].name).toBe('ocr-a') })
  failing = false
  await act(async () => { await result.current.vision.refetch() })
  await waitFor(() => { expect(result.current.vision.data?.[0].name).toBe('vision-a') })
  expect(result.current.ocr.data?.[0].name).toBe('ocr-a')
  unmount(); client.clear()
})

it('does not expose a late A model response after switching to B', async () => {
  let resolveOld!: (response: ReturnType<typeof modelResponse>) => void
  const oldResponse = new Promise<ReturnType<typeof modelResponse>>((resolve) => { resolveOld = resolve })
  const get = vi.spyOn(apiClient, 'get').mockImplementation(async (url) => {
    const id = useAuthStore.getState().user?.id ?? ''
    if (url === '/auth/userinfo') return identity(id)
    return id === 'a' ? oldResponse : modelResponse('vision-b')
  })
  const { result, unmount, client } = setupHook()
  await waitFor(() => { expect(get).toHaveBeenCalledTimes(2) })
  act(() => { localStorage.setItem('access_token', 'token-b'); useAuthStore.setState({ user: user('b') }) })
  await waitFor(() => { expect(result.current.data?.[0].name).toBe('vision-b') })
  await act(async () => { resolveOld(modelResponse('vision-a')); await oldResponse })
  expect(result.current.data?.[0].name).toBe('vision-b')
  expect(client.getQueryData([...multiragKeys.models('image2text'), 'a'])).toBeUndefined()
  unmount(); client.clear()
})

it('does not label null model data as a verified empty list', async () => {
  vi.spyOn(apiClient, 'get').mockResolvedValueOnce(identity('a')).mockResolvedValueOnce({ data: { success: true, data: null } })
  await expect(readOwnedMultiragModels('image2text', new AbortController().signal)).rejects.toThrow()
})
