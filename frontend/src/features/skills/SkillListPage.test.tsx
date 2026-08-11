import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SkillListPage from './SkillListPage'
import type { Skill } from '@/api/skills'

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
})
