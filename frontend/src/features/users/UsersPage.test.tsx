import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import UsersPage from './UsersPage'

// UsersPage 按 auth.mode 分叉：builtin 显示邀请区，casdoor 改为注册引导。
// 这些测试只覆盖分叉渲染，不覆盖既有交互逻辑。
vi.mock('@/api/auth', () => ({
  authApi: {
    getAuthMode: vi.fn()
  }
}))

vi.mock('@/api/users', () => ({
  usersApi: {
    listUsers: vi.fn(),
    updateUser: vi.fn(),
    resetPassword: vi.fn(),
    listInvites: vi.fn(),
    createInvite: vi.fn(),
    revokeInvite: vi.fn(),
    getSignupUrl: vi.fn()
  }
}))

import { authApi } from '@/api/auth'
import { usersApi } from '@/api/users'

function renderUsersPage() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } }
  })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <UsersPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ConfigProvider>
  )
}

describe('UsersPage 按 auth.mode 分叉渲染', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(usersApi.listUsers).mockResolvedValue([])
    vi.mocked(usersApi.listInvites).mockResolvedValue([])
    vi.mocked(usersApi.getSignupUrl).mockResolvedValue({ signupUrl: 'https://casdoor.example.com/signup/org' })
  })

  it('builtin 模式：渲染「创建邀请」按钮和「邀请记录」标题', async () => {
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'builtin', initialized: true })
    renderUsersPage()
    expect(await screen.findByRole('button', { name: '创建邀请' })).toBeInTheDocument()
    expect(await screen.findByText('邀请记录')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '去 Casdoor 注册' })).not.toBeInTheDocument()
  })

  it('casdoor 模式：渲染「去 Casdoor 注册」，隐藏创建邀请与邀请记录', async () => {
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'casdoor', initialized: true })
    renderUsersPage()
    expect(await screen.findByRole('button', { name: '去 Casdoor 注册' })).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: '创建邀请' })).not.toBeInTheDocument()
      expect(screen.queryByText('邀请记录')).not.toBeInTheDocument()
    })
  })
})
