import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
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

describe('UsersPage casdoor 待审批用户展示', () => {
  const pendingUser = {
    id: 'casdoor-u1',
    username: 'newbie',
    displayName: '新同学',
    email: 'newbie@example.com',
    role: '' as const,
    status: 'pending' as const,
    createdAt: '2026-08-18T10:00:00Z'
  }

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'casdoor', initialized: true })
    vi.mocked(usersApi.listUsers).mockResolvedValue([pendingUser])
    vi.mocked(usersApi.listInvites).mockResolvedValue([])
    vi.mocked(usersApi.getSignupUrl).mockResolvedValue({ signupUrl: 'https://casdoor.example.com/signup/org' })
  })

  it('pending 用户显示「待审批」标签，分配角色后调用 updateUser', async () => {
    vi.mocked(usersApi.updateUser).mockResolvedValue(undefined as never)
    renderUsersPage()
    expect(await screen.findByText('待审批')).toBeInTheDocument()
    // 通过角色 Select 分配角色 = 审批动作
    const combobox = await screen.findByRole('combobox')
    fireEvent.mouseDown(combobox)
    const option = await screen.findByText('maintainer', { selector: '.ant-select-item-option-content' })
    fireEvent.click(option)
    await waitFor(() => {
      expect(usersApi.updateUser).toHaveBeenCalledWith('casdoor-u1', { role: 'maintainer' })
    })
  })

  it('pending 用户不显示禁用/重置密码按钮', async () => {
    renderUsersPage()
    expect(await screen.findByText('待审批')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '禁用' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '启用' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重置密码' })).not.toBeInTheDocument()
  })
})
