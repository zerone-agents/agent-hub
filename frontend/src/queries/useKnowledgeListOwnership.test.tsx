import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import apiClient from '@/api/client';
import { useAuthStore } from '@/stores/auth';
import { readOwnedKnowledgeDetail, readOwnedKnowledgeList, useKnowledgeList } from './useKnowledge';

const user = (id: string) => ({ id, name: id, email: `${id}@example.invalid`, role: 'admin' });
const identity = (id: string) => ({ data: { success: true, data: { id, roles: ['admin'] } } });
const rows = (id: string) => ({ data: { success: true, data: { total: 1, datasets: [{ id }] } } });
beforeEach(() => { useAuthStore.setState({ user: user('a') }); localStorage.setItem('access_token', 'token-a'); });
afterEach(() => { vi.restoreAllMocks(); useAuthStore.setState({ user: null }); });

it('requires authenticated identity to match the store before labeling any list result', async () => {
  localStorage.setItem('access_token', 'token-b');
  const get = vi.spyOn(apiClient, 'get').mockResolvedValue(identity('b'));
  await expect(readOwnedKnowledgeList({}, new AbortController().signal)).rejects.toThrow('账号或登录凭据已变化');
  expect(get).toHaveBeenCalledTimes(1);
  expect(get.mock.calls[0][0]).toBe('/auth/userinfo');
});

it('stops before reading the list when credentials change during identity verification', async () => {
  const get = vi.spyOn(apiClient, 'get').mockImplementation(async () => { localStorage.setItem('access_token', 'token-b'); return identity('a'); });
  await expect(readOwnedKnowledgeList({}, new AbortController().signal)).rejects.toThrow('账号或登录凭据已变化');
  expect(get).toHaveBeenCalledTimes(1);
});

it('keeps old cached data bound to A during token-only B transition and reloads only verified B data', async () => {
  const get = vi.spyOn(apiClient, 'get').mockImplementation(async (url) => url === '/auth/userinfo' ? identity(localStorage.getItem('access_token') === 'token-a' ? 'a' : 'b') : rows(localStorage.getItem('access_token') === 'token-a' ? 'kb-a' : 'kb-b'));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={qc}>{children}</QueryClientProvider>; }
  const { result, unmount } = renderHook(() => useKnowledgeList({}, { owned: true }), { wrapper: Wrapper });
  await waitFor(() => { expect(result.current.origin).toEqual({ id: 'a', token: 'token-a', role: 'admin' }); });
  localStorage.setItem('access_token', 'token-b');
  expect(result.current.origin?.token).toBe('token-a');
  await act(async () => { await result.current.refetch(); });
  await waitFor(() => { expect(result.current.isError).toBe(true); });
  expect(result.current.origin?.token).toBe('token-a');
  expect(result.current.data?.datasets[0].id).toBe('kb-a');
  expect(get.mock.calls.filter(([url]) => url.includes('/knowledge/datasets'))).toHaveLength(1);
  act(() => { useAuthStore.setState({ user: user('b') }); });
  await waitFor(() => { expect(result.current.origin).toEqual({ id: 'b', token: 'token-b', role: 'admin' }); });
  expect(result.current.data?.datasets[0].id).toBe('kb-b');
  expect(JSON.stringify(qc.getQueryCache().getAll().map((query) => query.queryKey))).not.toMatch(/token-a|token-b/);
  expect(JSON.stringify(qc.getQueryCache().getAll().map((query) => query.state.data))).not.toMatch(/token-a|token-b/);
  unmount(); qc.clear();
});

it('requires verified identity before reading a settings record, including token-only transition', async () => {
  localStorage.setItem('access_token', 'token-b');
  const get = vi.spyOn(apiClient, 'get').mockResolvedValue(identity('b'));
  await expect(readOwnedKnowledgeDetail('kb-a', new AbortController().signal)).rejects.toThrow('账号或登录凭据已变化');
  expect(get).toHaveBeenCalledTimes(1);
});
