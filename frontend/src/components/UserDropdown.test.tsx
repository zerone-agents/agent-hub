import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { UsersIcon } from '@phosphor-icons/react'
import { antdTheme } from '@/lib/antd-theme'
import UserDropdown from './UserDropdown'
import { setAuthRole } from '@/test/auth-store-mock'

vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

// 密码修改弹窗依赖较多，本测试只关心菜单项
vi.mock('@/features/users/ChangePasswordModal', () => ({
  default: () => null,
}))

function renderDropdown(extraItems?: Parameters<typeof UserDropdown>[0]['extraItems']) {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <UserDropdown extraItems={extraItems} />
      </MemoryRouter>
    </ConfigProvider>
  )
}

async function openUserMenu() {
  const user = userEvent.setup()
  const avatar = document.querySelector('.ant-avatar') as HTMLElement
  await user.click(avatar)
}

describe('UserDropdown', () => {
  it('默认菜单仅含修改密码与退出登录（聊天页形态）', async () => {
    setAuthRole('admin')
    renderDropdown()
    await openUserMenu()
    expect(await screen.findByText('修改密码')).toBeInTheDocument()
    expect(screen.getByText('退出登录')).toBeInTheDocument()
    // 即使 admin 也不出现管理页专属入口——extraItems 未注入
    expect(screen.queryByText('用户管理')).not.toBeInTheDocument()
    expect(screen.queryByText('CLI Tokens')).not.toBeInTheDocument()
    expect(screen.queryByText('AIGC 标识配置')).not.toBeInTheDocument()
  })

  it('extraItems 注入在修改密码之前', async () => {
    renderDropdown([
      { key: 'users', icon: <UsersIcon size={14} />, label: '用户管理' },
    ])
    await openUserMenu()
    const extra = await screen.findByText('用户管理')
    const pwd = screen.getByText('修改密码')
    expect(
      extra.compareDocumentPosition(pwd) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })
})
