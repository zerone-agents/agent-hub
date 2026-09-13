import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import { antdTheme } from '@/lib/antd-theme'
import AgentSwitcher from './AgentSwitcher'

const agents = vi.hoisted(() => [
  { name: 'agent-a', config: { title: { zh: '甲助手' } } },
  { name: 'agent-b', config: { title: { zh: '乙助手' } } },
  { name: 'agent-c', config: { title: { zh: '丙助手' } } },
])

vi.mock('@/queries/useAgents', () => ({
  usePublicAgents: () => ({ data: agents }),
}))

// 跳转走 react-router 的 useNavigate，mock 之并保留其余真实导出（repo 惯例）。
const navigateMock = vi.fn()
vi.mock('react-router', async () => ({
  ...(await vi.importActual<object>('react-router')),
  useNavigate: () => navigateMock,
}))

function renderSwitcher(current = 'agent-a') {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <AgentSwitcher current={current} />
      </MemoryRouter>
    </ConfigProvider>
  )
}

// antd v6 Select 在 jsdom 的现实：可见虚拟列表依赖布局高度（jsdom 全 0）渲染
// 0 个选项，但 rc-select 的隐藏 a11y 镜像列表（role=listbox/option，选项
// aria-label=label）始终随 activeIndex±1 渲染，键盘导航（ArrowDown/Enter，
// 事件须带 keyCode——rc-select 读的是 keyCode）走真实交互代码路径。
function openSelect() {
  const root = document.querySelector('.ant-select')
  expect(root).not.toBeNull()
  fireEvent.mouseDown(root as HTMLElement)
  return screen.getByRole('combobox')
}

/** a11y 镜像 listbox 内选项的可访问名（aria-label = 展示标题）。 */
function listboxOptionTitles(): string[] {
  const listbox = screen.getByRole('listbox')
  return within(listbox)
    .getAllByRole('option')
    .map((o) => o.getAttribute('aria-label') ?? '')
}

describe('AgentSwitcher', () => {
  beforeEach(() => {
    navigateMock.mockClear()
  })

  it('渲染当前 Agent 的标题作为选中值', () => {
    renderSwitcher('agent-a')
    expect(screen.getByText('甲助手')).toBeInTheDocument()
  })

  it('选择其他 Agent 时以 replace 跳转到对应聊天页', () => {
    renderSwitcher('agent-a')
    const input = openSelect()
    // 打开时 activeIndex 停在当前值（agent-a），ArrowDown 移到 agent-b，Enter 选中
    fireEvent.keyDown(input, { key: 'ArrowDown', keyCode: 40, which: 40 })
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 13, which: 13 })
    expect(navigateMock).toHaveBeenCalledTimes(1)
    expect(navigateMock).toHaveBeenCalledWith('/agents/agent-b/chat', { replace: true })
  })

  it('重新选择当前 Agent 不触发跳转', () => {
    renderSwitcher('agent-a')
    const input = openSelect()
    // activeIndex 停在当前值上，直接 Enter = 重新选中 agent-a
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 13, which: 13 })
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('showSearch 按标题过滤选项', async () => {
    renderSwitcher('agent-a')
    openSelect()
    expect(listboxOptionTitles()).toEqual(['甲助手', '乙助手'])
    await userEvent.type(screen.getByRole('combobox'), '乙')
    expect(listboxOptionTitles()).toEqual(['乙助手'])
  })
})
