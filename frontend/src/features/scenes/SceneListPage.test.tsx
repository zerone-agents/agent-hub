import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SceneListPage from './SceneListPage'
import type { Scene } from '@/api/scenes'

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
})
