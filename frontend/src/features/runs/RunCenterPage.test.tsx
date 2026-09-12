import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { MemoryRouter, Route, Routes } from 'react-router'
import { antdTheme } from '@/lib/antd-theme'
import RunCenterPage from './RunCenterPage'

const runs = [
  {
    id: 'run-market',
    name: '市场研究',
    description: '评估新市场机会',
    status: 'running',
    createdBy: '林经理',
    createdAt: '2026-09-12T08:00:00Z',
    updatedAt: '2026-09-12T09:00:00Z',
    agents: [{ id: 11, agentId: 7, agentNameSnapshot: '研究分析师', role: '主分析' }],
    capabilityBindings: [{ namespace: 'io.zerone.research', packageName: '研究协作', version: '1.0.0' }],
  },
  {
    id: 'run-risk',
    name: '风险审查',
    status: 'paused',
    createdAt: '2026-09-12T07:00:00Z',
    updatedAt: '2026-09-12T07:30:00Z',
    agents: [{ id: 12, agentId: 7, agentNameSnapshot: 'analyst', role: '复核' }],
  },
] as const

const createMutate = vi.fn()
const transitionMutate = vi.fn()
const addAgentMutate = vi.fn()
const composePromptMutate = vi.fn((_variables: unknown, options?: { onSuccess?: (value: unknown) => void }) => options?.onSuccess?.({
  id: 'prompt-1', runId: 'run-market', runAgentId: 11, agentId: 7,
  renderedText: '## 平台安全边界\n不得越权。\n\n## 职责\n分析市场。', renderedHash: '1234567890abcdef',
  provenance: [
    { stage: 'platform_safety', label: '平台安全边界', sourceType: 'platform', sourceId: 'agenthub.safety', sourceVersion: '1', contentHash: 'a'.repeat(64), tokenEstimate: 12 },
    { stage: 'responsibilities', label: '职责', sourceType: 'agent_snapshot', sourceId: '研究分析师', sourceVersion: 'v1', contentHash: 'b'.repeat(64), tokenEstimate: 8 },
  ],
  createdAt: '2026-09-12T09:00:00Z',
}))
let detailStatus = 'running'
let collaborationMessages: unknown[] = []

vi.mock('@/hooks/useCanWrite', () => ({ useCanWrite: () => true }))
vi.mock('@/queries/useAgents', () => ({
  useAgents: () => ({ data: [{ id: 9, name: 'reviewer', config: { title: { 'zh-CN': '风险复核员' } } }] }),
}))

vi.mock('@/queries/useRuns', () => ({
  useRuns: () => ({ data: runs, isLoading: false, isError: false }),
  useRun: () => ({
    data: {
      run: { ...runs[0], status: detailStatus },
      states: [{ id: 21, namespace: 'io.zerone.research', schemaName: 'progress', schemaVersion: '1', subjectType: 'agent', subjectId: 'analyst', revision: 2, data: { progress: 65, confidence: 0.8 } }],
    },
    isLoading: false,
    isError: false,
  }),
  useRunStateChanges: () => ({
    data: [{ id: 'change-31', runStateId: 21, revisionBefore: 1, revisionAfter: 2, before: { progress: 30 }, after: { progress: 65 }, reason: '完成竞品分析', source: 'agent', createdAt: '2026-09-12T09:00:00Z' }],
    isLoading: false,
    isError: false,
  }),
  useRunActivities: () => ({
    data: [{ id: 'activity-1', kind: 'tool_finished', status: 'completed', actorId: 'analyst', name: '网络检索', occurredAt: '2026-09-12T08:50:00Z' }],
    isLoading: false,
    isError: false,
  }),
  useRunEvents: () => ({
    data: [{ event: { id: 'evt-1', type: 'agenthub.state.changed.v1', source: 'tool:review', actor: { type: 'agent', id: 'analyst' }, occurredAt: '2026-09-12T08:55:00Z', recordedAt: '2026-09-12T08:55:00Z' }, delivery: { status: 'delivered' } }],
    isLoading: false, isError: false,
  }),
  useRunToolResults: () => ({
    data: [{ id: 'tool-result-1', toolName: '报告审核', actorId: 'reviewer', status: 'applied', stateProposals: [{ stateId: 21, reason: '通过' }], committedChangeIds: ['change-31'], createdAt: '2026-09-12T08:54:00Z' }],
    isLoading: false, isError: false,
  }),
  useRunAgentMessages: () => ({ data: collaborationMessages, isLoading: false, isError: false }),
  useCreateRun: () => ({ mutate: createMutate, isPending: false }),
  useEnabledCapabilityPackages: () => ({ data: [{ id: 41, namespace: 'io.zerone.research', name: 'research-team', displayName: '研究协作', version: '1.0.0', contentHash: 'abc', enabled: true }], isLoading: false }),
  useTransitionRun: () => ({ mutate: transitionMutate, isPending: false }),
  useAddRunAgent: () => ({ mutate: addAgentMutate, isPending: false }),
  useComposeRunPrompt: () => ({ mutate: composePromptMutate, isPending: false }),
}))

