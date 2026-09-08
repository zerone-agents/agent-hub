import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
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
  descriptionEn: 'Greeting tool',
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

// hoisted: 让测试能断言提交时 mutateAsync 是否被调用（#96 回归用例需要）
const { createCustomToolMock, uploadToolFileMock, updateToolMock } = vi.hoisted(() => ({
  createCustomToolMock: vi.fn(),
  uploadToolFileMock: vi.fn(),
  updateToolMock: vi.fn()
}))

vi.mock('@/queries/useTools', () => ({
  useCreateCustomTool: () => ({ mutateAsync: createCustomToolMock, isPending: false }),
  useUploadToolFile: () => ({ mutateAsync: uploadToolFileMock, isPending: false }),
  useUpdateTool: () => ({ mutateAsync: updateToolMock, isPending: false })
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

  it('#96: failed re-selection clears stale selectedFile and blocks submit with old file', async () => {
    createCustomToolMock.mockClear()
    const user = userEvent.setup()
    renderForm('create', null)
    // rc-upload 每次选择后都会 setState 更换 input 的 key（元素被 React 重建），
    // 必须每次上传前重新查询，不能缓存引用
    const uploadInput = () => document.querySelector('input[type="file"]') as HTMLInputElement

    // 先选一个有效文件
    await user.upload(uploadInput(), new File(['export {}'], 'Hello.ts', { type: 'text/typescript' }))
    expect(screen.getByText('Hello.ts')).toBeInTheDocument()

    // 再选一个校验失败的文件：按钮应回到占位文案，而不是残留旧的 Hello.ts。
    // 第二次上传用 fireEvent.change：userEvent.upload 与 rc-upload 的
    // key={uid} 重建 input 交互有兼容问题（第二次调用不会派发 change，探针实证），
    // fireEvent.change 是 antd 官方测试的确定性做法，且同样命中 beforeUpload 校验路径
    fireEvent.change(uploadInput(), { target: { files: [new File(['x'], 'bad.txt', { type: 'text/plain' })] } })
    expect(screen.getByText('仅支持 .ts / .mts / .js / .mjs 文件')).toBeInTheDocument()
    expect(screen.queryByText('Hello.ts')).not.toBeInTheDocument()
    expect(screen.getByText('选择 .ts / .mts / .js / .mjs 文件')).toBeInTheDocument()

    // 提交被拦截：提示重新选择文件，而不是把旧的 Hello.ts 提交上去
    await user.type(screen.getByLabelText('工具标识'), 'SayHello')
    await user.click(screen.getByRole('button', { name: /上\s*传/ }))
    await waitFor(() => expect(screen.getByText('请选择工具文件')).toBeInTheDocument())
    expect(createCustomToolMock).not.toHaveBeenCalled()
  })

  it('#93: edit mode prefills the English description field', async () => {
    renderForm('edit', customReadyTool)
    expect(screen.getByLabelText('Description (EN)')).toHaveValue('Greeting tool')
  })

  it('#93: create submit passes descriptionEn through', async () => {
    createCustomToolMock.mockClear()
    const user = userEvent.setup()
    renderForm('create', null)
    await user.type(screen.getByLabelText('工具标识'), 'SayHello')
    await user.type(screen.getByLabelText('功能描述'), '问候工具')
    await user.type(screen.getByLabelText('Description (EN)'), 'Greeting tool')
    // 与 #96 用例同款确定性做法：单次文件选择用 fireEvent.change
    const uploadInput = () => document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(uploadInput(), {
      target: { files: [new File(['export {}'], 'Hello.ts', { type: 'text/typescript' })] }
    })
    await user.click(screen.getByRole('button', { name: /上\s*传/ }))
    await waitFor(() => {
      expect(createCustomToolMock).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'SayHello',
          description: '问候工具',
          descriptionEn: 'Greeting tool'
        })
      )
    })
  })

  it('#93: edit submit passes descriptionEn through', async () => {
    updateToolMock.mockClear()
    const user = userEvent.setup()
    renderForm('edit', customReadyTool)
    const enField = screen.getByLabelText('Description (EN)')
    await user.clear(enField)
    await user.type(enField, 'Greeting tool v2')
    await user.click(screen.getByRole('button', { name: /更\s*新/ }))
    await waitFor(() => {
      expect(updateToolMock).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'SayHello',
          data: expect.objectContaining({ descriptionEn: 'Greeting tool v2' })
        })
      )
    })
  })
})
