import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import ExtensionAcceptancePage from './ExtensionAcceptancePage'

const mutate = vi.fn()
const reset = vi.fn()
const info = {
  platformVersion: '0.9.0-h0',
  protocolVersion: 'v1alpha1',
  status: '可验收',
  examples: [
    { id: 'speeding', name: 'Speeding', kind: '垂直应用', description: '财富处置游戏示例', manifest: 'metadata:\n  namespace: speeding' },
    { id: 'research', name: '通用研究团队', kind: '平台示例', description: '非游戏多 Agent 示例', manifest: 'metadata:\n  namespace: research' },
  ],
  contributionCategories: [
    { key: 'state', label: '状态 Schema', description: '声明扩展状态' },
    { key: 'events', label: '事件', description: '声明领域事件' },
  ],
}

let validationState: Record<string, unknown>

vi.mock('@/queries/useExtensions', () => ({
  useH0AcceptanceInfo: () => ({ data: info, isLoading: false, isError: false }),
  useValidateExtension: () => ({ mutate, reset, isPending: false, isError: false, ...validationState }),
}))

function renderPage() {
  return render(<ConfigProvider theme={antdTheme}><ExtensionAcceptancePage /></ConfigProvider>)
}

describe('ExtensionAcceptancePage', () => {
  beforeEach(() => {
    mutate.mockReset()
    reset.mockReset()
    validationState = {}
  })

  it('shows protocol boundary, examples and contribution categories', async () => {
    renderPage()
    expect(screen.getByRole('heading', { name: '扩展能力验收' })).toBeInTheDocument()
    expect(screen.getByText('v1alpha1')).toBeInTheDocument()
    expect(screen.getByText(/垂直应用示例仅用于验证/)).toBeInTheDocument()
    expect(screen.getByText('状态 Schema')).toBeInTheDocument()
    expect(await screen.findByDisplayValue(/namespace: speeding/)).toBeInTheDocument()
  })

  it('submits the edited manifest for backend validation', async () => {
    renderPage()
    const editor = await screen.findByLabelText('extension.yaml')
    fireEvent.change(editor, { target: { value: 'metadata:\n  namespace: custom' } })
    fireEvent.click(screen.getByRole('button', { name: '校验扩展包' }))
    expect(mutate).toHaveBeenCalledWith('metadata:\n  namespace: custom')
  })

  it('renders a successful backend result', () => {
    validationState = {
      data: { valid: true, package: { name: 'speeding', namespace: 'speeding', version: '1.0.0' } },
    }
    renderPage()
    expect(screen.getByText('扩展包校验通过')).toBeInTheDocument()
    expect(screen.getByTestId('validation-success')).toHaveTextContent('speeding')
  })

  it('renders field-level validation errors', () => {
    validationState = {
      data: { valid: false, errors: [{ path: '/metadata/namespace', message: '必填字段缺失' }] },
    }
    renderPage()
    expect(screen.getByText('校验未通过')).toBeInTheDocument()
    expect(screen.getByTestId('validation-errors')).toHaveTextContent('/metadata/namespace')
    expect(screen.getByTestId('validation-errors')).toHaveTextContent('必填字段缺失')
  })

  it('switches to the generic non-game example', async () => {
    renderPage()
    fireEvent.mouseDown(screen.getByLabelText('验收示例'))
    fireEvent.click(await screen.findByText('通用研究团队 · 平台示例'))
    await waitFor(() => {
      expect((screen.getByLabelText('extension.yaml') as HTMLTextAreaElement).value).toContain('namespace: research')
    })
  })
})
