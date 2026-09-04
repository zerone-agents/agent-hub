// frontend/src/features/agent-chat/useAttachments.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useAttachments, ATTACHMENT_LIMITS } from './useAttachments'

vi.mock('@/api/agent-chat', () => ({
  agentChatApi: {
    uploadFiles: vi.fn(),
  },
}))
import { agentChatApi } from '@/api/agent-chat'

const uploadFiles = vi.mocked(agentChatApi.uploadFiles)

function png(name: string, size = 10): File {
  return new File([new Uint8Array(size)], name, { type: 'image/png' })
}
function txt(name: string, size = 10): File {
  return new File([new Uint8Array(size)], name, { type: 'text/plain' })
}

beforeEach(() => {
  uploadFiles.mockReset()
})

describe('useAttachments', () => {
  it('adds files and creates image preview URLs', async () => {
    const { result } = renderHook(() => useAttachments())
    // React 19 act() 返回 wakeable；await 后 resolve 为回调返回值
    const err = (await act(() => result.current.add([png('a.png')]))) ?? null
    expect(err).toBeNull()
    expect(result.current.items).toHaveLength(1)
    expect(result.current.items[0]?.previewUrl).toMatch(/^blob:/)
    expect(result.current.items[0]?.status).toBe('local')
  })

  it('rejects more than 10 files without adding them', async () => {
    const { result } = renderHook(() => useAttachments())
    const files = Array.from({ length: 11 }, (_, i) => txt(`f${i}.txt`))
    const err = await act(() => result.current.add(files))
    expect(err).toContain('10')
    expect(result.current.items).toHaveLength(0)
  })

  it('rejects a single file over 20MB', async () => {
    const { result } = renderHook(() => useAttachments())
    const err = await act(() =>
      result.current.add([txt('big.txt', ATTACHMENT_LIMITS.maxFileBytes + 1)])
    )
    expect(err).toContain('20MB')
  })

  it('remove() drops the item and revokes its preview URL', () => {
    const { result } = renderHook(() => useAttachments())
    act(() => {
      result.current.add([png('a.png')])
    })
    const id = result.current.items[0]?.id as string
    const url = result.current.items[0]?.previewUrl as string
    const revokeSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    act(() => {
      result.current.remove(id)
    })
    expect(result.current.items).toHaveLength(0)
    expect(revokeSpy).toHaveBeenCalledWith(url)
    revokeSpy.mockRestore()
  })

  it('upload() uploads in order and marks items uploaded', async () => {
    uploadFiles.mockResolvedValue([
      { id: 'r1', name: 'a.png', mime: 'image/png', size: 10, path: '.zerone-uploads/a.png' },
      { id: 'r2', name: 'b.txt', mime: 'text/plain', size: 10, path: '.zerone-uploads/b.txt' },
    ])
    const { result } = renderHook(() => useAttachments())
    act(() => {
      result.current.add([png('a.png'), txt('b.txt')])
    })
    await act(() => result.current.upload('min', 's1'))
    expect(uploadFiles).toHaveBeenCalledWith('min', 's1', expect.anything())
    expect(result.current.items.map((i) => i.status)).toEqual(['uploaded', 'uploaded'])
    expect(result.current.items[0]?.descriptor?.id).toBe('r1')
  })

  it('upload() failure resets items to local and rethrows', async () => {
    uploadFiles.mockRejectedValue(new Error('HTTP 413'))
    const { result } = renderHook(() => useAttachments())
    act(() => {
      result.current.add([txt('a.txt')])
    })
    await expect(act(() => result.current.upload('min', 's1'))).rejects.toThrow('HTTP 413')
    expect(result.current.items[0]?.status).toBe('local')
  })

  it('invalidate() drops descriptors back to local (attachment_missing retry path)', async () => {
    uploadFiles.mockResolvedValue([
      { id: 'r1', name: 'a.txt', mime: 'text/plain', size: 10, path: '.zerone-uploads/a.txt' },
    ])
    const { result } = renderHook(() => useAttachments())
    act(() => {
      result.current.add([txt('a.txt')])
    })
    await act(() => result.current.upload('min', 's1'))
    act(() => {
      result.current.invalidate()
    })
    expect(result.current.items[0]?.status).toBe('local')
    expect(result.current.items[0]?.descriptor).toBeUndefined()
  })

  it('freezes queue mutations while upload is in flight (queue-freeze contract)', async () => {
    let resolveUpload: (
      descs: { id: string; name: string; mime: string; size: number; path: string }[]
    ) => void = () => undefined
    uploadFiles.mockReturnValue(new Promise((res) => { resolveUpload = res }))
    const { result } = renderHook(() => useAttachments())
    act(() => {
      result.current.add([txt('a.txt')])
    })
    const inFlight = act(() => result.current.upload('min', 's1'))
    // 冻结期：add 拒绝并给文案、remove 静默拒绝、条目不复活不泄漏。
    // 两处均为「无状态变更」的调用（add 被拒 / remove no-op），直接裸调、不包 act：
    // React 19 不支持 act 作用域重叠（inFlight 的 act 还开着，再开嵌套 act 会打乱
    // 作用域栈并泄漏 actScopeDepth，毒化后续所有渲染）。
    const addErr = result.current.add([txt('b.txt')])
    expect(addErr).toContain('上传进行中')
    expect(result.current.items).toHaveLength(1)
    const firstId = result.current.items[0]?.id as string
    result.current.remove(firstId)
    expect(result.current.items).toHaveLength(1)
    // 在根层级 await 在途 act（不与其它 act 嵌套），完成后再做变更队列的调用
    resolveUpload([
      { id: 'r1', name: 'a.txt', mime: 'text/plain', size: 10, path: '.zerone-uploads/a.txt' },
    ])
    await inFlight
    // 解冻后 remove 恢复正常
    act(() => {
      result.current.remove(firstId)
    })
    expect(result.current.items).toHaveLength(0)
  })

  it('upload() rejects a short descriptor list and resets queue to local', async () => {
    uploadFiles.mockResolvedValue([
      { id: 'r1', name: 'a.txt', mime: 'text/plain', size: 10, path: '.zerone-uploads/a.txt' },
    ])
    const { result } = renderHook(() => useAttachments())
    act(() => {
      result.current.add([txt('a.txt'), txt('b.txt')])
    })
    await expect(act(() => result.current.upload('min', 's1'))).rejects.toThrow('响应异常')
    expect(result.current.items.every((i) => i.status === 'local')).toBe(true)
  })
})
