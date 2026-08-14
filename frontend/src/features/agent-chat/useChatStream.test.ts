import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useChatStream } from './useChatStream'
import { agentChatApi } from '@/api/agent-chat'

vi.mock('@/api/agent-chat', () => ({
  agentChatApi: {
    sendMessageStream: vi.fn(),
  },
}))

const mockSend = vi.mocked(agentChatApi.sendMessageStream)

function sseResponse(sse: string): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(sse))
      controller.close()
    },
  })
  return { body: stream } as unknown as Response
}

describe('useChatStream', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('surfaces runtime errors carried by the result event (e.g. rate limit)', async () => {
    mockSend.mockResolvedValue(
      sseResponse(
        'event: result\n' +
          'data: {"type":"result","subtype":"error","error_type":"rate_limit","usage":{"input_tokens":0,"output_tokens":0},"num_turns":1,"cost":0,"errors":["429 Your token-plan 1-week quota has been exhausted. The quota will reset at 08-17 01:36:00 UTC."]}\n' +
          '\n' +
          'event: done\n' +
          'data: {}\n' +
          '\n'
      )
    )

    const { result } = renderHook(() => useChatStream())
    await act(async () => {
      await result.current.send('agent', 'session-1', '你好')
    })

    expect(result.current.state.phase).toBe('error')
    expect(result.current.state.error).toContain('429')
    expect(result.current.state.error).toContain('quota has been exhausted')
    // result 事件错误后端会落库（saveErrorMessage），页面据此去重
    expect(result.current.state.errorPersisted).toBe(true)
  })

  it('marks transport-level errors as not persisted (backend never saw a stream)', async () => {
    mockSend.mockRejectedValue(new Error('HTTP 404: session not found'))

    const { result } = renderHook(() => useChatStream())
    await act(async () => {
      await result.current.send('agent', 'session-1', '你好')
    })

    expect(result.current.state.phase).toBe('error')
    expect(result.current.state.error).toContain('HTTP 404')
    expect(result.current.state.errorPersisted).toBe(false)
  })

  it('ignores successful result stats and finishes normally on done', async () => {
    mockSend.mockResolvedValue(
      sseResponse(
        'event: result\n' +
          'data: {"type":"result","subtype":"success","usage":{"input_tokens":10,"output_tokens":5},"num_turns":1,"cost":0.001}\n' +
          '\n' +
          'event: done\n' +
          'data: {}\n' +
          '\n'
      )
    )

    const { result } = renderHook(() => useChatStream())
    await act(async () => {
      await result.current.send('agent', 'session-1', '你好')
    })

    expect(result.current.state.phase).toBe('done')
    expect(result.current.state.error).toBeNull()
  })

  it('exposes runtime retry events (system/retry) while waiting for the model', async () => {
    let ctrl: ReadableStreamDefaultController<Uint8Array> | undefined
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        ctrl = c
        c.enqueue(
          new TextEncoder().encode(
            'event: system\n' +
              'data: {"type":"system","subtype":"retry","attempt":2,"error_type":"rate_limit","delay_ms":5646.997510600469}\n' +
              '\n'
          )
        )
        // 不 close：模拟 runtime 退避等待期间流保持打开
      },
    })
    mockSend.mockResolvedValue({ body: stream } as unknown as Response)

    const { result } = renderHook(() => useChatStream())
    let sendPromise: Promise<void> | undefined
    act(() => {
      sendPromise = result.current.send('agent', 'session-1', '你好')
    })

    await waitFor(() => {
      expect(result.current.state.retry).toEqual({
        attempt: 2,
        errorType: 'rate_limit',
        delayMs: 5646.997510600469,
      })
    })
    expect(result.current.state.phase).toBe('streaming')

    // 收尾：关闭流，让 send 完成，避免泄漏定时器/挂起 promise
    await act(async () => {
      ctrl?.close()
      await sendPromise
    })
    expect(result.current.state.phase).toBe('done')
  })

  it('clears the retry state once content starts flowing', async () => {
    mockSend.mockResolvedValue(
      sseResponse(
        'event: system\n' +
          'data: {"type":"system","subtype":"retry","attempt":1,"error_type":"rate_limit","delay_ms":1000}\n' +
          '\n' +
          'event: partial_message\n' +
          'data: {"type":"partial_message","partial":{"type":"text","text":"你好"}}\n' +
          '\n' +
          'event: done\n' +
          'data: {}\n' +
          '\n'
      )
    )

    const { result } = renderHook(() => useChatStream())
    await act(async () => {
      await result.current.send('agent', 'session-1', '你好')
    })

    expect(result.current.state.phase).toBe('done')
    expect(result.current.state.retry).toBeNull()
    expect(result.current.state.parts).toEqual([{ type: 'text', text: '你好' }])
  })

  it('tracks which session the stream belongs to, so the page can scope rendering', async () => {
    mockSend.mockResolvedValue(sseResponse('event: done\ndata: {}\n\n'))

    const { result } = renderHook(() => useChatStream())
    expect(result.current.state.sessionId).toBeNull()

    await act(async () => {
      await result.current.send('agent', 'session-1', '你好')
    })
    expect(result.current.state.sessionId).toBe('session-1')

    act(() => {
      result.current.reset()
    })
    expect(result.current.state.sessionId).toBeNull()
  })
})
