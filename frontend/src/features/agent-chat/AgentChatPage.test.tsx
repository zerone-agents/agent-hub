import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect } from 'react'
import AgentChatPage from './AgentChatPage'

vi.mock('react-router', () => ({
  useParams: () => ({ name: 'test-agent' }),
}))

vi.mock('@/queries/useAgentChat', () => ({
  useAgentChatMessages: vi.fn(() => ({ data: { items: [], total: 0 } })),
  // 页面新增 capabilities 消费点（issue #94）；既有断言不依赖附件，保持关闭态
  useAgentChatCapabilities: () => ({ data: { attachmentsEnabled: false } }),
}))

// 模拟流状态：每个用例可改写 mockStream.state 覆盖不同场景；
// send/reset 提升为可断言的 spy（去重用例依赖 reset 调用与否）
const mockStream = vi.hoisted(() => ({
  state: {
    phase: 'error' as string,
    parts: [] as unknown[],
    error: '429 Your token-plan 1-week quota has been exhausted.' as string | null,
    retry: null as { attempt: number; errorType: string; delayMs: number } | null,
    sessionId: 'session-1' as string | null,
    errorPersisted: false,
  },
  send: vi.fn(),
  reset: vi.fn(),
}))

vi.mock('./useChatStream', () => ({
  useChatStream: () => ({
    state: mockStream.state,
    send: mockStream.send,
    reset: mockStream.reset,
  }),
}))

// 自动选中一个会话，让主聊天区渲染出来
vi.mock('./ChatSessionList', () => {
  function MockChatSessionList({ onSelect }: { onSelect: (s: { id: string }) => void }) {
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount-only auto-select for tests
    useEffect(() => { onSelect({ id: 'session-1' }) }, [])
    return <div data-testid="session-list" />
  }
  return { default: MockChatSessionList }
})

vi.mock('./AgentDetailBar', () => ({ default: () => <div /> }))
vi.mock('./CwdFilePanel', () => ({ default: () => <div /> }))
vi.mock('./SceneWelcome', () => ({ default: () => <div /> }))
// MessageBubble 渲染消息 content 文本，让 transient 流式内容可被断言
vi.mock('@/features/chat/MessageBubble', () => ({
  default: ({ message }: { message: { id: string; content: string } }) => <div>{message.content}</div>,
}))

function renderPage(qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  render(
    <QueryClientProvider client={qc}>
      <AgentChatPage />
    </QueryClientProvider>
  )
  return qc
}

describe('AgentChatPage stream error display', { timeout: 15000 }, () => {
  beforeEach(() => {
    mockStream.reset.mockClear()
    mockStream.state = {
      phase: 'error',
      parts: [],
      error: '429 Your token-plan 1-week quota has been exhausted.',
      retry: null,
      sessionId: 'session-1',
      errorPersisted: false,
    }
  })

  it('shows the runtime error when the stream ends in error phase', async () => {
    renderPage()
    expect(await screen.findByText(/429 Your token-plan/)).toBeInTheDocument()
  })

  it('shows a retry notice while the runtime is backing off', async () => {
    mockStream.state = {
      phase: 'streaming',
      parts: [],
      error: null,
      retry: { attempt: 2, errorType: 'rate_limit', delayMs: 5646.997 },
      sessionId: 'session-1',
      errorPersisted: false,
    }
    renderPage()
    expect(await screen.findByText(/第 2 次/)).toBeInTheDocument()
    expect(screen.getByText(/rate_limit/)).toBeInTheDocument()
  })

  it('does not show the error in a different session than the one that produced it', async () => {
    // 流属于 other-session，但当前选中的是 session-1（ChatSessionList mock 自动选中）
    mockStream.state = {
      phase: 'error',
      parts: [],
      error: '429 Your token-plan 1-week quota has been exhausted.',
      retry: null,
      sessionId: 'other-session',
      errorPersisted: false,
    }
    renderPage()
    // 等待会话区渲染完成，再断言错误不存在
    expect(await screen.findByTestId('session-list')).toBeInTheDocument()
    expect(screen.queryByText(/429 Your token-plan/)).not.toBeInTheDocument()
  })

  it('does not show the retry notice in a different session than the one that produced it', async () => {
    mockStream.state = {
      phase: 'streaming',
      parts: [],
      error: null,
      retry: { attempt: 2, errorType: 'rate_limit', delayMs: 5646.997 },
      sessionId: 'other-session',
      errorPersisted: false,
    }
    renderPage()
    expect(await screen.findByTestId('session-list')).toBeInTheDocument()
    expect(screen.queryByText(/第 2 次/)).not.toBeInTheDocument()
  })

  it('resets the stream after refetch when the backend persisted the error, so history is the single source', async () => {
    mockStream.state = {
      phase: 'error',
      parts: [],
      error: '429 Your token-plan 1-week quota has been exhausted.',
      retry: null,
      sessionId: 'session-1',
      errorPersisted: true,
    }
    renderPage()
    await waitFor(() => {
      expect(mockStream.reset).toHaveBeenCalled()
    })
  })

  it('keeps the transient error bubble when the error was not persisted (transport failure)', async () => {
    // errorPersisted=false：后端没见过这条流，历史里不会有错误消息，
    // 必须保留 transient 气泡，不能 reset
    mockStream.state = {
      phase: 'error',
      parts: [],
      error: 'HTTP 404: session not found',
      retry: null,
      sessionId: 'session-1',
      errorPersisted: false,
    }
    renderPage()
    expect(await screen.findByText(/HTTP 404/)).toBeInTheDocument()
    // 给异步的 invalidate().then() 留出执行窗口后再断言
    await new Promise((r) => setTimeout(r, 50))
    expect(mockStream.reset).not.toHaveBeenCalled()
  })
})

describe('AgentChatPage session scoping', { timeout: 15000 }, () => {
  beforeEach(() => {
    mockStream.state = {
      phase: 'streaming',
      parts: [{ type: 'text', text: '流式内容' }],
      error: null,
      retry: null,
      sessionId: 'session-1',
      errorPersisted: false,
    }
  })

  it('renders in-flight stream content in the session that produced it', async () => {
    renderPage()
    expect(await screen.findByText(/流式内容/)).toBeInTheDocument()
  })

  it('does not render in-flight stream content in a different session', async () => {
    mockStream.state = { ...mockStream.state, sessionId: 'other-session' }
    renderPage()
    expect(await screen.findByTestId('session-list')).toBeInTheDocument()
    expect(screen.queryByText(/流式内容/)).not.toBeInTheDocument()
  })

  it('promotes finished stream content into the cache of the session that produced it', async () => {
    // 流属于 other-session，但当前选中的是 session-1：提升必须写入 other-session 的缓存
    mockStream.state = {
      phase: 'done',
      parts: [{ type: 'text', text: '完成内容' }],
      error: null,
      retry: null,
      sessionId: 'other-session',
      errorPersisted: false,
    }
    const qc = renderPage()
    expect(await screen.findByTestId('session-list')).toBeInTheDocument()

    const ownCache = qc.getQueryData<{ items: { id: string }[] }>([
      'agent-chat-messages',
      'test-agent',
      'other-session',
    ])
    expect(ownCache?.items.some((m) => m.id === '__streaming__')).toBe(true)

    const selectedCache = qc.getQueryData<{ items: { id: string }[] }>([
      'agent-chat-messages',
      'test-agent',
      'session-1',
    ])
    expect(selectedCache?.items.some((m) => m.id === '__streaming__')).toBeFalsy()
  })
})
