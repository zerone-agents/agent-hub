import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SessionListPanel from './SessionListPanel'
import { setAuthRole } from '@/test/auth-store-mock'

// vi.mock 工厂会被提升，用 async 工厂动态 import helper（不能引用静态 import）
vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

// Helpers to override per-test
let sessionsMock: any[] = []
let providersMock: any[] = []

vi.mock('@/queries/useChat', () => ({
  useChatSessions: () => ({
    data: { items: sessionsMock, total: sessionsMock.length, page: 1, page_size: 30, total_pages: 1 },
    isLoading: false,
    page: 1,
    pageSize: 30,
    setPage: vi.fn(),
  }),
  useDeleteChatSession: () => ({ mutate: vi.fn() }),
}))

vi.mock('@/queries/useProviders', () => ({
  useProviders: () => ({ data: providersMock }),
}))

function renderPanel() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <SessionListPanel selectedId={null} onSelect={vi.fn()} />
      </MemoryRouter>
    </ConfigProvider>
  )
}

describe('SessionListPanel — model display resolver', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('falls back to session.model when no provider catalog match', () => {
    sessionsMock = [
      { id: 's1', title: '会话A', user_id: 'u1', model: 'kimi-k3', updated_at: '2026-07-18T10:00:00Z' },
    ]
    providersMock = []

    renderPanel()
    expect(screen.getByText('kimi-k3')).toBeInTheDocument()
  })

  it('admin: renders delete button on session item', () => {
    sessionsMock = [
      { id: 's1', title: '会话A', user_id: 'u1', model: 'kimi-k3', updated_at: '2026-07-18T10:00:00Z' },
    ]
    providersMock = []

    renderPanel()
    expect(screen.getByTitle('删除')).toBeInTheDocument()
  })

  it('member: hides delete button but still sees sessions (readonly)', () => {
    setAuthRole('member')
    sessionsMock = [
      { id: 's1', title: '会话A', user_id: 'u1', model: 'kimi-k3', updated_at: '2026-07-18T10:00:00Z' },
    ]
    providersMock = []

    renderPanel()

    // 数据仍可见（只读）
    expect(screen.getByText('会话A')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByTitle('删除')).not.toBeInTheDocument()
  })

  it('resolves displayName via (provider_id, model_selection_id)', () => {
    sessionsMock = [
      {
        id: 's2',
        title: '会话B',
        user_id: 'u1',
        model: 'kimi-k3',
        model_selection_id: 'kimi-k3-2',
        provider_id: '7',
        updated_at: '2026-07-18T10:00:00Z',
      },
    ]
    providersMock = [
      {
        id: 7,
        defaultModels: [
          { selectionId: 'kimi-k3', modelId: 'kimi-k3', displayName: 'Kimi-K3' },
          { selectionId: 'kimi-k3-2', modelId: 'kimi-k3', displayName: 'Kimi-K3 (1M)' },
        ],
      },
    ]

    renderPanel()
    expect(screen.getByText('Kimi-K3 (1M)')).toBeInTheDocument()
    expect(screen.queryByText('kimi-k3')).not.toBeInTheDocument()
  })

  it('falls back to session.model when selectionId no longer in catalog', () => {
    // 历史快照场景：provider 配置已变，selectionId 已不存在
    sessionsMock = [
      {
        id: 's3',
        title: '会话C',
        user_id: 'u1',
        model: 'kimi-k3',
        model_selection_id: 'kimi-k3-2',
        provider_id: '7',
        updated_at: '2026-07-18T10:00:00Z',
      },
    ]
    providersMock = [
      { id: 7, defaultModels: [{ selectionId: 'kimi-k3', modelId: 'kimi-k3', displayName: 'Kimi-K3' }] },
    ]

    renderPanel()
    expect(screen.getByText('kimi-k3')).toBeInTheDocument()
  })
})
