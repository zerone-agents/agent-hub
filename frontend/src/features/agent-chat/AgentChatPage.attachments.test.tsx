// frontend/src/features/agent-chat/AgentChatPage.attachments.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useSyncExternalStore } from 'react'
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import AgentChatPage from './AgentChatPage'

vi.mock('react-router', async () => ({
  ...(await vi.importActual<object>('react-router')),
  useParams: () => ({ name: 'min' }),
}))

vi.mock('@/queries/useAgentChat', () => ({
  // 镜像真实 hook 的 queryKey 并订阅 query cache：页面 optimistic update 走
  // queryClient.setQueryData，静态 mock 读不回缓存，断言不了乐观消息。
  useAgentChatMessages: (agentName: string, sessionId: string | null) => {
    const qc = useQueryClient()
    const data = useSyncExternalStore(
      (cb) => qc.getQueryCache().subscribe(cb),
      () => qc.getQueryData<{ items: unknown[]; total: number }>(['agent-chat-messages', agentName, sessionId])
    )
    return { data: data ?? { items: [], total: 0 } }
  },
  useAgentChatCapabilities: () => ({ data: { attachmentsEnabled: true } }),
}))

vi.mock('@/api/agent-chat', () => ({
  agentChatApi: {
    sendMessageStream: vi.fn(),
    uploadFiles: vi.fn(),
  },
  attachmentContentUrl: (agentName: string, sessionId: string, path: string) =>
    `/api/v1/agents/${agentName}/chat/sessions/${sessionId}/attachments/content?path=${path}`,
  ApiError: class ApiError extends Error {
    code?: string
    status: number
    constructor(message: string, status: number, code?: string) {
      super(message)
      this.name = 'ApiError'
      this.status = status
      this.code = code
    }
  },
}))
import { agentChatApi, ApiError } from '@/api/agent-chat'

vi.mock('./AgentDetailBar', () => ({ default: () => <div /> }))
vi.mock('./ChatSessionList', () => {
  function MockChatSessionList({ onSelect }: { onSelect: (s: { id: string }) => void }) {
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount-only auto-select for tests
    useEffect(() => { onSelect({ id: 's1' }) }, [])
    return <div data-testid="session-list" />
  }
  return { default: MockChatSessionList }
})
vi.mock('./CwdFilePanel', () => ({ default: () => <div /> }))
vi.mock('./SceneWelcome', () => ({ default: () => <div /> }))
vi.mock('./AigcHint', () => ({ default: () => <div /> }))
vi.mock('@/features/chat/MessageBubble', () => ({
  default: ({ message }: { message: { content: string } }) => (
    <div data-testid="bubble">{message.content}</div>
  ),
}))

const uploadFiles = vi.mocked(agentChatApi.uploadFiles)
const sendMessageStream = vi.mocked(agentChatApi.sendMessageStream)

function sseResponse(): Response {
  const sse =
    'event: result\ndata: {"type":"result","subtype":"success"}\n\nevent: done\ndata: {}\n\n'
  return new Response(new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(sse))
      controller.close()
    },
  }), { status: 200 })
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AgentChatPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  uploadFiles.mockReset()
  sendMessageStream.mockReset()
})

describe('AgentChatPage attachments flow', () => {
  it('uploads then sends, optimistic message carries file parts', async () => {
    uploadFiles.mockResolvedValue([
      { id: 'r1', name: 'a.txt', mime: 'text/plain', size: 3, path: '.zerone-uploads/a.txt' },
    ])
    sendMessageStream.mockResolvedValue(sseResponse())
    renderPage()

    const file = new File(['abc'], 'a.txt', { type: 'text/plain' })
    const input = await waitFor(() =>
      document.querySelector('input[type="file"]') as HTMLInputElement
    )
    await userEvent.upload(input, file)
    expect(await screen.findByText(/a.txt/)).toBeInTheDocument()

    await userEvent.type(screen.getByRole('textbox'), '看下')
    await userEvent.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(uploadFiles).toHaveBeenCalledWith('min', 's1', [file])
    })
    expect(uploadFiles).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(sendMessageStream).toHaveBeenCalled()
    })
    const callArgs = sendMessageStream.mock.calls[0] as unknown[]
    expect(callArgs[4]).toEqual([
      { id: 'r1', name: 'a.txt', mime: 'text/plain', size: 3, path: '.zerone-uploads/a.txt' },
    ])
    await waitFor(() => {
      const bubbles = screen.getAllByTestId('bubble')
      const optimistic = bubbles.find((b) => b.textContent.includes('.zerone-uploads/a.txt'))
      expect(optimistic?.textContent).toContain('"type":"file"')
      expect(optimistic?.textContent).toContain('"type":"text"')
      // file part 在 text part 之前（与持久化顺序一致）
      const content = optimistic?.textContent ?? ''
      const fileIdx = content.indexOf('"type":"file"')
      const textIdx = content.indexOf('"type":"text"')
      expect(fileIdx).toBeGreaterThanOrEqual(0)
      expect(fileIdx).toBeLessThan(textIdx)
    })
  })

  it('keeps the local file and shows an error when upload fails', async () => {
    uploadFiles.mockRejectedValue(new Error('HTTP 413: too many files'))
    renderPage()

    const file = new File(['abc'], 'a.txt', { type: 'text/plain' })
    const input = await waitFor(() =>
      document.querySelector('input[type="file"]') as HTMLInputElement
    )
    await userEvent.upload(input, file)
    await userEvent.type(screen.getByRole('textbox'), '重试场景')
    await userEvent.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(screen.getByText(/上传失败/)).toBeInTheDocument())
    expect(screen.getByRole('textbox')).toHaveValue('重试场景') // 文本保留
    expect(screen.getByText(/a.txt/)).toBeInTheDocument() // 本地文件保留
    expect(sendMessageStream).not.toHaveBeenCalled()
  })

  it('restores sent text into the input on attachment_missing (retry contract, fix round 1)', async () => {
    uploadFiles.mockResolvedValue([
      { id: 'r1', name: 'a.txt', mime: 'text/plain', size: 3, path: '.zerone-uploads/a.txt' },
    ])
    sendMessageStream.mockRejectedValueOnce(
      new ApiError('Attachment not found', 400, 'attachment_missing')
    )
    renderPage()

    const file = new File(['abc'], 'a.txt', { type: 'text/plain' })
    const input = await waitFor(() =>
      document.querySelector('input[type="file"]') as HTMLInputElement
    )
    await userEvent.upload(input, file)
    await userEvent.type(screen.getByRole('textbox'), '重发这段')
    await userEvent.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(screen.getByText(/附件已过期/)).toBeInTheDocument())
    // 文本写回输入框 + 本地文件恢复：可直接重试发送。
    // tray chip 用移除按钮的 aria-label 唯一定位：optimistic 气泡（cache-aware
    // mock 下不被 refetch 替换）也含 a.txt 字样，getByText(/a.txt/) 会多元素。
    expect(screen.getByRole('textbox')).toHaveValue('重发这段')
    expect(screen.getByLabelText('移除 a.txt')).toBeInTheDocument()
  })
})
