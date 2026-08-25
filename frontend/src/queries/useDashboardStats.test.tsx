import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

import { useAgents } from './useAgents'
import { useTools } from './useTools'
import { useSkills } from './useSkills'
import { useScenes } from './useScenes'
import { useProviders } from './useProviders'
import { useMcps } from './useMcps'
import { useKnowledgeList } from './useKnowledge'
import { useChatSessions } from './useChat'
import { useDashboardStats } from './useDashboardStats'

vi.mock('./useAgents', () => ({ useAgents: vi.fn() }))
vi.mock('./useTools', () => ({ useTools: vi.fn() }))
vi.mock('./useSkills', () => ({ useSkills: vi.fn() }))
vi.mock('./useScenes', () => ({ useScenes: vi.fn() }))
vi.mock('./useProviders', () => ({ useProviders: vi.fn() }))
vi.mock('./useMcps', () => ({ useMcps: vi.fn() }))
vi.mock('./useKnowledge', () => ({ useKnowledgeList: vi.fn() }))
vi.mock('./useChat', () => ({ useChatSessions: vi.fn() }))

interface QueryState {
  data?: unknown
  isLoading: boolean
  isError: boolean
  refetch: ReturnType<typeof vi.fn>
}

function okQuery(data: unknown = []): QueryState {
  return { data, isLoading: false, isError: false, refetch: vi.fn() }
}

function mockAll(overrides: { knowledge?: Partial<QueryState>; agents?: Partial<QueryState> } = {}) {
  vi.mocked(useAgents).mockReturnValue({ ...okQuery(), ...overrides.agents } as never)
  vi.mocked(useTools).mockReturnValue(okQuery() as never)
  vi.mocked(useSkills).mockReturnValue(okQuery() as never)
  vi.mocked(useScenes).mockReturnValue(okQuery() as never)
  vi.mocked(useProviders).mockReturnValue(okQuery() as never)
  vi.mocked(useMcps).mockReturnValue(okQuery() as never)
  vi.mocked(useKnowledgeList).mockReturnValue({
    ...okQuery({ datasets: [], total: 0 }),
    ...overrides.knowledge
  } as never)
  vi.mocked(useChatSessions).mockReturnValue(okQuery({ items: [], total: 0 }) as never)
}

describe('useDashboardStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('aggregates data from all domain queries', () => {
    mockAll({
      knowledge: { data: { datasets: [{ id: 'kb-1', doc_num: 3 }], total: 1 } }
    })
    const { result } = renderHook(() => useDashboardStats())
    expect(result.current.isError).toBe(false)
    expect(result.current.isLoading).toBe(false)
    expect(result.current.knowledgeDatasets).toHaveLength(1)
  })

  it('does not fail the dashboard when the knowledge module is unavailable (503)', () => {
    // MultiRAG not configured -> knowledge list request rejects with 503.
    mockAll({ knowledge: { data: undefined, isError: true } })
    const { result } = renderHook(() => useDashboardStats())
    expect(result.current.isError).toBe(false)
    expect(result.current.knowledgeDatasets).toEqual([])
  })

  it('does not block the dashboard while knowledge is still loading', () => {
    mockAll({ knowledge: { data: undefined, isLoading: true } })
    const { result } = renderHook(() => useDashboardStats())
    expect(result.current.isLoading).toBe(false)
    expect(result.current.knowledgeDatasets).toEqual([])
  })

  it('still fails the dashboard when a required query errors', () => {
    mockAll({ agents: { isError: true } })
    const { result } = renderHook(() => useDashboardStats())
    expect(result.current.isError).toBe(true)
  })

  it('refetch resolves even when the knowledge refetch rejects', async () => {
    const knowledgeRefetch = vi.fn().mockRejectedValue(new Error('503'))
    mockAll({ knowledge: { data: undefined, isError: true, refetch: knowledgeRefetch } })
    const { result } = renderHook(() => useDashboardStats())
    await expect(result.current.refetch()).resolves.toBeUndefined()
    expect(knowledgeRefetch).toHaveBeenCalled()
  })
})
