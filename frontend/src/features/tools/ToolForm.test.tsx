import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import ToolForm from './ToolForm'
import type { Tool } from '@/api/tools'

const customReadyTool: Tool = {
  id: 2,
  name: 'SayHello',
  title: '问候',
  description: '问候工具',
  isDefault: false,
  source: 'custom',
  artifactStatus: 'ready',
  fileName: 'say.ts',
  fileHash: 'abcd1234abcd1234',
  fileSize: 1024,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z'
}

const customMissingTool: Tool = {
  id: 3,
  name: 'Legacy',
  title: '存量',
  description: '存量工具',
  isDefault: false,
  source: 'custom',
  artifactStatus: 'missing',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z'
}

vi.mock('@/queries/useTools', () => ({
  useCreateCustomTool: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUploadToolFile: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateTool: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

type FormMode = 'create' | 'edit' | 'upload'

const renderForm = (mode: FormMode, editingTool: Tool | null) =>
  render(
    <ConfigProvider theme={antdTheme}>
      <ToolForm open mode={mode} editingTool={editingTool} onClose={vi.fn()} />
    </ConfigProvider>
  )

describe('ToolForm', () => {
  it('create mode requires file upload and has no default switch', async () => {
    renderForm('create', null)
    expect(screen.getByText('上传自定义工具')).toBeInTheDocument() // modal 标题
    expect(screen.getByText(/选择 \.ts \/ \.mts \/ \.js \/ \.mjs 文件/)).toBeInTheDocument()
    expect(screen.queryByText('默认工具')).not.toBeInTheDocument()
  })

  it('create mode blocks submit without file', async () => {
    const user = userEvent.setup()
    renderForm('create', null)
    await user.type(screen.getByLabelText('工具标识'), 'SayHello')
    // antd 对两字中文按钮自动插入空格（"上 传"），用 \s* 兼容（KnowledgeForm.test 先例）
    await user.click(screen.getByRole('button', { name: /上\s*传/ }))
    await waitFor(() => expect(screen.getByText('请选择工具文件')).toBeInTheDocument())
  })

  it('edit mode shows metadata only for custom ready tool', async () => {
    renderForm('edit', customReadyTool)
    expect(screen.getByDisplayValue('SayHello')).toBeDisabled()
    expect(screen.getByText('替换文件（可选）')).toBeInTheDocument()
  })

  it('upload mode (backfill) requires file', async () => {
    renderForm('upload', customMissingTool)
    expect(screen.getByRole('button', { name: /补\s*传/ })).toBeInTheDocument()
  })
})
