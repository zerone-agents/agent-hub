import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import PendingApprovalPage from './PendingApprovalPage'

// 退出登录走 auth store 的 logout，这里 mock store 避免真实网络请求。
const logoutMock = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: { logout: () => void }) => unknown) => selector({ logout: logoutMock })
}))

describe('PendingApprovalPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染等待审批提示与联系管理员文案', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <PendingApprovalPage />
      </ConfigProvider>
    )
    expect(screen.getByText('账号待审批')).toBeInTheDocument()
    expect(
      screen.getByText('账号待审批：你已成功登录，但账号尚未被租户管理员分配角色。请联系管理员开通。')
    ).toBeInTheDocument()
  })

  it('提供退出登录按钮并调用 logout', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <PendingApprovalPage />
      </ConfigProvider>
    )
    const btn = screen.getByRole('button', { name: '退出登录' })
    fireEvent.click(btn)
    expect(logoutMock).toHaveBeenCalledTimes(1)
  })
})
