import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useDirEntries } from './useAgentFiles'
import type { ListFilesResponse } from '@/api/agent-files'

vi.mock('@/api/agent-files', () => ({
  agentFilesApi: {
    list: vi.fn(),
    getContent: vi.fn(),
    head: vi.fn(),
  },
}))

import { agentFilesApi } from '@/api/agent-files'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useDirEntries', () => {
  beforeEach(() => vi.clearAllMocks())

  it('does not fetch when enabled=false', () => {
    ;(agentFilesApi.list as any).mockResolvedValue({ data: { path: '', entries: [] } })
    renderHook(() => useDirEntries('x', '', false), { wrapper: makeWrapper() })
    expect(agentFilesApi.list).not.toHaveBeenCalled()
  })

  it('fetches when enabled=true and returns entries', async () => {
    const data: ListFilesResponse = {
      path: '',
      entries: [
        { name: 'src', type: 'directory' },
        { name: 'package.json', type: 'file', size: 844, mime: 'application/json' },
      ],
    }
    ;(agentFilesApi.list as any).mockResolvedValue({ data })

    const { result } = renderHook(() => useDirEntries('x', '', true), { wrapper: makeWrapper() })
    await waitFor(() => { expect(result.current.isSuccess).toBe(true); })
    expect(result.current.data?.entries).toHaveLength(2)
    expect(agentFilesApi.list).toHaveBeenCalledWith('x', { path: '' })
  })

  it('uses different queryKey for different dirPath', async () => {
    ;(agentFilesApi.list as any).mockResolvedValue({ data: { path: 'a', entries: [] } })
    const { rerender } = renderHook(({ p }) => useDirEntries('x', p, true), {
      wrapper: makeWrapper(),
      initialProps: { p: 'a' },
    })
    await waitFor(() => { expect(agentFilesApi.list).toHaveBeenCalledTimes(1); })
    rerender({ p: 'b' })
    await waitFor(() => { expect(agentFilesApi.list).toHaveBeenCalledTimes(2); })
  })
})
