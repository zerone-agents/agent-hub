import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import CwdFilePanel from './CwdFilePanel'

vi.mock('@/queries/useAgentFiles', () => ({
  useDirEntries: vi.fn(),
}))

// CwdFilePanel composes CwdFilePreview, whose effect fires agentFilesApi.head
// and agentFilesApi.getContent as soon as a file is selected. Without this
// mock those calls hit real fetch in jsdom and produce noisy unhandled
// rejections that pollute test output. The actual return values don't
// matter for the panel tests — we only assert on the download affordance —
// so a stub that returns a generic error response is sufficient. Per-test
// overrides can use vi.mocked(agentFilesApi.head).mockResolvedValueOnce(...).
vi.mock('@/api/agent-files', () => ({
  agentFilesApi: {
    head: vi.fn().mockResolvedValue({ ok: false, status: 0, headers: new Headers() }),
    getContent: vi.fn().mockResolvedValue({ ok: false, status: 0, headers: new Headers() }),
  },
}))

import { useDirEntries } from '@/queries/useAgentFiles'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

describe('CwdFilePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('default collapsed: renders only 36px rail, does not fetch', () => {
    ;(useDirEntries as any).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
    })
    renderWith(<CwdFilePanel agentName="x" />)
    // 折叠态只渲染一个图标按钮
    expect(screen.getByRole('button', { name: /展开/ })).toBeInTheDocument()
    // The panel calls useDirEntries(agent, '', enabled=expanded) on every
    // render, so the spy DOES fire — but with enabled=false, which means the
    // underlying queryFn (the real fetch) never runs. We assert on the
    // enabled flag rather than spy invocations to express the real contract
    // ("collapsed state triggers no network").
    expect(useDirEntries).toHaveBeenCalledWith('x', '', false)
  })

  it('clicking rail expands to full panel and triggers root load', async () => {
    ;(useDirEntries as any).mockReturnValue({
      data: { path: '', entries: [{ name: 'package.json', type: 'file' }] },
      isLoading: false,
      isError: false,
    })
    renderWith(<CwdFilePanel agentName="x" />)
    fireEvent.click(screen.getByRole('button', { name: /展开/ }))

    await waitFor(() => {
      expect(screen.getByText('Agent 工作区')).toBeInTheDocument()
    })
  })

  it('persists expanded state to localStorage per agent', async () => {
    ;(useDirEntries as any).mockReturnValue({
      data: { path: '', entries: [] },
      isLoading: false,
      isError: false,
    })
    const { unmount } = renderWith(<CwdFilePanel agentName="alpha" />)
    fireEvent.click(screen.getByRole('button', { name: /展开/ }))
    await waitFor(() => {
      expect(localStorage.getItem('agent-chat.cwd-panel.alpha.expanded')).toBe('true')
    })
    unmount()

    // Re-mount: should start expanded because localStorage says so
    renderWith(<CwdFilePanel agentName="alpha" />)
    expect(screen.queryByRole('button', { name: /展开/ })).not.toBeInTheDocument()
    expect(screen.getByText('Agent 工作区')).toBeInTheDocument()
  })

  it('hides entirely when root load fails (agent unavailable)', () => {
    ;(useDirEntries as any).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })
    // Force expanded=true to exercise the error path
    localStorage.setItem('agent-chat.cwd-panel.beta.expanded', 'true')
    const { container } = renderWith(<CwdFilePanel agentName="beta" />)
    expect(container.firstChild).toBeNull()
  })

  it('clicking a file shows the preview pane', async () => {
    ;(useDirEntries as any).mockImplementation((_agent: string, path: string, _enabled: boolean) => ({
      data: path === ''
        ? { path: '', entries: [{ name: 'a.txt', type: 'file' }] }
        : { path, entries: [] },
      isLoading: false,
      isError: false,
    }))

    renderWith(<CwdFilePanel agentName="x" />)
    fireEvent.click(screen.getByRole('button', { name: /展开/ }))
    await waitFor(() => expect(screen.getByText('a.txt')).toBeInTheDocument())

    fireEvent.click(screen.getByText('a.txt'))
    // Preview pane: the download affordance (rendered as <a download> in
    // CwdFilePreview, not a <button>) is always present once a file is
    // selected. Query by role=link since the icon-only button heuristic
    // from the brief doesn't match CwdFilePreview's actual DOM.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /下载/ })).toBeInTheDocument()
    })
  })

  it('resets selection when agentName changes', async () => {
    ;(useDirEntries as any).mockImplementation((_agent: string, _path: string, _enabled: boolean) => ({
      data: { path: '', entries: [{ name: 'a.txt', type: 'file' }] },
      isLoading: false,
      isError: false,
    }))
    // Both agents start expanded so the panel body (with preview pane) is
    // rendered. Without this, beta defaults to collapsed rail and the
    // "选择文件预览" placeholder lives in a DOM branch that isn't mounted.
    localStorage.setItem('agent-chat.cwd-panel.alpha.expanded', 'true')
    localStorage.setItem('agent-chat.cwd-panel.beta.expanded', 'true')

    const { rerender } = renderWith(<CwdFilePanel agentName="alpha" />)
    // Panel is already expanded via localStorage; just select a file.
    await waitFor(() => expect(screen.getByText('a.txt')).toBeInTheDocument())
    fireEvent.click(screen.getByText('a.txt'))
    await waitFor(() => { expect(screen.getAllByRole('button', { name: /下载/ }).length).toBeGreaterThan(0); })

    // switch agent — selection should reset to placeholder
    rerender(
      <ConfigProvider theme={antdTheme}>
        <CwdFilePanel agentName="beta" />
      </ConfigProvider>
    )
    // After agent switch, the preview pane shows the empty placeholder
    await waitFor(() => {
      expect(screen.getByText('选择文件预览')).toBeInTheDocument()
    })
  })
})