function renderPage(path = '/runs/run-market') {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/runs/:runId?" element={<RunCenterPage />} />
          <Route path="/agents/:name/chat" element={<div>Agent Run Chat</div>} />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>,
  )
}

describe('RunCenterPage', () => {
  beforeEach(() => { vi.clearAllMocks(); detailStatus = 'running'; collaborationMessages = [] })

  it('shows a product-readable run archive', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: '运行中心' })).toBeInTheDocument()
    expect(screen.getByText('林经理')).toBeInTheDocument()
    expect(screen.getByText('研究分析师')).toBeInTheDocument()
    expect(screen.getByText('完成竞品分析')).toBeInTheDocument()
    expect(screen.getByText('65')).toBeInTheDocument()
  })

  it('shows real execution, tool decisions and causality in product language', () => {
    renderPage()
    expect(screen.getByText(/工具完成 · 网络检索/)).toBeInTheDocument()
    expect(screen.getByText('已生效')).toBeInTheDocument()
    expect(screen.getByText(/因果链起点/)).toBeInTheDocument()
    expect(screen.getByText(/H3 在原有运行档案/)).toBeInTheDocument()
  })

  it('opens participant chat inside the selected run', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /进入本次对话/ }))
    expect(screen.getByText('Agent Run Chat')).toBeInTheDocument()
  })

  it('explains an Agent judgment context in product language', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /查看判断背景/ }))
    expect(screen.getByText('研究分析师 的判断背景')).toBeInTheDocument()
    expect(screen.getByText(/2 个判断依据/)).toBeInTheDocument()
    expect(screen.getByText(/人格和上下文不会赋予额外权限/)).toBeInTheDocument()
    expect(screen.getByText('职责')).toBeInTheDocument()
  })

  it('guides a product manager through a real multi-hop acceptance test without JSON', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: 'Agent 协作链' })).toBeInTheDocument()
    expect(screen.getByText('1. 配置传递方向')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /从 研究分析师 发起测试/ })).toBeInTheDocument()
    expect(screen.queryByText(/JSON/)).not.toBeInTheDocument()
  })

  it('shows A to B to C, return, queue timing and budget guard in product language', () => {
    collaborationMessages = [
      { id: 'm1', relationId: 1, runId: 'run-market', conversationId: 'c1', rootMessageId: 'm1', hop: 1, maxHops: 3, eventBudget: 3, eventCount: 1, tokenBudget: 1000, tokensUsed: 100, visitedAgentIds: [1, 2], scope: 'project', sourceAgent: 'A', targetAgent: 'B', action: 'handoff', deliveryPolicy: 'async', contextPolicy: 'summary_only', status: 'completed', createdAt: '2026-09-12T08:00:00Z', startedAt: '2026-09-12T08:00:02Z', completedAt: '2026-09-12T08:00:05Z' },
      { id: 'm2', relationId: 2, runId: 'run-market', conversationId: 'c1', rootMessageId: 'm1', parentMessageId: 'm1', hop: 2, maxHops: 3, eventBudget: 3, eventCount: 2, tokenBudget: 1000, tokensUsed: 300, visitedAgentIds: [1, 2, 3], scope: 'project', sourceAgent: 'B', targetAgent: 'C', action: 'assign', deliveryPolicy: 'async', contextPolicy: 'summary_only', status: 'completed', createdAt: '2026-09-12T08:00:05Z', startedAt: '2026-09-12T08:00:06Z', completedAt: '2026-09-12T08:00:10Z' },
      { id: 'm3', relationId: 3, runId: 'run-market', conversationId: 'c1', rootMessageId: 'm1', parentMessageId: 'm2', hop: 3, maxHops: 3, eventBudget: 3, eventCount: 4, tokenBudget: 1000, tokensUsed: 500, visitedAgentIds: [1, 2, 3, 1], scope: 'project', sourceAgent: 'C', targetAgent: 'A', action: 'report', deliveryPolicy: 'async', contextPolicy: 'summary_only', status: 'guarded', guardReason: 'event_budget_exceeded', createdAt: '2026-09-12T08:00:10Z' },
    ]
    renderPage()
    expect(screen.getAllByText('A').length).toBeGreaterThan(0)
    expect(screen.getAllByText('B').length).toBeGreaterThan(0)
    expect(screen.getByText(/回报发起人/)).toBeInTheDocument()
    expect(screen.getByText(/排队 2.0 秒/)).toBeInTheDocument()
    expect(screen.getByText(/消息额度已用完/)).toBeInTheDocument()
    expect(screen.getByText('已拦截')).toBeInTheDocument()
  })

  it('filters runs by task name', () => {
    renderPage()
    fireEvent.change(screen.getByPlaceholderText('搜索任务'), { target: { value: '风险' } })
    expect(screen.queryByRole('button', { name: /市场研究/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /风险审查/ })).toBeInTheDocument()
  })

  it('creates a run from the empty-site entry point', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /新建运行/ }))
    fireEvent.change(screen.getByLabelText('运行名称'), { target: { value: '新市场调研' } })
    fireEvent.change(screen.getByLabelText('运行说明'), { target: { value: '评估需求' } })
    fireEvent.mouseDown(screen.getByLabelText('本次使用的能力'))
    fireEvent.click(await screen.findByText('研究协作 · 1.0.0'))
    fireEvent.click(screen.getByRole('button', { name: '创建运行' }))
    await waitFor(() => expect(createMutate).toHaveBeenCalledWith(
        { name: '新市场调研', description: '评估需求', capabilityBindings: [{ namespace: 'io.zerone.research', packageName: 'research-team', version: '1.0.0' }] },
        expect.objectContaining({ onSuccess: expect.any(Function) }),
      ))
  })

  it('moves a running run to paused or completed', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '暂 停' }))
    expect(transitionMutate).toHaveBeenCalledWith({ id: 'run-market', status: 'paused' })
    fireEvent.click(screen.getByRole('button', { name: '完成运行' }))
    expect(transitionMutate).toHaveBeenCalledWith({ id: 'run-market', status: 'completed' })
  })

  it('adds an existing Agent before the run starts', async () => {
    detailStatus = 'draft'
    renderPage()
    fireEvent.mouseDown(screen.getByLabelText('选择 Agent'))
    fireEvent.click(await screen.findByText('风险复核员'))
    fireEvent.change(screen.getByLabelText('参与角色'), { target: { value: '复核者' } })
    fireEvent.click(screen.getByRole('button', { name: /添加/ }))
    expect(addAgentMutate).toHaveBeenCalledWith(
      { id: 'run-market', agentId: 9, role: '复核者' },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    )
  })
})
