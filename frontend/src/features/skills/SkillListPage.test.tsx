import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SkillListPage from './SkillListPage'
import type { Skill } from '@/api/skills'
import { setAuthRole } from '@/test/auth-store-mock'

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

const mockSkills: Skill[] = [
  { id: 1, name: 'webapp', type: 'expert', title: 'Web应用', titleEn: 'Web App', description: 'Web 应用测试', descriptionEn: '', url: 'https://example.com/f.zip', fileHash: 'abcdef1234567890', fileSize: 10240, createdAt: '2026-06-10T10:00:00Z', updatedAt: '' },
  { id: 2, name: 'cli', type: 'community', title: 'CLI工具', titleEn: 'CLI Tool', description: '命令行工具', descriptionEn: '', url: '', fileHash: '', fileSize: 0, createdAt: '2026-06-12T10:00:00Z', updatedAt: '' }
]

vi.mock('@/queries/useSkills', () => ({
  useSkills: () => ({ data: mockSkills, isLoading: false }),
  useDeleteSkill: () => ({ mutate: vi.fn() }),
  useCreateSkill: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSkill: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

vi.mock('@/queries/useSkillMd', () => ({
  useSkillMd: () => ({ data: [], isLoading: false, error: null })
}))

vi.mock('@/api/skills', () => ({
  skillApi: { download: vi.fn() }
}))

describe('SkillListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders skill cards grouped by type', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <SkillListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    expect(screen.getByText('技能管理')).toBeInTheDocument()
    expect(screen.getByText('新建技能')).toBeInTheDocument()
    // Section headers
    expect(screen.getByText('专家技能')).toBeInTheDocument()
    expect(screen.getByText('社区技能')).toBeInTheDocument()
    // Card titles
    expect(screen.getByText('Web应用')).toBeInTheDocument()
    expect(screen.getByText('CLI工具')).toBeInTheDocument()
  })

  it('member: hides create/edit/delete but still sees skills and download', () => {
    setAuthRole('member')
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <SkillListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // 数据与只读下载入口仍可见：下载按钮（ArrowDownIcon，title="下载"）保留
    expect(screen.getByText('Web应用')).toBeInTheDocument()
    expect(screen.getByText('CLI工具')).toBeInTheDocument()
    expect(screen.getAllByTitle('下载').length).toBeGreaterThan(0)
    // 写操作按钮隐藏
    expect(screen.queryByText('新建技能')).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })
})
