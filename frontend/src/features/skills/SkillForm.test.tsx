import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SkillForm from './SkillForm'

// hoisted: 让测试能断言提交时 mutateAsync/parseSkillMd 是否被调用
const { createSkillMock, updateSkillMock, parseSkillMdMock } = vi.hoisted(() => ({
  createSkillMock: vi.fn(),
  updateSkillMock: vi.fn(),
  parseSkillMdMock: vi.fn()
}))

vi.mock('@/queries/useSkills', () => ({
  useCreateSkill: () => ({ mutateAsync: createSkillMock, isPending: false }),
  useUpdateSkill: () => ({ mutateAsync: updateSkillMock, isPending: false })
}))

vi.mock('@/queries/useSkillMd', () => ({
  useSkillMd: () => ({ data: undefined, isLoading: false, error: null })
}))

// 不解析真实 zip（JSZip），beforeUpload 里的预览解析直接 mock
vi.mock('./parseSkillMd', () => ({ parseSkillMd: parseSkillMdMock }))

// 预览组件是 lazy import（@lobehub/ui Markdown 过重），测试里用替身
vi.mock('./SkillMdPreview', () => ({
  default: () => null
}))

const renderForm = () =>
  render(
    <ConfigProvider theme={antdTheme}>
      <SkillForm open editingSkill={null} onClose={vi.fn()} />
    </ConfigProvider>
  )

describe('SkillForm', () => {
  it('renders create mode with upload placeholder', () => {
    renderForm()
    expect(screen.getByText('新建技能')).toBeInTheDocument()
    expect(screen.getByText('选择 .zip 文件')).toBeInTheDocument()
  })

  it('#96: failed re-selection clears stale selectedFile and blocks submit with old file', async () => {
    createSkillMock.mockClear()
    parseSkillMdMock.mockResolvedValue([])
    const user = userEvent.setup()
    renderForm()
    // rc-upload 每次选择后都会 setState 更换 input 的 key（元素被 React 重建），
    // 必须每次上传前重新查询，不能缓存引用
    const uploadInput = () => document.querySelector('input[type="file"]') as HTMLInputElement

    // 先选一个有效文件
    await user.upload(uploadInput(), new File(['zip'], 'good.zip', { type: 'application/zip' }))
    expect(screen.getByText('good.zip')).toBeInTheDocument()

    // 再选一个校验失败的文件：按钮应回到占位文案，而不是残留旧的 good.zip。
    // 第二次上传用 fireEvent.change：userEvent.upload 与 rc-upload 的
    // key={uid} 重建 input 交互有兼容问题（第二次调用不会派发 change，探针实证），
    // fireEvent.change 是 antd 官方测试的确定性做法，且同样命中 beforeUpload 校验路径
    fireEvent.change(uploadInput(), { target: { files: [new File(['x'], 'bad.txt', { type: 'text/plain' })] } })
    expect(screen.getByText('仅支持 .zip 格式文件')).toBeInTheDocument()
    // 失败路径下错误文案接管，默认提示（hint）隐藏——「已选→失败→未选」状态机一致
    expect(screen.queryByText('ZIP 包内须包含 SKILL.md（位于根目录或子目录均可），最大 50MB')).not.toBeInTheDocument()
    expect(screen.queryByText('good.zip')).not.toBeInTheDocument()
    expect(screen.getByText('选择 .zip 文件')).toBeInTheDocument()

    // 提交被拦截：提示重新选择文件，而不是把旧的 good.zip 提交上去
    await user.type(screen.getByLabelText('技能标识'), 'webapp-testing')
    await user.click(screen.getByRole('button', { name: /创\s*建/ }))
    await waitFor(() => expect(screen.getByText('请选择要上传的 .zip 文件')).toBeInTheDocument())
    expect(createSkillMock).not.toHaveBeenCalled()
  })
})