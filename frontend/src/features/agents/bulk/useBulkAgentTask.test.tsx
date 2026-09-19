import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useBulkAgentTask } from './useBulkAgentTask'
import type { ClassifiedItem } from './classifyBulkOperation'
import type { Agent } from '@/api/agents'

vi.mock('@/api/agents', () => ({
  agentApi: {
    deploy: vi.fn(),
    stopDeployment: vi.fn(),
    delete: vi.fn(),
  },
}))

import { agentApi } from '@/api/agents'

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

const agent = (name: string): Agent => ({ id: 1, name, config: { title: { zh: name } } })

function executable(name: string): ClassifiedItem {
  return { agent: agent(name), precheck: { kind: 'success', status: { status: 'not_found' } }, classification: 'executable' }
}

function skipped(name: string): ClassifiedItem {
  return { agent: agent(name), precheck: { kind: 'success', status: { status: 'running' } }, classification: 'skipped', reason: '已部署，建议重新部署' }
}

beforeEach(() => vi.clearAllMocks())

describe('useBulkAgentTask — 执行与汇总', () => {
  it('全部成功：逐项 succeeded、phase done、onFinished 收到成功名单、invalidate agents', async () => {
    vi.mocked(agentApi.deploy).mockResolvedValue({ data: { success: true } } as never)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const onFinished = vi.fn()
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [executable('a'), executable('b')], onFinished }) })

    expect(result.current.phase).toBe('running')
    expect(result.current.presentation).toBe('modal-open')

    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(result.current.items.map((i) => i.status)).toEqual(['succeeded', 'succeeded'])
    expect(result.current.summary).toEqual({ total: 2, succeeded: 2, failed: 0, skipped: 0, blocked: 0 })
    expect(onFinished).toHaveBeenCalledWith(['a', 'b'])
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['agents'] })
  })

  it('单项失败不中断其他项，失败原因来自 parseApiError', async () => {
    vi.mocked(agentApi.deploy)
      .mockResolvedValueOnce({ data: { success: true } } as never)
      .mockRejectedValueOnce({ isAxiosError: true, response: { status: 500, data: { error: '后端爆炸' } } })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [executable('a'), executable('b')] }) })

    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(result.current.summary).toEqual({ total: 2, succeeded: 1, failed: 1, skipped: 0, blocked: 0 })
    const failed = result.current.items.find((i) => i.status === 'failed')
    expect(failed?.reason).toBe('后端爆炸')
  })

  it('跳过/受限项以终态进入列表与汇总，不发起请求', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [skipped('s1'), executable('a')] }) })

    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(result.current.items.find((i) => i.name === 's1')).toMatchObject({ status: 'skipped', reason: '已部署，建议重新部署' })
    expect(result.current.summary).toEqual({ total: 2, succeeded: 1, failed: 0, skipped: 1, blocked: 0 })
    expect(agentApi.deploy).toHaveBeenCalledTimes(1)
  })

  it('仅跳过批次（hook 直接输入场景）：无请求、直接 done、绿点成立', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [skipped('s1')] }) })
    expect(result.current.phase).toBe('done')
    expect(result.current.summary.skipped).toBe(1)
    expect(agentApi.deploy).not.toHaveBeenCalled()
  })

  it('deploy 并发上限 2：同时 in-flight 不超过 2', async () => {
    let active = 0
    let peak = 0
    const gate = () => new Promise<void>((resolve) => {
      active++; peak = Math.max(peak, active)
      setTimeout(() => { active--; resolve() }, 10)
    })
    vi.mocked(agentApi.deploy).mockImplementation(async () => { await gate(); return { data: { success: true } } as never })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => {
      result.current.start({ operation: 'deploy', items: ['a', 'b', 'c', 'd', 'e'].map(executable) })
    })

    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(peak).toBe(2)
  })

  it('操作映射：redeploy → deploy(name, true)；stop → stopDeployment；delete → delete', async () => {
    vi.mocked(agentApi.deploy).mockResolvedValue({ data: { success: true } } as never)
    vi.mocked(agentApi.stopDeployment).mockResolvedValue({ data: { success: true } } as never)
    vi.mocked(agentApi.delete).mockResolvedValue({ data: { success: true } } as never)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'redeploy', items: [executable('a')] }) })
    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(agentApi.deploy).toHaveBeenCalledWith('a', true)

    act(() => { result.current.close() })
    act(() => { result.current.start({ operation: 'stop', items: [executable('a')] }) })
    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(agentApi.stopDeployment).toHaveBeenCalledWith('a')

    act(() => { result.current.close() })
    act(() => { result.current.start({ operation: 'delete', items: [executable('a')] }) })
    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(agentApi.delete).toHaveBeenCalledWith('a')
  })
})

describe('useBulkAgentTask — 单批次状态机（执行 × 展示正交）', () => {
  it('收起态完成：collapse → done → reopen → close 才回 idle', async () => {
    vi.mocked(agentApi.deploy).mockResolvedValue({ data: { success: true } } as never)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [executable('a')] }) })
    act(() => { result.current.collapse() })
    expect(result.current.presentation).toBe('collapsed')

    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(result.current.presentation).toBe('collapsed') // 收起态完成 = 未查看

    act(() => { result.current.reopen() })
    expect(result.current.presentation).toBe('modal-open')

    act(() => { result.current.close() })
    expect(result.current.phase).toBe('idle')
    expect(result.current.presentation).toBe('dismissed')
    expect(result.current.items).toEqual([])
  })

  it('展开态完成：close 直接清除（已查看）', async () => {
    vi.mocked(agentApi.deploy).mockResolvedValue({ data: { success: true } } as never)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [executable('a')] }) })
    await waitFor(() => { expect(result.current.phase).toBe('done') })
    expect(result.current.presentation).toBe('modal-open') // 展开态完成 = 已查看

    act(() => { result.current.close() })
    expect(result.current.phase).toBe('idle')
  })

  it('done 后新批次整体替换旧状态', async () => {
    vi.mocked(agentApi.deploy).mockResolvedValue({ data: { success: true } } as never)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useBulkAgentTask(), { wrapper: makeWrapper(qc) })

    act(() => { result.current.start({ operation: 'deploy', items: [executable('old')] }) })
    await waitFor(() => { expect(result.current.phase).toBe('done') })
    act(() => { result.current.collapse() })

    act(() => { result.current.start({ operation: 'deploy', items: [executable('new')] }) })
    expect(result.current.phase).toBe('running')
    expect(result.current.presentation).toBe('modal-open')
    expect(result.current.items.map((i) => i.name)).toEqual(['new'])
    await waitFor(() => { expect(result.current.phase).toBe('done') })
  })
})
