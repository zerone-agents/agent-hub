import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import CLITokensPage from './CLITokensPage'
import type { CLIToken } from '@/api/cli-tokens'

const mockTokens: CLIToken[] = [
  {
    id: 1,
    name: 'my-macbook',
    createdAt: '2026-06-01T00:00:00Z',
    lastUsedAt: null,
    expiresAt: '2026-08-30T00:00:00Z'
  },
  {
    id: 2,
    name: 'ci-server',
    createdAt: '2026-05-15T00:00:00Z',
    lastUsedAt: '2026-06-20T00:00:00Z',
    expiresAt: '2026-08-15T00:00:00Z'
  }
]

vi.mock('@/queries/useCLITokens', () => ({
  useCLITokens: () => ({ data: mockTokens, isLoading: false }),
  useIssueCLIToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRevokeCLIToken: () => ({ mutate: vi.fn() })
}))

const Wrapper = ({ children }: { children: React.ReactNode }) => (
  <ConfigProvider theme={antdTheme}>
    <MemoryRouter>{children}</MemoryRouter>
  </ConfigProvider>
)

describe('CLITokensPage', () => {
  it('renders page title and create button', () => {
    render(<CLITokensPage />, { wrapper: Wrapper })
    expect(screen.getByText('CLI Tokens')).toBeInTheDocument()
    expect(screen.getByText(/创建 CLI Token/)).toBeInTheDocument()
  })

  it('renders existing tokens in the table', () => {
    render(<CLITokensPage />, { wrapper: Wrapper })
    expect(screen.getByText('my-macbook')).toBeInTheDocument()
    expect(screen.getByText('ci-server')).toBeInTheDocument()
  })

  it('shows 从未使用 for tokens never used', () => {
    render(<CLITokensPage />, { wrapper: Wrapper })
    const cells = screen.getAllByText('从未使用')
    expect(cells.length).toBeGreaterThanOrEqual(1)
  })

  it('renders revoke buttons for each token', () => {
    render(<CLITokensPage />, { wrapper: Wrapper })
    const revokeButtons = screen.getAllByText('撤销')
    expect(revokeButtons).toHaveLength(2)
  })
})
