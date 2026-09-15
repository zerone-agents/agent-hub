import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import { antdTheme } from '@/lib/antd-theme'
import GuestLandingPage from './GuestLandingPage'

// 退出登录走 auth store 的 logout，这里 mock store 避免真实网络请求。
const logoutMock = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: { logout: () => void }) => unknown) => selector({ logout: logoutMock })
}))

// 跳转走 react-router 的 useNavigate，mock 之并保留其余真实导出。
const navigateMock = vi.fn()
vi.mock('react-router', async () => ({
  ...(await vi.importActual<object>('react-router')),
  useNavigate: () => navigateMock,
}))

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <GuestLandingPage />
      </MemoryRouter>
    </ConfigProvider>
  )
}

describe('GuestLandingPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染体验模式标题与待审核文案', () => {
    renderPage()
    expect(screen.getByText('体验模式')).toBeInTheDocument()
    expect(
      screen.getByText('账号尚未开通管理权限（待审核）。可先前往 Agent 聊天页继续体验，或联系管理员分配角色。')
    ).toBeInTheDocument()
  })

  it('「前往 Agent 聊天」按钮跳转 /agents/chat', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '前往 Agent 聊天' }))
    expect(navigateMock).toHaveBeenCalledWith('/agents/chat')
  })

  it('「退出登录」调用 store logout 后显式跳转登录页', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '退出登录' }))
    expect(logoutMock).toHaveBeenCalledTimes(1)
    // 隐式依赖 RequireAuth 重渲染会卡在落地页（需二次点击/手动刷新）——必须显式 navigate
    await waitFor(() => { expect(navigateMock).toHaveBeenCalledWith('/login') })
  })
})
