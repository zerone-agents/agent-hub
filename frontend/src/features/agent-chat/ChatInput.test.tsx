// frontend/src/features/agent-chat/ChatInput.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ChatInput from './ChatInput'
import type { ChatInputAttachments, ChatInputHandle } from './ChatInput'

function makeAttachments(overrides: Partial<ChatInputAttachments> = {}): ChatInputAttachments {
  return {
    enabled: true,
    items: [],
    uploading: false,
    add: vi.fn(() => null),
    remove: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('ChatInput attachments', () => {
  it('shows the attach button only when attachments are enabled', () => {
    const { rerender } = render(<ChatInput disabled={false} onSend={vi.fn()} attachments={makeAttachments()} />)
    expect(screen.getByLabelText('添加附件')).toBeInTheDocument()
    rerender(
      <ChatInput
        disabled={false}
        onSend={vi.fn()}
        attachments={makeAttachments({ enabled: false })}
      />
    )
    expect(screen.queryByLabelText('添加附件')).not.toBeInTheDocument()
  })

  it('sends with attachments only (no text)', async () => {
    const onSend = vi.fn().mockResolvedValue(true)
    const file = new File(['x'], 'a.txt', { type: 'text/plain' })
    render(
      <ChatInput
        disabled={false}
        onSend={onSend}
        attachments={makeAttachments({ items: [{ id: 'att-1', file, previewUrl: null, status: 'local' }] })}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: '发送' }))
    expect(onSend).toHaveBeenCalledWith('')
  })

  it('keeps text when onSend resolves (true or false) — clearing is the page onEstablished duty (spec F2)', async () => {
    // 第一次 resolve false（上传失败重试路径）、第二次 resolve true（SSE 已建立）：
    // 两种结果组件都不清空文本——清空责任已移交页面 onEstablished（clearText），
    // 409/502/网络错误等失败不再丢用户输入。
    const onSend = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true)
    const user = userEvent.setup()
    render(<ChatInput disabled={false} onSend={onSend} attachments={makeAttachments()} />)
    await user.type(screen.getByRole('textbox'), 'hello')
    await user.click(screen.getByRole('button', { name: '发送' }))
    expect(onSend).toHaveBeenCalledWith('hello')
    expect(screen.getByRole('textbox')).toHaveValue('hello')
    await waitFor(() => expect(screen.getByRole('button', { name: '发送' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: '发送' }))
    expect(onSend).toHaveBeenCalledTimes(2)
    expect(screen.getByRole('textbox')).toHaveValue('hello')
  })

  it('pastes files from the clipboard into the queue', () => {
    const add = vi.fn(() => null)
    render(<ChatInput disabled={false} onSend={vi.fn()} attachments={makeAttachments({ add })} />)
    const file = new File(['img'], 'p.png', { type: 'image/png' })
    fireEvent.paste(screen.getByRole('textbox'), { clipboardData: { files: [file] } })
    expect(add).toHaveBeenCalledWith([file])
  })

  it('drops files onto the input area', () => {
    const add = vi.fn(() => null)
    render(<ChatInput disabled={false} onSend={vi.fn()} attachments={makeAttachments({ add })} />)
    const file = new File(['img'], 'd.png', { type: 'image/png' })
    fireEvent.drop(screen.getByRole('textbox'), { dataTransfer: { files: [file] } })
    expect(add).toHaveBeenCalledWith([file])
  })

  it('shows add() error text and does not add again on failure', () => {
    const add = vi.fn(() => '附件最多 10 个')
    render(
      <ChatInput
        disabled={false}
        onSend={vi.fn()}
        attachments={makeAttachments({ add })}
      />
    )
    const file = new File(['x'], 'a.txt', { type: 'text/plain' })
    fireEvent.paste(screen.getByRole('textbox'), { clipboardData: { files: [file] } })
    expect(screen.getByText('附件最多 10 个')).toBeInTheDocument()
  })

  it('renders tray chips with remove buttons for queued items', () => {
    const remove = vi.fn()
    const file = new File(['x'], 'a.txt', { type: 'text/plain' })
    render(
      <ChatInput
        disabled={false}
        onSend={vi.fn()}
        attachments={makeAttachments({
          items: [{ id: 'att-1', file, previewUrl: null, status: 'uploaded' }],
          remove,
        })}
      />
    )
    expect(screen.getByText(/a.txt/)).toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('移除 a.txt'))
    expect(remove).toHaveBeenCalledWith('att-1')
  })

  it('restoreText (ref handle) writes text back into the input', () => {
    const handle = { current: null as ChatInputHandle | null }
    render(
      <ChatInput
        ref={(h: ChatInputHandle | null) => { handle.current = h; }}
        disabled={false}
        onSend={vi.fn()}
        attachments={makeAttachments()}
      />
    )
    act(() => { handle.current?.restoreText('回归文本'); })
    expect(screen.getByRole('textbox')).toHaveValue('回归文本')
  })

  it('clearText (ref handle) empties the input (page calls it on SSE established)', async () => {
    const handle = { current: null as ChatInputHandle | null }
    const user = userEvent.setup()
    render(
      <ChatInput
        ref={(h: ChatInputHandle | null) => { handle.current = h; }}
        disabled={false}
        onSend={vi.fn()}
        attachments={makeAttachments()}
      />
    )
    await user.type(screen.getByRole('textbox'), '待清空')
    act(() => { handle.current?.clearText(); })
    expect(screen.getByRole('textbox')).toHaveValue('')
  })
})
