import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// 真实 hook + mock API 层：分页是「控件 onChange → hook 状态 → queryKey → 请求」
// 一条链，hook 被 mock 掉就测不到接线断在哪（会话列表的每页条数失效正是此类）。
vi.mock('@/api/chat', () => ({
  chatApi: { listSessions: vi.fn(), listMessages: vi.fn(), deleteSession: vi.fn() },
}))
vi.mock('@/queries/useProviders', () => ({ useProviders: () => ({ data: [] }) }))
vi.mock('@/api/agent-files', () => ({
  adminFileContentUrl: (a: string, p: string) => `/files/${a}/${p}`,
}))
vi.mock('./ChatMarkdown', () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>,
}))

import { chatApi } from '@/api/chat'
import SessionListPanel from './SessionListPanel'
import MessageViewer from './MessageViewer'

const session = (id: string, title: string) => ({
  user_id: 'u1', id, title, created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z', model: 'gpt-4', system_prompt: '',
  status: 'active', mode: 'chat', provider_id: 'p1', agent_id: 'general',
  permission_profile: '', hidden: false, extra_directories: '', is_user_bound: false,
})

const message = (id: string, content: string) => ({
  user_id: 'u1', id, session_id: 's1', role: 'assistant', content,
  created_at: '2026-01-01T00:00:00Z', hidden: false, token_usage: '', feedback: '',
})

function envelope(data: unknown) {
  return { data: { success: true, data } } as never
}

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

function newQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

/** 通过 antd 的每页条数下拉切换 pageSize（total > 50 时 antd 才渲染该下拉）。 */
async function switchPageSizeTo(size: number) {
  const combo = document.querySelector(
    '.ant-pagination-options-size-changer .ant-select-content'
  ) as HTMLElement | null
  expect(combo).not.toBeNull()
  fireEvent.mouseDown(combo!)
  const option = (await screen.findAllByRole('option')).find(
    // title 形如 "100 / page" / "100 条/页"——按数字前缀匹配，不依赖 antd 文案语言
    (o) => (o.getAttribute('title') ?? '').trim().startsWith(String(size))
  )
  expect(option).toBeDefined()
  fireEvent.click(option!)
}

describe('chat pagination', () => {
  beforeEach(() => {
    vi.mocked(chatApi.listSessions).mockImplementation((params) => {
      const page = params?.page ?? 1
      const size = params?.pageSize ?? 30
      return Promise.resolve(
        envelope({
          items: [session(`s-${page}-${size}`, `会话 p${page} size${size}`)],
          total: 90, page, page_size: size, total_pages: Math.ceil(90 / size),
        })
      )
    })
    vi.mocked(chatApi.listMessages).mockImplementation((_id, params) => {
      const page = params?.page ?? 1
      const size = params?.pageSize ?? 20
      return Promise.resolve(
        envelope({
          items: [message(`m-${page}-${size}`, `消息 p${page} size${size}`)],
          total: 60, page, page_size: size, total_pages: Math.ceil(60 / size),
        })
      )
    })
  })

  it('会话列表：切换每页条数会带上新的 pageSize 重新请求', async () => {
    const qc = newQueryClient()
    render(<SessionListPanel selectedId={null} onSelect={vi.fn()} />, { wrapper: wrapper(qc) })
    await screen.findByText('会话 p1 size30')

    await switchPageSizeTo(100)

    await waitFor(() => {
      expect(chatApi.listSessions).toHaveBeenCalledWith({ page: 1, pageSize: 100 })
    })
    await screen.findByText('会话 p1 size100')
  })

  it('会话列表：翻页仍按当前 pageSize 请求', async () => {
    const qc = newQueryClient()
    render(<SessionListPanel selectedId={null} onSelect={vi.fn()} />, { wrapper: wrapper(qc) })
    await screen.findByText('会话 p1 size30')

    const next = document.querySelector('.ant-pagination-next button')
    expect(next).not.toBeNull()
    fireEvent.click(next!)

    await waitFor(() => {
      expect(chatApi.listSessions).toHaveBeenCalledWith({ page: 2, pageSize: 30 })
    })
    await screen.findByText('会话 p2 size30')
  })

  it('消息列表：切换每页条数会带上新的 pageSize 重新请求', async () => {
    const qc = newQueryClient()
    render(<MessageViewer session={session('s1', '会话') as never} />, { wrapper: wrapper(qc) })
    await screen.findByText('消息 p1 size20')

    await switchPageSizeTo(100)

    await waitFor(() => {
      expect(chatApi.listMessages).toHaveBeenCalledWith('s1', { page: 1, pageSize: 100 })
    })
    await screen.findByText('消息 p1 size100')
  })
})