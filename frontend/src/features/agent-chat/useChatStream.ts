import { useState, useCallback, useRef } from 'react'
import { agentChatApi } from '@/api/agent-chat'
import type { ContentPart } from './types'

export type StreamPhase = 'idle' | 'sending' | 'streaming' | 'done' | 'error'

export interface StreamState {
  phase: StreamPhase
  parts: ContentPart[]
  error: string | null
}

const INITIAL: StreamState = { phase: 'idle', parts: [], error: null }

// ── SSE event payload shapes ────────────────────────────────────────
// Runtime SSE data is JSON.parse'd and structurally unverified; these
// types describe the expected shape so the rest of the parser can use
// type-safe member access. Missing fields fall back via `?? defaults`.
interface SSEPartial {
  type?: 'text' | 'thinking' | 'tool_use'
  text?: string
  id?: string
  tool_name?: string
  input?: unknown
}
interface SSEContentBlock {
  type?: 'text' | 'thinking' | 'tool_use'
  text?: string
  thinking?: string
  id?: string
  name?: string
  input?: unknown
}
interface SSEPayload {
  partial?: SSEPartial
  message?: { content?: SSEContentBlock[] }
  result?: { output?: unknown; tool_use_id?: string }
}

interface UseChatStreamReturn {
  state: StreamState
  send: (agentName: string, sessionId: string, content: string) => Promise<void>
  reset: () => void
}

/**
 * Drives a single SSE conversation turn. Each `send` call:
 *  1. Resets state to { phase: 'sending' }
 *  2. Opens the fetch stream
 *  3. Parses SSE events incrementally, updating `parts` live
 *  4. On 'done' event or stream end, transitions to { phase: 'done' }
 *  5. On error, transitions to { phase: 'error' }
 *
 * SDK SSE protocol (verified 2026-07-06):
 *  - `partial_message` × N: streaming delta (token-by-token). Multiple events
 *    accumulate into the current turn's incremental content.
 *  - `assistant` × 1 per turn: the COMPLETE final message for that turn.
 *    Arrives AFTER all partial_message events for the same turn. Replaces the
 *    partial-accumulated content with the authoritative full message.
 *  - `tool_result` × 1 per tool call: tool execution result.
 *  - `result`: run-level stats (cost, duration). Ignored.
 *  - `done`: stream end marker.
 *
 * Implementation: single `parts` array with `currentTurnStart` index tracking.
 * partial_message appends to parts; assistant truncates [currentTurnStart, end)
 * then appends the full message; tool_result appends and advances the marker.
 */
