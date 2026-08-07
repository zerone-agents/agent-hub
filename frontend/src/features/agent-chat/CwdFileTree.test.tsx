import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import CwdFileTree from './CwdFileTree'
import type { FileEntry } from '@/api/agent-files'

vi.mock('@/queries/useAgentFiles', () => ({
  useDirEntries: vi.fn(),
}))

import { useDirEntries } from '@/queries/useAgentFiles'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

function mockDir(entries: FileEntry[], isLoading = false, isError = false) {
  ;(useDirEntries as any).mockReturnValue({
    data: { path: '', entries },
    isLoading,
    isError,
    refetch: vi.fn(),
  })
}

describe('CwdFileTree', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders loading skeleton when isLoading', () => {
    mockDir([], true)
    const { container } = renderWith(
      <CwdFileTree agentName="x" selectedPath={null} onSelect={vi.fn()} />
    )
    // 至少有 placeholder
    expect(container.textContent).toContain('加载中')
  })

  it('renders entries in the order received (runtime sorts dir-first + name-asc)', () => {
    // Runtime already returns entries dir-first + name-asc. Component must
    // render them as-received without client-side re-sorting.
    const entries: FileEntry[] = [
      { name: 'outputs', type: 'directory' },
      { name: 'src', type: 'directory' },
      { name: 'README.md', type: 'file', size: 50 },
      { name: 'package.json', type: 'file', size: 100 },
    ]
    mockDir(entries)
    renderWith(<CwdFileTree agentName="x" selectedPath={null} onSelect={vi.fn()} />)

    const items = screen.getAllByRole('treeitem')
    expect(items[0]).toHaveTextContent('outputs')
    expect(items[1]).toHaveTextContent('src')
    expect(items[2]).toHaveTextContent('README.md')
    expect(items[3]).toHaveTextContent('package.json')
  })

  it('clicking directory node triggers lazy fetch of children', async () => {
    const entries: FileEntry[] = [{ name: 'src', type: 'directory' }]
    // 第一次调用（根）返回 src 目录；后续调用（子目录）返空（mock 由测试驱动）
    let callCount = 0
    ;(useDirEntries as any).mockImplementation((_agent: string, _path: string, _enabled: boolean) => {
      callCount++
      return {
        data: callCount === 1
          ? { path: '', entries }
          : { path: 'src', entries: [{ name: 'index.ts', type: 'file' }] as FileEntry[] },
        isLoading: false,
        isError: false,
        refetch: vi.fn(),
      }
    })

    renderWith(<CwdFileTree agentName="x" selectedPath={null} onSelect={vi.fn()} />)
    fireEvent.click(screen.getByText('src'))
    await waitFor(() => {
      expect(screen.getByText('index.ts')).toBeInTheDocument()
    })
  })

  it('clicking file node calls onSelect', () => {
    const entries: FileEntry[] = [
      { name: 'package.json', type: 'file', size: 100, mime: 'application/json' },
    ]
    mockDir(entries)
    const onSelect = vi.fn()
    renderWith(<CwdFileTree agentName="x" selectedPath={null} onSelect={onSelect} />)

    fireEvent.click(screen.getByText('package.json'))
    expect(onSelect).toHaveBeenCalledWith('package.json', entries[0])
  })

  it('shows symlink target arrow', () => {
    const entries: FileEntry[] = [
      { name: 'outputs-link', type: 'symlink', target: 'outputs' },
    ]
    mockDir(entries)
    renderWith(<CwdFileTree agentName="x" selectedPath={null} onSelect={vi.fn()} />)
    expect(screen.getByText(/→ outputs/)).toBeInTheDocument()
  })

  it('renders directory entry without crashing', () => {
    // 子目录 loading 后 error：mock 树已展开但子目录 error
    const entries: FileEntry[] = [{ name: 'src', type: 'directory' }]
    ;(useDirEntries as any).mockImplementation((_agent: string, _path: string, _enabled: boolean) => ({
      data: { path: '', entries },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    }))
    renderWith(<CwdFileTree agentName="x" selectedPath={null} onSelect={vi.fn()} />)
    // 不展开时不会显示错误提示；展开需要点 src，子目录 mock 已被实现覆盖
    // 此用例仅验证基本渲染不崩
    expect(screen.getByText('src')).toBeInTheDocument()
  })
})
