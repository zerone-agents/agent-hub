import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import ChatViewPage from './ChatViewPage'

// Mock ChatMarkdown to avoid pulling lobe-ui (ESM resolution issue in vitest)
vi.mock('./ChatMarkdown', () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>
}))

vi.mock('@/queries/useChat', () => ({
  useChatSessions: () => ({
    data: {
      items: [
        { id: 's1', title: '测试会话1', user_id: 'user-abc', model: 'gpt-4', agent_id: 'general', status: 'active', updated_at: '2026-06-15T10:00:00Z' },
        { id: 's2', title: '编程问答', user_id: 'user-def', model: 'claude-3', agent_id: 'coder', status: 'active', updated_at: '2026-06-16T10:00:00Z' }
      ],
      total: 2, page: 1, page_size: 30, total_pages: 1
    },
    isLoading: false,
    page: 1,
    pageSize: 30,
    setPage: vi.fn()
  }),
  useChatMessages: () => ({
    data: {
      items: [
        { id: 'm1', session_id: 's1', role: 'user', content: '你好', created_at: '2026-06-15T09:00:00Z', hidden: false, token_usage: '', user_id: 'u1', feedback: '' },
        { id: 'm2', session_id: 's1', role: 'assistant', content: '你好！有什么可以帮你的？', created_at: '2026-06-15T09:01:00Z', hidden: false, token_usage: '50 tokens', user_id: 'u1', feedback: '' }
      ],
      total: 2, page: 1, page_size: 100, total_pages: 1
    },
    isLoading: false,
    page: 1,
    pageSize: 100,
    setPage: vi.fn()
  }),
  useDeleteChatSession: () => ({ mutate: vi.fn() })
}))

vi.mock('@/queries/useProviders', () => ({
  useProviders: () => ({ data: [] })
}))

describe('ChatViewPage', () => {
  it('renders session list and empty state pane', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <ChatViewPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // Sidebar title
    expect(screen.getByText('聊天记录')).toBeInTheDocument()
    // Session items from mock
    expect(screen.getByText('测试会话1')).toBeInTheDocument()
    expect(screen.getByText('编程问答')).toBeInTheDocument()
    // Empty state prompt
    expect(screen.getByText('选择一个会话')).toBeInTheDocument()
  })
})
