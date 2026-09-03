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

// antd Select 逐键输入较重，全套并行负载下单测易超 5s 默认预算（超时还会连坐污染后续用例），
// 与 AgentChatPage.test.tsx 一致放宽 describe 级超时
describe('AgentForm disallowedTools', { timeout: 15000 }, () => {
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
    // 新建态未触碰 → undefined → 序列化丢 key，hub 侧视为「未配置」
    expect(JSON.parse(JSON.stringify(payload.config))).not.toHaveProperty('disallowedTools')
  })

  it('submits an explicit empty disallowedTools array when editing clears all tags', async () => {
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

    const disallowedItem = screen.getByText('禁用工具').closest<HTMLElement>('.ant-form-item')!
    expect(within(disallowedItem).getByText('Bash')).toBeInTheDocument()
    expect(within(disallowedItem).getByText('WebSearch')).toBeInTheDocument()

    // 逐个点击 antd Select tag 的移除图标，清空全部禁用工具（每次重查，避免 re-render 后节点失效）
    for (let i = 0; i < 2; i++) {
      const remove = disallowedItem.querySelector('.ant-select-selection-item-remove')
      expect(remove).not.toBeNull()
      await user.click(remove!)
    }
    expect(within(disallowedItem).queryByText('Bash')).not.toBeInTheDocument()
    expect(within(disallowedItem).queryByText('WebSearch')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存更新' }))

    await waitFor(() => { expect(updateAgent).toHaveBeenCalledTimes(1) })
    const payload = updateAgent.mock.calls[0][0] as { name: string; data: { config: AgentConfig } }
    expect(payload.name).toBe('deny-agent')
    // 显式空数组而非 key 缺席——hub applyUpdateConfig 对 absent key 不变更，[] 才能清空 deny 名单
    expect(payload.data.config.disallowedTools).toEqual([])
    const serialized = JSON.parse(JSON.stringify(payload.data.config)) as AgentConfig
    expect(serialized).toHaveProperty('disallowedTools')
    expect(serialized.disallowedTools).toEqual([])
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
