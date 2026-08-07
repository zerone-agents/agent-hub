import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import { buildToolInputMarkdown } from '@/lib/tool-format'
import ToolCallBlock from './ToolCallBlock'

vi.mock('@/lib/tool-format', () => ({
  getToolSummary: vi.fn(() => 'SUMMARY'),
  buildToolInputMarkdown: vi.fn(() => '| Key | Value |\n| --- | ----- |\n| file_path | x.ts |'),
  detectResultLang: vi.fn(() => 'text'),
  escapeCodeFences: vi.fn((s: string) => s)
}))

// Mock ChatMarkdown to avoid pulling lobe-ui (ESM resolution issue in vitest)
vi.mock('./ChatMarkdown', () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>
}))

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

describe('ToolCallBlock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders pending state with spinner and placeholder text', () => {
    renderWith(
      <ToolCallBlock toolName="Read" toolId="t1" input={{ file_path: 'x.ts' }} status="pending" />
    )
    expect(screen.getByText('Read')).toBeInTheDocument()
    expect(screen.getByText('SUMMARY')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('tool-call-title'))
    expect(screen.getByText('等待结果…')).toBeInTheDocument()
  })

  it('renders success state with input + result', () => {
    renderWith(
      <ToolCallBlock
        toolName="Read"
        toolId="t1"
        input={{ file_path: 'x.ts' }}
        result="hello world"
        status="success"
      />
    )
    expect(screen.getByText('Read')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('tool-call-title'))
    expect(screen.getByText('输入')).toBeInTheDocument()
    expect(screen.getByText('Result')).toBeInTheDocument()
    expect(screen.getByText(/hello world/)).toBeInTheDocument()
  })

  it('renders error state with destructive background tint on title', () => {
    const { container } = renderWith(
      <ToolCallBlock
        toolName="Bash"
        toolId="t1"
        input={{ command: 'npm test' }}
        result="ENOENT"
        status="error"
      />
    )
    const titleRow = container.querySelector('[data-testid="tool-call-title"]')
    expect(titleRow).not.toBeNull()
    expect(titleRow?.getAttribute('data-status')).toBe('error')
    fireEvent.click(screen.getByTestId('tool-call-title'))
    expect(screen.getByText('Result')).toBeInTheDocument()
  })

  it('renders "（无输出）" placeholder when result is empty', () => {
    renderWith(
      <ToolCallBlock
        toolName="Read"
        toolId="t1"
        input={{ file_path: 'x.ts' }}
        result=""
        status="success"
      />
    )
    fireEvent.click(screen.getByTestId('tool-call-title'))
    expect(screen.getByText('（无输出）')).toBeInTheDocument()
  })

  it('omits input section when buildToolInputMarkdown returns empty', () => {
    vi.mocked(buildToolInputMarkdown).mockReturnValueOnce('')
    renderWith(
      <ToolCallBlock
        toolName="Read"
        toolId="t1"
        input={{}}
        result="x"
        status="success"
      />
    )
    fireEvent.click(screen.getByTestId('tool-call-title'))
    expect(screen.queryByText('输入')).not.toBeInTheDocument()
    expect(screen.getByText('Result')).toBeInTheDocument()
  })

  it('shows [Show More] button when result exceeds limit and grows it on click', () => {
    const longResult = 'a'.repeat(1500)
    renderWith(
      <ToolCallBlock
        toolName="Bash"
        toolId="t1"
        input={{ command: 'x' }}
        result={longResult}
        status="success"
      />
    )
    // Body is collapsed by default because the content is long.
    // Click title to expand.
    fireEvent.click(screen.getByTestId('tool-call-title'))
    const showMore = screen.getByRole('button', { name: 'Show More' })
    expect(showMore).toBeInTheDocument()
    expect(screen.getByText(/truncated/)).toBeInTheDocument()
    fireEvent.click(showMore)
    expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument()
  })

  it('toggles body visibility when title row is clicked', () => {
    renderWith(
      <ToolCallBlock
        toolName="Read"
        toolId="t1"
        input={{ file_path: 'x.ts' }}
        result="hi"
        status="success"
      />
    )
    const titleRow = screen.getByTestId('tool-call-title')
    expect(screen.queryByText('Result')).not.toBeInTheDocument()
    fireEvent.click(titleRow)
    expect(screen.getByText('Result')).toBeInTheDocument()
    fireEvent.click(titleRow)
    expect(screen.queryByText('Result')).not.toBeInTheDocument()
  })
})