export function useChatStream(): UseChatStreamReturn {
  const [state, setState] = useState<StreamState>(INITIAL)
  const abortRef = useRef<AbortController | null>(null)

  const reset = useCallback(() => {
    abortRef.current?.abort()
    setState(INITIAL)
  }, [])

  const send = useCallback(async (agentName: string, sessionId: string, content: string) => {
    // Abort any in-flight stream
    abortRef.current?.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl

    // Idle watchdog: the backend heartbeat is 15s. If we see no bytes for
    // 60s (4 missed beats), the connection is dead — a proxy killed it, the
    // runtime crashed, or the network dropped. We abort with a sentinel
    // reason so the catch block can tell this apart from a user stop.
    const IDLE_TIMEOUT_MS = 60_000
    const IDLE_REASON = 'IDLE_TIMEOUT'
    let idleTimer: ReturnType<typeof setTimeout> | null = null
    const armIdleTimer = () => {
      if (idleTimer) clearTimeout(idleTimer)
      idleTimer = setTimeout(() => { ctrl.abort(new Error(IDLE_REASON)); }, IDLE_TIMEOUT_MS)
    }
    armIdleTimer()

    setState({ phase: 'sending', parts: [], error: null })

    try {
      const resp = await agentChatApi.sendMessageStream(agentName, sessionId, content, ctrl.signal)
      armIdleTimer() // fetch handshake may take time; re-arm once streaming starts
      setState((s) => ({ ...s, phase: 'streaming' }))

      const reader = resp.body!.getReader()
      const decoder = new TextDecoder()
      let buf = ''

      const parts: ContentPart[] = []
      // 当前 turn 在 parts 中的起始索引。assistant 到来时清空
      // [currentTurnStart, parts.length)（丢弃 partial 累积），替换为完整 message。
      let currentTurnStart = 0

      const publish = () => {
        setState((s) => ({ ...s, parts: [...parts] }))
      }

      let eventName = ''

      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- SSE infinite loop, exits via `if (done) break`
      while (true) {
        const { value, done } = await reader.read()
        armIdleTimer() // any byte resets the watchdog — data OR backend heartbeat ping
        if (done) break
        buf += decoder.decode(value, { stream: true })

        const lines = buf.split('\n')
        buf = lines.pop() ?? ''

        for (const rawLine of lines) {
          const line = rawLine.replace(/\r$/, '')
          if (line === '') {
            eventName = ''
            continue
          }
          if (line.startsWith('event:')) {
            eventName = line.slice(6).trim()
            continue
          }
          if (!line.startsWith('data:')) continue

          const payload = line.slice(5).trim()
          if (!payload || payload === '{}') continue

          let data: SSEPayload
          try {
            data = JSON.parse(payload) as SSEPayload
          } catch {
            continue
          }

          if (eventName === 'partial_message') {
            // 流式增量：追加到 parts 末尾（当前 turn）
            const partial = data.partial ?? {}
            if (partial.type === 'text') {
              const last = parts[parts.length - 1]
              if (last.type === 'text') {
                last.text = (last.text ?? '') + (partial.text ?? '')
              } else {
                parts.push({ type: 'text', text: partial.text ?? '' })
              }
              publish()
            } else if (partial.type === 'thinking') {
              const last = parts[parts.length - 1]
              if (last.type === 'reasoning') {
                last.reasoning = (last.reasoning ?? '') + (partial.text ?? '')
              } else {
                parts.push({ type: 'reasoning', reasoning: partial.text ?? '' })
              }
              publish()
            } else if (partial.type === 'tool_use') {
              parts.push({
                type: 'tool_use',
                id: partial.id,
                name: partial.tool_name,
                input: partial.input as Record<string, unknown> | undefined,
              })
              publish()
            }
          } else if (eventName === 'assistant') {
            // turn 完成的完整 message：替换当前 turn 的 partial 累积
            parts.length = currentTurnStart
            const content = data.message?.content ?? []
            for (const block of content) {
              if (block.type === 'text' && block.text) {
                parts.push({ type: 'text', text: block.text })
              } else if (block.type === 'thinking') {
                parts.push({ type: 'reasoning', reasoning: block.thinking ?? '' })
              } else if (block.type === 'tool_use') {
                parts.push({
                  type: 'tool_use',
                  id: block.id,
                  name: block.name,
                  input: block.input as Record<string, unknown> | undefined,
                })
              }
            }
            currentTurnStart = parts.length
            publish()
          } else if (eventName === 'tool_result') {
            // 工具结果：直接追加，并推进 turn 起始标记
            const result = data.result ?? {}
            parts.push({
              type: 'tool_result',
              content: result.output,
              toolUseId: result.tool_use_id,
              isError: false,
            })
            currentTurnStart = parts.length
            publish()
          } else if (eventName === 'done') {
            publish()
            setState((s) => ({ ...s, phase: 'done' }))
            return
          }
        }
      }

      // Stream ended without explicit 'done' event
      publish()
      setState((s) => ({ ...s, phase: 'done' }))
    } catch (err: unknown) {
      // Distinguish three failure modes:
      //  1. Idle-timeout abort → show a friendly timeout message.
      //  2. User-initiated abort (stop button / new send / reset) → silent.
      //  3. Any other error → show the raw message.
      const errName = err instanceof Error ? err.name : undefined
      const errMsg = err instanceof Error ? err.message : 'stream failed'
      if (errName === 'AbortError' || ctrl.signal.aborted) {
        const reason: unknown = ctrl.signal.reason
        if (reason instanceof Error && reason.message === IDLE_REASON) {
          setState((s) => ({
            ...s,
            phase: 'error',
            error: '连接超时（60 秒无数据），可能是网络中断或工具执行时间过长；刷新后可查看已保存的消息',
          }))
        }
        // User abort: silent
        return
      }
      setState((s) => ({ ...s, phase: 'error', error: errMsg }))
    } finally {
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- idleTimer may legitimately be null when stream errors before arming
      if (idleTimer) clearTimeout(idleTimer)
      if (abortRef.current === ctrl) {
        abortRef.current = null
      }
    }
  }, [])

  return { state, send, reset }
}
