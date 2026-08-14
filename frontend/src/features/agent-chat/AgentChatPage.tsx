import { useEffect, useMemo, useRef, useState, useCallback } from 'react'
import { useParams } from 'react-router'
import { Empty } from 'antd'
import { StopIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useQueryClient } from '@tanstack/react-query'
import type { AgentChatSession } from '@/api/agent-chat'
import type { ChatMessage } from '@/api/chat'
import { useAgentChatMessages } from '@/queries/useAgentChat'
import { tokens as t } from '@/styles/tokens'
import MessageBubble from '@/features/chat/MessageBubble'
import ChatSessionList from './ChatSessionList'
import ChatInput from './ChatInput'
import SceneWelcome from './SceneWelcome'
import StreamingMessage from './StreamingMessage'
import AgentDetailBar from './AgentDetailBar'
import AigcHint from './AigcHint'
import { useChatStream } from './useChatStream'
import CwdFilePanel from './CwdFilePanel'

const useStyles = createStyles(({ css }) => ({
  page: css`
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: ${t.surface};
  `,
  body: css`
    flex: 1;
    display: flex;
    min-height: 0;
    @media (max-width: 768px) {
      flex-direction: column;
    }
  `,
  main: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    height: 100%;
    overflow: hidden;
  `,
  messages: css`
    flex: 1;
    overflow-y: auto;
    padding: 24px;
    @media (max-width: 768px) {
      padding: 16px;
    }
  `,
  emptyPane: css`
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    @media (max-width: 768px) {
      display: none;
    }
  `,
  mobileBack: css`
    display: none;
    @media (max-width: 768px) {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 12px 16px;
      font-size: ${t.textSm};
      font-weight: 500;
      color: ${t.ink};
      background: transparent;
      border: none;
      border-bottom: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
      cursor: pointer;
      width: 100%;
      flex-shrink: 0;
      transition: background 0.15s;
      &:hover {
        background: ${t.inkSubtle};
      }
    }
  `,
  stopBar: css`
    display: flex;
    justify-content: center;
    padding: 8px 0;
    flex-shrink: 0;
  `,
  stopBtn: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 16px;
    border: 1px solid ${t.danger};
    border-radius: 20px;
    background: var(--card);
    color: ${t.danger};
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
    &:hover {
      background: rgba(220, 38, 38, 0.06);
    }
  `
}))

import { ArrowLeftIcon } from '@phosphor-icons/react'

