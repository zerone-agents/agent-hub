import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import AppSidebar from './AppSidebar'
import { NAV_ITEMS } from '@/lib/nav'
import { setAuthRole } from '@/test/auth-store-mock'
vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())
// 结构性断言（i18n P3）：NAV_ITEMS.label 已 key 化，渲染输出为翻译值——
// 自反断言须同包 i18next.t 保持「数据→渲染」一致性语义。
import i18next from '@/i18n'

function renderSidebar(props: { collapsed?: boolean } = {}, route = '/dashboard') {
  return render(
    <MemoryRouter initialEntries={[route]}>
      <AppSidebar collapsed={props.collapsed ?? false} />
    </MemoryRouter>
  )
}

describe('AppSidebar', () => {
  beforeEach(() => { setAuthRole('admin') })
  it('renders all nav items with labels when expanded', () => {
    renderSidebar()
    for (const item of NAV_ITEMS) {
      expect(screen.getByText(i18next.t(item.label))).toBeInTheDocument()
    }
  })

  it('marks the nav item matching the current route as active', () => {
    renderSidebar({}, '/agents')
    const active = screen.getByRole('button', { name: /Agent管理/ })
    expect(active).toHaveAttribute('aria-current', 'page')
    const inactive = screen.getByRole('button', { name: /仪表盘/ })
    expect(inactive).not.toHaveAttribute('aria-current')
  })

  it('hides labels and shows only icons when collapsed', () => {
    renderSidebar({ collapsed: true })
    for (const item of NAV_ITEMS) {
      expect(screen.queryByText(i18next.t(item.label))).not.toBeInTheDocument()
      // icon-only buttons still expose an accessible name
      expect(screen.getByRole('button', { name: i18next.t(item.label) })).toBeInTheDocument()
    }
  })

  it.each(['member', 'guest', ''])('does not expose memory management to %s', (role) => {
    setAuthRole(role)
    renderSidebar()
    expect(screen.queryByRole('button', { name: '记忆管理' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '知识库管理' })).toBeInTheDocument()
  })

  it('shows memory management to a maintainer', () => {
    setAuthRole('maintainer')
    renderSidebar({}, '/memory')
    expect(screen.getByRole('button', { name: '记忆管理' })).toHaveAttribute('aria-current', 'page')
  })
})
