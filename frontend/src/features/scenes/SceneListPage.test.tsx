import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SceneListPage from './SceneListPage'
import type { Scene } from '@/api/scenes'
import { setAuthRole } from '@/test/auth-store-mock'

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

const mockScenes: Scene[] = [
  { id: 1, name: 'chat', agent: 'general', title: '聊天', titleEn: 'Chat', prompt: 'You are a chatbot', promptEn: '', enabled: true, createdAt: '2026-06-10T10:00:00Z', updatedAt: '' },
  { id: 2, name: 'code', agent: 'coder', title: '编程', titleEn: 'Code', prompt: 'Write code', promptEn: '', enabled: false, createdAt: '2026-06-12T10:00:00Z', updatedAt: '' }
]

vi.mock('@/queries/useScenes', () => ({
  useScenes: () => ({ data: mockScenes, isLoading: false }),
  useDeleteScene: () => ({ mutate: vi.fn() }),
  useCreateScene: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateScene: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

vi.mock('@/queries/useAgents', () => ({
  useAgents: () => ({
    data: [{ id: 1, name: 'general', config: { title: { zh: '通用助手' } } }],
    isLoading: false
  })
}))

describe('SceneListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders scene table with data', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <SceneListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    expect(screen.getByText('场景管理')).toBeInTheDocument()
    expect(screen.getByText('chat')).toBeInTheDocument()
    expect(screen.getByText('code')).toBeInTheDocument()
    expect(screen.getByText('新建场景')).toBeInTheDocument()
  })

  it('member: hides create/edit/delete but still sees scenes', () => {
    setAuthRole('member')
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <SceneListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // 数据仍可见（只读）
    expect(screen.getByText('场景管理')).toBeInTheDocument()
    expect(screen.getByText('chat')).toBeInTheDocument()
    expect(screen.getByText('code')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByText('新建场景')).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })
})
