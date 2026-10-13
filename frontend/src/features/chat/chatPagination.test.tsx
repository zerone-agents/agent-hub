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
import ChatViewPage from './ChatViewPage'

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
      const { page, pageSize: size } = params
      return Promise.resolve(
        envelope({
          items: [session(`s-${page}-${size}`, `会话 p${page} size${size}`)],
          total: 90, page, page_size: size, total_pages: Math.ceil(90 / size),
        })
      )
    })
    vi.mocked(chatApi.listMessages).mockImplementation((_id, params) => {
      const { page, pageSize: size } = params
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
    await screen.findByText('会话 p1 size20')

    await switchPageSizeTo(100)

    await waitFor(() => {
      expect(chatApi.listSessions).toHaveBeenCalledWith({ page: 1, pageSize: 100 })
    })
    await screen.findByText('会话 p1 size100')
  })

  it('会话列表：翻页仍按当前 pageSize 请求', async () => {
    const qc = newQueryClient()
    render(<SessionListPanel selectedId={null} onSelect={vi.fn()} />, { wrapper: wrapper(qc) })
    await screen.findByText('会话 p1 size20')

    const next = document.querySelector('.ant-pagination-next button')
    expect(next).not.toBeNull()
    fireEvent.click(next!)

    await waitFor(() => {
      expect(chatApi.listSessions).toHaveBeenCalledWith({ page: 2, pageSize: 20 })
    })
    await screen.findByText('会话 p2 size20')
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

  it('会话列表：条数切到 ≥ total 后分页控件不消失（否则无法切回）', async () => {
    const qc = newQueryClient()
    render(<SessionListPanel selectedId={null} onSelect={vi.fn()} />, { wrapper: wrapper(qc) })
    await screen.findByText('会话 p1 size20')

    await switchPageSizeTo(100)
    await screen.findByText('会话 p1 size100')

    // total=90 < pageSize=100 → 页数只剩 1 页，但「每页条数」下拉必须还在，
    // 否则用户把自己锁死在 100。
    expect(document.querySelector('.ant-pagination')).not.toBeNull()
    expect(document.querySelector('.ant-pagination-options-size-changer')).not.toBeNull()

    await switchPageSizeTo(20)
    await waitFor(() => {
      expect(chatApi.listSessions).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 })
    })
  })

  it('会话列表：末页切到更大条数时页码被钳制回 1', async () => {
    const qc = newQueryClient()
    render(<SessionListPanel selectedId={null} onSelect={vi.fn()} />, { wrapper: wrapper(qc) })
    await screen.findByText('会话 p1 size20')

    // total=90 / pageSize=20 → 5 页，先翻到末页
    const next = () => document.querySelector('.ant-pagination-next button') as HTMLElement
    for (let p = 2; p <= 5; p++) {
      fireEvent.click(next())
      await screen.findByText(`会话 p${p} size20`)
    }

    // 切到 100：页数变 1，antd 会把 current 钳制为 1（本次断言即锁定该契约）
    await switchPageSizeTo(100)
    await waitFor(() => {
      expect(chatApi.listSessions).toHaveBeenLastCalledWith({ page: 1, pageSize: 100 })
    })
  })

  it('会话列表：条数下拉只列标准选项，且不启用搜索', async () => {
    const qc = newQueryClient()
    render(<SessionListPanel selectedId={null} onSelect={vi.fn()} />, { wrapper: wrapper(qc) })
    await screen.findByText('会话 p1 size20')

    // 默认 pageSize 必须在选项集合内：rc-pagination 会把当前值 concat 进选项，
    // 默认值在集合外就会多出一个"野"选项（历史默认 30 即如此）。
    // 按数值断言，不锁 antd 文案语言（title 形如 "10 / page" / "10 条/页"）。
    const combo = document.querySelector(
      '.ant-pagination-options-size-changer .ant-select-content'
    ) as HTMLElement
    fireEvent.mouseDown(combo)
    const opts = await screen.findAllByRole('option')
    expect(
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- getAttribute/textContent 运行时可为 null，TS 声明不完备的防御
      opts.map((o) => parseInt((o.getAttribute('title') ?? o.textContent ?? '').trim(), 10))
    ).toEqual([10, 20, 50, 100])

    // 只有几项，不该是搜索框：antd 默认给条数下拉开 showSearch，本页显式关掉。
    // 断言范围限定在条数下拉内，避免误伤同页其它 Select。
    expect(document.querySelector('.ant-pagination-options-size-changer.ant-select-show-search')).toBeNull()
    expect(document.querySelector('.ant-pagination-options-size-changer input')?.hasAttribute('readonly')).toBe(true)
  })

  it('切换会话后消息分页回到第 1 页，不残留上一会话的页码', async () => {
    const qc = newQueryClient()
    vi.mocked(chatApi.listSessions).mockResolvedValue(
      envelope({
        items: [session('s1', '会话A'), session('s2', '会话B')],
        total: 2, page: 1, page_size: 20, total_pages: 1,
      })
    )
    vi.mocked(chatApi.listMessages).mockImplementation((sessionId, params) => {
      const { page } = params
      const total = sessionId === 's1' ? 60 : 5
      return Promise.resolve(
        envelope({
          items: [message(`${sessionId}-p${page}`, `${sessionId} 第${page}页`)],
          total, page, page_size: 20, total_pages: Math.ceil(total / 20),
        })
      )
    })
    render(<ChatViewPage />, { wrapper: wrapper(qc) })

    fireEvent.click(await screen.findByText('会话A'))
    await screen.findByText('s1 第1页')

    // 会话A 有 3 页，翻到末页
    const next = () => document.querySelector('.ant-pagination-next button') as HTMLElement
    fireEvent.click(next())
    await screen.findByText('s1 第2页')
    fireEvent.click(next())
    await screen.findByText('s1 第3页')

    // 切到会话B（只有 5 条=1 页）：若页码残留为 3，请求会越界、列表空白且
    // 分页控件因 total(5) 既 ≤ pageSize 又 ≤ 50 而整块不渲染 → 用户卡死。
    fireEvent.click(screen.getByText('会话B'))
    await waitFor(() => {
      expect(chatApi.listMessages).toHaveBeenLastCalledWith('s2', { page: 1, pageSize: 20 })
    })
    await screen.findByText('s2 第1页')
  })
})