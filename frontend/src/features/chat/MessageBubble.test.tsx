import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import MessageBubble from './MessageBubble'
import type { ChatMessage } from '@/api/chat'

// Mock ChatMarkdown to avoid pulling lobe-ui (ESM resolution issue in vitest)
vi.mock('./ChatMarkdown', () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>
}))

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

const aigcLabel = JSON.stringify({
  Label: '1',
  ContentProducer: '001191320118MAK93FC72D10004',
  ProduceID: '20260724065621-aff31b29c8d3',
  ReservedCode1: '9b657b9e5e7f258a'
})

function makeMessage(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    user_id: 'u1',
    id: 'm1',
    session_id: 's1',
    role: 'assistant',
    content: JSON.stringify([{ type: 'text', text: '你好' }]),
    created_at: '2026-07-24T10:00:00Z',
    hidden: false,
    token_usage: '',
    feedback: '',
    ...overrides
  }
}

describe('MessageBubble Raw mode AIGC display', () => {
  it('shows AIGC label JSON in Raw mode when message has aigc', () => {
    const msg = makeMessage({ aigc: aigcLabel })
    renderWith(<MessageBubble message={msg} />)

    // Switch to Raw
    fireEvent.click(screen.getByText('Raw'))

    // The <pre> element holds the full raw text (content + aigc)
    const pre = document.querySelector('pre')
    expect(pre).toBeTruthy()
    const raw = pre!.textContent ?? ''
    expect(raw).toContain('—— AIGC Label ——')
    expect(raw).toContain('ContentProducer')
    expect(raw).toContain('ProduceID')
    expect(raw).toContain('001191320118MAK93FC72D10004')
  })

  it('does not show AIGC section when message has no aigc', () => {
    const msg = makeMessage({ aigc: '' })
    renderWith(<MessageBubble message={msg} />)

    fireEvent.click(screen.getByText('Raw'))

    const pre = document.querySelector('pre')
    expect(pre).toBeTruthy()
    expect(pre!.textContent).not.toContain('—— AIGC Label ——')
  })
})
