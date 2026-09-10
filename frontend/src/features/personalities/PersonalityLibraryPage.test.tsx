import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import { antdTheme } from '@/lib/antd-theme'
import { setAuthRole } from '@/test/auth-store-mock'
import type { Personality } from '@/api/personalities'
import PersonalityLibraryPage from './PersonalityLibraryPage'

vi.mock('@/stores/auth', async () =>
  (await import('@/test/auth-store-mock')).createAuthStoreMock(),
)

const personality: Personality = {
  id: 1,
  name: 'duty-whistleblower',
  title: '尽职揭弊者',
  description: '重事实与公共责任，敢于暴露被压下的问题。',
  prompt: '你以事实和可追溯证据为核心。\n\n常规渠道失效时，你会越级报告。',
  currentVersion: 2,
  enabled: true,
  isBuiltin: true,
  usageCount: 3,
  createdAt: '2026-09-09T10:00:00Z',
  updatedAt: '2026-09-10T10:00:00Z',
  versions: [
    {
      id: 2,
      templateId: 1,
      version: 2,
      prompt: 'v2',
      changeNote: '补充越级条件',
      createdAt: '2026-09-10T10:00:00Z',
    },
  ],
}

vi.mock('@/queries/usePersonalities', () => ({
  usePersonalities: () => ({ data: [personality], isLoading: false }),
  usePersonality: (name: string) => ({ data: name ? personality : undefined }),
  useDeletePersonality: () => ({ mutate: vi.fn() }),
  useCreatePersonality: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdatePersonality: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <PersonalityLibraryPage />
      </MemoryRouter>
    </ConfigProvider>,
  )
}

describe('PersonalityLibraryPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('presents prompt source, provenance and version history', async () => {
    renderPage()
    expect(screen.getByText('人格库')).toBeInTheDocument()
    expect(
      (await screen.findAllByText('尽职揭弊者')).length,
    ).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/常规渠道失效时/)).toBeInTheDocument()
    expect(screen.getByText('3 个 Agent 使用')).toBeInTheDocument()
    expect(screen.getByText('补充越级条件')).toBeInTheDocument()
    expect(screen.getByText('新建人格')).toBeInTheDocument()
  })

  it('keeps the library readable for members but hides authoring actions', async () => {
    setAuthRole('member')
    renderPage()
    expect(
      (await screen.findAllByText('尽职揭弊者')).length,
    ).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText('新建人格')).not.toBeInTheDocument()
    expect(screen.queryByTitle('编辑人格')).not.toBeInTheDocument()
  })
})