export default function AgentChatPage() {
  const { styles } = useStyles()
  const { name = '' } = useParams<{ name: string }>()
  const [selected, setSelected] = useState<AgentChatSession | null>(null)
  const { data: msgData } = useAgentChatMessages(name, selected?.id ?? null)
  const stream = useChatStream()
  const isStreaming = stream.state.phase === 'sending' || stream.state.phase === 'streaming'
  const scrollRef = useRef<HTMLDivElement>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const queryClient = useQueryClient()

  const STREAMING_MSG_ID = '__streaming__'

  const history = useMemo(() => msgData?.items ?? [], [msgData?.items])

  // Treat the in-flight stream as a transient assistant message rendered by the
  // same MessageBubble component as persisted messages. When the stream ends we
  // immediately promote the transient content into the query cache as a
  // placeholder persisted message, then reset the stream. This guarantees that
  // the transient message and the real persisted message are never displayed
  // at the same time, eliminating duplication while keeping the same component
  // and position.
  const transientMessage: ChatMessage | null = useMemo(() => {
    if (stream.state.phase === 'idle' || stream.state.parts.length === 0) return null
    // 会话门控：流式内容只渲染在产生它的会话里；用户在流式中途切换到
    // 其他会话时，不把另一个会话的内容带过去。
    if (stream.state.sessionId !== selected?.id) return null
    return {
      id: STREAMING_MSG_ID,
      // 上面的 sessionId 门控已保证 selected 非空且与流所属会话一致
      session_id: selected.id,
      role: 'assistant',
      content: JSON.stringify(stream.state.parts),
      created_at: new Date().toISOString(),
      hidden: false,
      token_usage: '',
      feedback: '',
      user_id: ''
    }
  }, [stream.state, selected])

  const displayMessages: ChatMessage[] = useMemo(() => {
    if (!transientMessage) return history
    const base = history.filter((m) => m.id !== STREAMING_MSG_ID)
    return [...base, transientMessage]
  }, [history, transientMessage])

  // Sticky-bottom auto-scroll: pause when user scrolls up, resume when they
  // scroll back to the bottom (within 80px tolerance).
  const handleScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    setAutoScroll(distanceFromBottom < 80)
  }, [])

  // Auto-scroll on new content (only when sticky-bottom is active).
  // Depend on parts reference directly (useChatStream creates a new array on
  // every publish) instead of a derived key, so consecutive tool_result events
  // (which share the same type/text/reasoning) still trigger scrolling.
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [displayMessages.length, stream.state.parts, autoScroll])

  // Warn before closing/refreshing the tab while a stream is active
  useEffect(() => {
    if (!isStreaming) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
    }
    window.addEventListener('beforeunload', handler)
    return () => { window.removeEventListener('beforeunload', handler); }
  }, [isStreaming])

  // Abort any in-flight stream when the page unmounts
  useEffect(() => {
    return () => {
      stream.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- unmount-only cleanup; stream.reset is stable (useCallback in useChatStream)
  }, [stream.reset])

  // When a stream finishes, promote the streamed content into the query cache
  // as a placeholder persisted message and reset the stream. Then invalidate
  // so the real server-generated message (with correct id/timestamps) replaces
  // the placeholder seamlessly.
  //
  // 缓存操作一律以 stream.state.sessionId（产生内容的会话）为准，而不是当前
  // 选中的会话：用户可能在流式中途切换会话，用 selected.id 会把 A 会话的
  // 内容提升进 B 会话的缓存。
  useEffect(() => {
    const streamSessionId = stream.state.sessionId
    if (stream.state.phase === 'done' && streamSessionId) {
      const parts = stream.state.parts
      if (parts.length > 0) {
        const placeholder: ChatMessage = {
          id: STREAMING_MSG_ID,
          session_id: streamSessionId,
          role: 'assistant',
          content: JSON.stringify(parts),
          created_at: new Date().toISOString(),
          hidden: false,
          token_usage: '',
          feedback: '',
          user_id: ''
        }
        queryClient.setQueryData(
          ['agent-chat-messages', name, streamSessionId],
          (old: { items: ChatMessage[]; total: number } | undefined) => {
            const base = (old?.items ?? []).filter((m) => m.id !== STREAMING_MSG_ID)
            return { items: [...base, placeholder], total: base.length + 1 }
          }
        )
      }
      stream.reset()
      void queryClient.invalidateQueries({
        queryKey: ['agent-chat-messages', name, streamSessionId],
      })
      void queryClient.invalidateQueries({
        queryKey: ['agent-chat-sessions', name],
      })
    }
    if (stream.state.phase === 'error' && streamSessionId) {
      void queryClient
        .invalidateQueries({
          queryKey: ['agent-chat-messages', name, streamSessionId],
        })
        .then(() => {
          // errorPersisted=true（result/subtype=error）：后端已把错误落库为
          // 系统消息，refetch 完成后 reset 流状态，让历史记录成为唯一展示
          // 来源，避免 transient 气泡与持久化消息双份显示。
          // 传输层错误（false）后端没见过这条流，历史里不会有，保留气泡。
          if (stream.state.errorPersisted) stream.reset()
        })
    }
  }, [stream.state.phase, stream.state.sessionId, name, queryClient, stream])

  const handleSend = async (content: string) => {
    if (!selected) return

    // Optimistic update: show user message immediately in the history list.
    // The real message (with server-generated id/timestamps) replaces this
    // after the stream finishes via queryClient.invalidateQueries.
    const optimisticMsg: ChatMessage = {
      user_id: '',
      id: `optimistic-${Date.now()}`,
      session_id: selected.id,
      role: 'user',
      content: JSON.stringify([{ type: 'text', text: content }]),
      created_at: new Date().toISOString(),
      hidden: false,
      token_usage: '',
      feedback: ''
    }
    queryClient.setQueryData(
      ['agent-chat-messages', name, selected.id],
      (old: { items: ChatMessage[]; total: number } | undefined) => ({
        items: [...(old?.items ?? []), optimisticMsg],
        total: (old?.total ?? 0) + 1
      })
    )

    await stream.send(name, selected.id, content)
  }

  return (
    <div className={styles.page}>
      <AgentDetailBar agentName={name} />

      <div className={styles.body}>
        <ChatSessionList
          agentName={name}
          selectedId={selected?.id ?? null}
          onSelect={setSelected}
          onDeleted={(id) => {
            if (selected?.id === id) setSelected(null)
          }}
          streamingSessionId={isStreaming ? selected?.id ?? null : null}
          hideOnMobile={!!selected}
        />

        <div className={styles.main}>
          {selected ? (
            <>
              <button
                type="button"
                className={styles.mobileBack}
                onClick={() => { setSelected(null); }}
              >
                <ArrowLeftIcon size={16} />
                返回会话列表
              </button>
              <div className={styles.messages} ref={scrollRef} onScroll={handleScroll}>
                {displayMessages.length === 0 && !isStreaming ? (
                  <SceneWelcome
                    agentName={name}
                    disabled={isStreaming}
                    onPick={(scene) => handleSend(scene.prompt)}
                  />
                ) : (
                  displayMessages.map((msg) => (
                    <MessageBubble
                      key={msg.id}
                      message={msg}
                      enableStream={msg.id === STREAMING_MSG_ID}
                    />
                  ))
                )}
                {/* 流以 error 结束时（如 runtime 返回 429 配额的 result 事件），
                    transientMessage 可能为空（本轮没有任何内容），必须单独渲染
                    错误提示，否则用户完全看不到失败原因。sessionId 门控：错误
                    只显示在产生它的会话里，切换会话后不泄漏。 */}
                {stream.state.phase === 'error' && stream.state.sessionId === selected.id && (
                  <StreamingMessage
                    parts={stream.state.parts}
                    phase="error"
                    error={stream.state.error}
                  />
                )}
                {/* runtime 内部自动重试（system/retry，如限流退避等待）期间，
                    还没有任何内容产出，单独渲染重试状态提示。sessionId 门控同上。 */}
                {isStreaming && stream.state.retry && stream.state.sessionId === selected.id && (
                  <StreamingMessage
                    parts={stream.state.parts}
                    phase={stream.state.phase}
                    retry={stream.state.retry}
                  />
                )}
              </div>
              {isStreaming && (
                <div className={styles.stopBar}>
                  <button type="button" className={styles.stopBtn} onClick={() => { stream.reset(); }}>
                    <StopIcon size={12} weight="fill" /> 停止回复
                  </button>
                </div>
              )}
              <ChatInput disabled={isStreaming} onSend={handleSend} />
              <AigcHint />
            </>
          ) : (
            <div className={styles.emptyPane}>
              <Empty description="选择左侧会话或新建会话开始对话" />
            </div>
          )}
        </div>

        <CwdFilePanel agentName={name} />
      </div>
    </div>
  )
}
