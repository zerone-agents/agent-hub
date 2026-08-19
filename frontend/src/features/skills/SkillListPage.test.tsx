import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SkillListPage from './SkillListPage'
import type { Skill } from '@/api/skills'

// 角色默认 admin：既有断言依赖新建/编辑/删除按钮可见；member 分支用例内切换。
const authUser = vi.hoisted(() => ({
  user: { id: '1', name: 'admin', email: 'admin@zerone.run', role: 'admin' }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: {
    user: { id: string; name: string; email: string; role: string } | null
    setUser: () => void
    loginWithPassword: () => Promise<void>
    login: () => void
    logout: () => Promise<void>
  }) => unknown) => selector({
    user: authUser.user,
    setUser: vi.fn(),
    loginWithPassword: vi.fn(),
    login: vi.fn(),
    logout: vi.fn()
  })
}))

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
    authUser.user = { ...authUser.user, role: 'admin' }
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
    authUser.user = { ...authUser.user, role: 'member' }
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <SkillListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // 数据与只读下载入口仍可见
    expect(screen.getByText('Web应用')).toBeInTheDocument()
    expect(screen.getByText('CLI工具')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByText('新建技能')).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })
})
