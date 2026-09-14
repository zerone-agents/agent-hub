import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { antdTheme } from '@/lib/antd-theme'
import RequireAuth from './RequireAuth'

// 可变 mock 状态：各用例按矩阵切换 token / user / mode。
// vi.mock 工厂会被提升到 import 之前执行，须用 vi.hoisted 引用。
const state = vi.hoisted(() => ({
  token: 'tok' as string | null,
  user: null as { id: string; name: string; email: string; role?: string } | null,
  mode: 'casdoor' as string | null | undefined
}))

vi.mock('@/api/client', () => ({
  default: vi.fn(),
  getAccessToken: () => state.token,
  clearTokens: vi.fn(),
  setTokens: vi.fn()
}))
vi.mock('@/queries/useUserInfo', () => ({
  useUserInfo: () => ({ data: state.user, isLoading: false, isError: false })
}))
vi.mock('@/features/login/useAuthMode', () => ({
  useAuthMode: () => ({ data: { mode: state.mode }, isLoading: false })
}))

/** 把当前 location 打到 DOM 上，让 <Navigate> 的目标（含 search）可断言。 */
function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>
}

function renderGuarded(path: string, props: { allowGuest?: boolean } = {}) {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/login" element={<LocationProbe />} />
          <Route path="/agents/chat" element={<LocationProbe />} />
          <Route
            path="/*"
            element={
              <RequireAuth allowGuest={props.allowGuest}>
                <div>child-content</div>
              </RequireAuth>
            }
          />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>
  )
}

describe('RequireAuth', () => {
  beforeEach(() => {
    state.token = 'tok'
    state.user = { id: '1', name: 'admin', email: 'admin@zerone.run', role: 'member' }
    state.mode = 'casdoor'
  })

  it('无 token 重定向 /login 且 redirect 携带当前路径', () => {
    state.token = null
    renderGuarded('/dashboard')
    expect(screen.getByTestId('location')).toHaveTextContent('/login?redirect=%2Fdashboard')
  })

  it('无 token 的 redirect 参数编码 pathname+search', () => {
    state.token = null
    renderGuarded('/dashboard?tab=x')
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/login?redirect=%2Fdashboard%3Ftab%3Dx'
    )
  })

  it('无 token 的 redirect 参数编码 pathname+search+hash', () => {
    state.token = null
    renderGuarded('/dashboard#frag')
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/login?redirect=%2Fdashboard%23frag'
    )
  })

  it('guest × allowGuest 渲染 children', () => {
    state.user = { id: '2', name: 'guest', email: 'g@zerone.run', role: 'guest' }
    state.mode = 'builtin'
    renderGuarded('/agents/chat/agent-1', { allowGuest: true })
    expect(screen.getByText('child-content')).toBeInTheDocument()
  })

  it('guest × 管理路由渲染 GuestLandingPage（前往 Agent 聊天）', () => {
    state.user = { id: '2', name: 'guest', email: 'g@zerone.run', role: 'guest' }
    state.mode = 'builtin'
    renderGuarded('/dashboard')
    expect(screen.getByText('体验模式')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '前往 Agent 聊天' })).toBeInTheDocument()
    expect(screen.queryByText('child-content')).not.toBeInTheDocument()
  })

  it('guest × / 重定向 /agents/chat', () => {
    state.user = { id: '2', name: 'guest', email: 'g@zerone.run', role: undefined }
    state.mode = 'casdoor'
    renderGuarded('/')
    expect(screen.getByTestId('location')).toHaveTextContent('/agents/chat')
  })

  it('正式用户渲染 children', () => {
    renderGuarded('/dashboard')
    expect(screen.getByText('child-content')).toBeInTheDocument()
  })
})
