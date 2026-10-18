import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import apiClient from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { readOwnedProviders, useProviders } from './useProviders'

const user = (id: string) => ({ id, name: id, email: `${id}@example.invalid`, role: 'admin' })
const identity = (id: string) => ({ data: { success: true, data: { id } } })
const providers = (id: string) => ({ data: { success: true, data: [{ id: 1, name: `provider-${id}` }] } })
beforeEach(() => { useAuthStore.setState({ user: user('a') }); localStorage.setItem('access_token', 'token-a') })
afterEach(() => { vi.restoreAllMocks(); useAuthStore.setState({ user: null }); localStorage.removeItem('access_token') })

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={client}>{children}</QueryClientProvider> }
  return { ...renderHook(() => useProviders(undefined, { owned: true }), { wrapper: Wrapper }), client }
}

it('does not read private providers when identity verification disagrees', async () => {
  const get = vi.spyOn(apiClient, 'get').mockResolvedValue(identity('b'))
  await expect(readOwnedProviders(undefined, new AbortController().signal)).rejects.toThrow()
  expect(get).toHaveBeenCalledTimes(1)
  expect(get.mock.calls[0][0]).toBe('/auth/userinfo')
})

it('rejects a late provider response after credentials change and forwards abort ownership', async () => {
  const get = vi.spyOn(apiClient, 'get').mockResolvedValueOnce(identity('a')).mockImplementationOnce(async () => {
    localStorage.setItem('access_token', 'token-b')
    return providers('a')
  })
  const signal = new AbortController().signal
  await expect(readOwnedProviders('embedding', signal)).rejects.toThrow()
  expect(get.mock.calls[1][1]).toMatchObject({ signal, params: { type: 'embedding' } })
})

it('hides stale token caches and isolates accounts without storing credentials in query data', async () => {
  vi.spyOn(apiClient, 'get').mockImplementation(async (url) => {
    const id = localStorage.getItem('access_token') === 'token-a' ? 'a' : 'b'
    return url === '/auth/userinfo' ? identity(id) : providers(id)
  })
  const { result, rerender, client, unmount } = setup()
  await waitFor(() => { expect(result.current.data?.[0].name).toBe('provider-a') })
  localStorage.setItem('access_token', 'token-b'); rerender()
  expect(result.current.data).toBeUndefined()
  await act(async () => { await result.current.refetch() })
  expect(result.current.isError).toBe(true)
  act(() => { useAuthStore.setState({ user: user('b') }) })
  await waitFor(() => { expect(result.current.data?.[0].name).toBe('provider-b') })
  const cached = client.getQueryCache().getAll().map(q => [q.queryKey, q.state.data])
  expect(JSON.stringify(cached)).not.toMatch(/token-a|token-b/)
  expect(client.getQueryData(['providers', 'all'])).toBeUndefined()
  unmount(); client.clear()
})

it('keeps the new account result when an old account response completes later', async () => {
  let resolveOld!: (value: ReturnType<typeof providers>) => void
  const old = new Promise<ReturnType<typeof providers>>(resolve => { resolveOld = resolve })
  const get = vi.spyOn(apiClient, 'get').mockImplementation(async url => {
    const id = useAuthStore.getState().user?.id ?? ''
    return url === '/auth/userinfo' ? identity(id) : id === 'a' ? old : providers('b')
  })
  const { result, client, unmount } = setup()
  await waitFor(() => { expect(get).toHaveBeenCalledTimes(2) })
  act(() => { localStorage.setItem('access_token', 'token-b'); useAuthStore.setState({ user: user('b') }) })
  await waitFor(() => { expect(result.current.data?.[0].name).toBe('provider-b') })
  await act(async () => { resolveOld(providers('a')); await old })
  expect(result.current.data?.[0].name).toBe('provider-b')
  expect(client.getQueryData(['providers', 'all', 'owned', 'a'])).toBeUndefined()
  unmount(); client.clear()
})

it('rejects null inventories instead of treating them as verified empty candidates', async () => {
  vi.spyOn(apiClient, 'get').mockResolvedValueOnce(identity('a')).mockResolvedValueOnce({ data: { success: true, data: null } })
  await expect(readOwnedProviders(undefined, new AbortController().signal)).rejects.toThrow()
})
