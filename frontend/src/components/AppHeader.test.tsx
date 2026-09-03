import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AppHeader from './AppHeader'
import { setAuthRole } from '@/test/auth-store-mock'

vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

// 知识库面包屑 hook 与本测试无关
vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeDetail: () => ({ data: undefined }),
}))

// 密码修改弹窗依赖较多，本测试只关心用户菜单项
vi.mock('@/features/users/ChangePasswordModal', () => ({
  default: () => null,
}))

function renderHeader() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={['/dashboard']}>
        <AppHeader onToggleSidebar={vi.fn()} />
      </MemoryRouter>
    </ConfigProvider>
  )
}

async function openUserMenu() {
  const user = userEvent.setup()
  // Dropdown trigger=click，头像按钮在 header 右侧
  const avatar = document.querySelector('.ant-avatar') as HTMLElement
  await user.click(avatar)
}

describe('AppHeader 页眉链接', () => {
  it('包含 GitHub 仓库与官方网站链接，新窗口打开', () => {
    renderHeader()
    const github = screen.getByRole('link', { name: 'GitHub 仓库' })
    expect(github).toHaveAttribute('href', 'https://github.com/zerone-agents/agent-hub')
    expect(github).toHaveAttribute('target', '_blank')
    expect(github).toHaveAttribute('rel', 'noopener noreferrer')
    const site = screen.getByRole('link', { name: '官方网站' })
    expect(site).toHaveAttribute('href', 'https://www.zerone.run/')
    expect(site).toHaveAttribute('target', '_blank')
  })
})

describe('AppHeader 用户菜单', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('admin: 用户菜单含 CLI Tokens 与 AIGC 标识配置', async () => {
    renderHeader()
    await openUserMenu()
    expect(await screen.findByText('CLI Tokens')).toBeInTheDocument()
    expect(screen.getByText('AIGC 标识配置')).toBeInTheDocument()
    expect(screen.getByText('用户管理')).toBeInTheDocument()
  })

  it('member: 用户菜单无 CLI Tokens / AIGC / 用户管理，保留修改密码与退出', async () => {
    setAuthRole('member')
    renderHeader()
    await openUserMenu()
    expect(await screen.findByText('修改密码')).toBeInTheDocument()
    expect(screen.queryByText('CLI Tokens')).not.toBeInTheDocument()
    expect(screen.queryByText('AIGC 标识配置')).not.toBeInTheDocument()
    expect(screen.queryByText('用户管理')).not.toBeInTheDocument()
    expect(screen.getByText('退出登录')).toBeInTheDocument()
  })
})
