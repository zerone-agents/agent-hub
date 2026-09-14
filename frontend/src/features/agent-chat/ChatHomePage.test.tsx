import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import { antdTheme } from '@/lib/antd-theme'
import type { Agent } from '@/api/agents'
import ChatHomePage from './ChatHomePage'

// 可变 mock 状态：用例按矩阵切换 agents / user / mode（vi.mock 工厂被提升，须用 vi.hoisted）。
const state = vi.hoisted(() => ({
  agents: [] as Agent[],
  user: null as { id: string; name: string; email: string; role?: string } | null,
  mode: 'casdoor' as string | null | undefined
}))

const coderAgent: Agent = {
  id: 1,
  name: 'coder',
  config: { title: { zh: '编码助手' }, description: { zh: '写代码' }, modelId: 'gpt' },
  tools: [],
  skills: [],
  mcps: [],
  subagents: [],
  datasets: []
}
const writerAgent: Agent = {
  id: 2,
  name: 'writer',
  config: { title: { zh: '写作助手' }, modelId: 'gpt' },
  tools: [],
  skills: [],
  mcps: [],
  subagents: [],
  datasets: []
}

vi.mock('@/queries/useAgents', () => ({
  usePublicAgents: () => ({ data: state.agents, isLoading: false })
}))
vi.mock('@/features/login/useAuthMode', () => ({
  useAuthMode: () => ({ data: { mode: state.mode }, isLoading: false })
}))
vi.mock('@/queries/useUserInfo', () => ({
  useUserInfo: () => ({ data: state.user, isLoading: false, isError: false })
}))

// 退出登录走 auth store 的 logout，mock 之避免真实网络请求。
const logoutMock = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: { logout: () => void }) => unknown) => selector({ logout: logoutMock })
}))

// 跳转走 react-router 的 useNavigate，mock 之并保留其余真实导出。
const navigateMock = vi.fn()
vi.mock('react-router', async () => ({
  ...(await vi.importActual<object>('react-router')),
  useNavigate: () => navigateMock
}))

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <ChatHomePage />
      </MemoryRouter>
    </ConfigProvider>
  )
}

describe('ChatHomePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.agents = [coderAgent, writerAgent]
    state.user = { id: 'u1', name: 'tester', email: 't@zerone.run', role: 'member' }
    state.mode = 'casdoor'
  })

  it('渲染公开聊天视图的全部卡片', () => {
    renderPage()
    expect(screen.getByText('编码助手')).toBeInTheDocument()
    expect(screen.getByText('写作助手')).toBeInTheDocument()
  })

  it('点击卡片跳转该 Agent 聊天页', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /编码助手/ }))
    expect(navigateMock).toHaveBeenCalledWith('/agents/coder/chat')
  })

  it('guest 显示体验模式徽标，formal 不显示', () => {
    state.user = { id: 'u2', name: 'guest', email: 'g@zerone.run', role: undefined }
    renderPage()
    expect(screen.getByText('体验模式')).toBeInTheDocument()
  })

  it('正式用户无返回管理入口（新页签打开，无管理上下文）', () => {
    renderPage()
    expect(screen.queryByText('返回管理')).not.toBeInTheDocument()
    expect(screen.queryByText('体验模式')).not.toBeInTheDocument()
  })

  it('guest 空列表显示体验专属空态', () => {
    state.agents = []
    state.user = { id: 'u2', name: 'guest', email: 'g@zerone.run', role: undefined }
    renderPage()
    expect(
      screen.getByText('暂无可体验的 Agent，请联系管理员开放')
    ).toBeInTheDocument()
  })

  it('按 group 分组展示，未分组垫底', () => {
    state.agents = [
      // 空串 group（DB 列默认值）与 undefined 都必须归「未分组」——?? 不回退空串
      { ...writerAgent, group: '' },
      { ...coderAgent, group: 'DevOps' },
      { ...coderAgent, id: 3, name: 'ops', group: 'DevOps', config: { ...coderAgent.config, title: { zh: '运维助手' } } },
    ]
    renderPage()
    const devops = screen.getByText('DevOps')
    const fallback = screen.getByText('未分组')
    // 命名组在前，未分组垫底
    expect(devops.compareDocumentPosition(fallback) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 组内计数徽章：DevOps 2 / 未分组 1
    expect(devops.parentElement?.textContent).toContain('2')
    expect(fallback.parentElement?.textContent).toContain('1')
    expect(screen.getByText('编码助手')).toBeInTheDocument()
    expect(screen.getByText('运维助手')).toBeInTheDocument()
    expect(screen.getByText('写作助手')).toBeInTheDocument()
  })
})
