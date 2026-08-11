import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import LoginPage from './LoginPage'

vi.mock('@/api/auth', () => ({
  authApi: {
    login: vi.fn()
  }
}))

import { authApi } from '@/api/auth'

function renderLogin() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } }
  })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <LoginPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ConfigProvider>
  )
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders brand and login button', () => {
    renderLogin()
    expect(screen.getByText('Zerone Hub')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Casdoor/ })).toBeInTheDocument()
  })

  it('calls authApi.login on button click', async () => {
    const user = userEvent.setup()
    renderLogin()
    const btn = screen.getByRole('button', { name: /Casdoor/ })
    await user.click(btn)
    expect(authApi.login).toHaveBeenCalledTimes(1)
  })
})
