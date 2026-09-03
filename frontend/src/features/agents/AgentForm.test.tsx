import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AgentForm from './AgentForm'
import type { Agent, AgentConfig } from '@/api/agents'

const { createAgent, updateAgent } = vi.hoisted(() => ({
  createAgent: vi.fn(),
  updateAgent: vi.fn(),
}))

vi.mock('@/queries/useAgents', () => ({
  useCreateAgent: () => ({ mutateAsync: createAgent, isPending: false }),
  useUpdateAgent: () => ({ mutateAsync: updateAgent, isPending: false }),
  useAgents: () => ({ data: [] as Agent[] }),
}))

function renderForm(editingAgent: Agent | null) {
  return render(
    <ConfigProvider theme={antdTheme}>
      <AgentForm open editingAgent={editingAgent} onClose={vi.fn()} />
    </ConfigProvider>
  )
}

describe('AgentForm disallowedTools', () => {
  beforeEach(() => {
    createAgent.mockReset()
    updateAgent.mockReset()
  })

  it('submits disallowedTools tags in create config', async () => {
    const user = userEvent.setup()
    renderForm(null)

    await user.type(screen.getByLabelText('代理标识'), 'deny-agent')
    // antd tags 模式下 placeholder 渲染为 span 而非 input 属性，
    // 经 Form.Item 容器定位 combobox（KnowledgeForm.test 先例）
    const disallowedItem = screen.getByText('禁用工具').closest<HTMLElement>('.ant-form-item')
    const tagsInput = within(disallowedItem!).getByRole('combobox')
    await user.type(tagsInput, 'Bash{enter}')
    await user.type(tagsInput, 'mcp__knowledge__lookup{enter}')
    await user.click(screen.getByRole('button', { name: '创建代理' }))

    await waitFor(() => { expect(createAgent).toHaveBeenCalledTimes(1) })
    const payload = createAgent.mock.calls[0][0] as { config: AgentConfig }
    expect(payload.config.disallowedTools).toEqual(['Bash', 'mcp__knowledge__lookup'])
  })

  it('omits disallowedTools key when no tags entered', async () => {
    const user = userEvent.setup()
    renderForm(null)

    await user.type(screen.getByLabelText('代理标识'), 'plain-agent')
    await user.click(screen.getByRole('button', { name: '创建代理' }))

    await waitFor(() => { expect(createAgent).toHaveBeenCalledTimes(1) })
    const payload = createAgent.mock.calls[0][0] as { config: AgentConfig }
    expect(payload.config.disallowedTools).toBeUndefined()
    // 空数组不下发该 key——与 hub 侧「未配置」语义对齐，避免存 '[]'
    expect(JSON.parse(JSON.stringify(payload.config))).not.toHaveProperty('disallowedTools')
  })

  it('backfills disallowedTools tags when editing and submits them on update', async () => {
    const user = userEvent.setup()
    const editingAgent: Agent = {
      id: 1,
      name: 'deny-agent',
      config: {
        permissionMode: 'auto',
        maxTurns: 50,
        disallowedTools: ['Bash', 'WebSearch'],
      },
    }
    renderForm(editingAgent)

    // 回填的禁用工具以 tag 形式可见
    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.getByText('WebSearch')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存更新' }))

    await waitFor(() => { expect(updateAgent).toHaveBeenCalledTimes(1) })
    const payload = updateAgent.mock.calls[0][0] as { name: string; data: { config: AgentConfig } }
    expect(payload.name).toBe('deny-agent')
    expect(payload.data.config.disallowedTools).toEqual(['Bash', 'WebSearch'])
  })
})
