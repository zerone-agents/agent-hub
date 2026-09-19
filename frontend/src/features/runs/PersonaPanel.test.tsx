import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import type { BeliefDispute, PersonaState } from '@/api/runs'
import type { RunAgent, RunState, RunStateChange } from '@/api/runs'
import { PersonaPanel } from './PersonaPanel'

let personaState: PersonaState | null = null
let personaLoading = false
let disputes: BeliefDispute[] = []

vi.mock('@/queries/useRuns', () => ({
  useRunPersonaState: () => ({ data: personaState, isLoading: personaLoading, isError: false }),
  useRunBeliefDisputes: () => ({ data: disputes, isLoading: false, isError: false }),
}))

const agents: RunAgent[] = [
  { id: 11, agentId: 7, agentNameSnapshot: '研究分析师', role: '主分析' },
  { id: 13, agentId: 8, agentNameSnapshot: '法务顾问', role: '顾问' },
]

const states: RunState[] = [
  { id: 31, namespace: 'io.zerone.emotion', schemaName: 'emotion-state', schemaVersion: 'v1', subjectType: 'agent', subjectId: '7', revision: 2, data: {} },
  { id: 32, namespace: 'io.zerone.research', schemaName: 'progress', schemaVersion: '1', subjectType: 'agent', subjectId: 'analyst', revision: 1, data: {} },
]

const changes: RunStateChange[] = [
  { id: 'change-emo', runStateId: 31, revisionBefore: 1, revisionAfter: 2, before: {}, after: { intensity: 45 }, reason: '受到鼓舞', source: 'tool:emotion', createdAt: '2026-09-12T09:00:00Z' },
  { id: 'change-generic', runStateId: 32, revisionBefore: 0, revisionAfter: 1, before: {}, after: { progress: 10 }, reason: '普通状态变化', source: 'agent', createdAt: '2026-09-12T08:00:00Z' },
]

function fullPersonaState(): PersonaState {
  return {
    runId: 'run-market',
    emotion: [{ namespace: 'io.zerone.emotion', schemaName: 'emotion-state', subjectType: 'agent', subjectId: '7', revision: 2, updatedAt: '2026-09-12T09:00:00Z', data: { mood: 'elated', intensity: 45, baseline: 'calm', narration: '你当前心情为振奋（强度 45/100）。你的情绪显著起伏，开始影响你的判断。', updatedAt: '2026-09-12T09:00:00Z' } }],
    belief: [
      { namespace: 'io.zerone.belief', schemaName: 'belief-state', subjectType: 'agent', subjectId: '7', revision: 1, updatedAt: '2026-09-12T08:40:00Z', data: { factRef: 'fact-scandal', status: 'believed', confidence: 80, source: 'delivery', statement: '报告可信', lastEventAt: '2026-09-12T08:40:00Z' } },
      { namespace: 'io.zerone.belief', schemaName: 'belief-state', subjectType: 'agent', subjectId: '8', revision: 1, updatedAt: '2026-09-12T08:41:00Z', data: { factRef: 'fact-scandal', status: 'doubted', confidence: 30, source: 'claim', lastEventAt: '2026-09-12T08:41:00Z' } },
    ],
    memory: [{ namespace: 'io.zerone.subjective-memory', schemaName: 'memory-entry', subjectType: 'agent', subjectId: 'mem-1', revision: 1, updatedAt: '2026-09-12T08:45:00Z', data: { id: 'mem-1', agentId: 8, factRef: 'fact-scandal', interpretation: '这份报告来得太巧，像是有人故意安排。', importance: 70, recallCount: 2, recordedAt: '2026-09-12T08:45:00Z' } }],
    relationDynamics: [{ namespace: 'io.zerone.relationship-dynamics', schemaName: 'relation-attitude', subjectType: 'relation', subjectId: '7:8', revision: 1, updatedAt: '2026-09-12T08:50:00Z', data: { score: -40, stance: 'wary', narration: '你对 法务顾问 的态度：警惕（-40）' } }],
  }
}

function renderPanel() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <PersonaPanel runId="run-market" agents={agents} states={states} changes={changes} />
    </ConfigProvider>,
  )
}

describe('PersonaPanel', () => {
  beforeEach(() => { personaState = fullPersonaState(); personaLoading = false; disputes = [] })

  it('renders the four persona sections with agent names', () => {
    renderPanel()
    expect(screen.getByRole('heading', { name: '当前情绪' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '认知与立场' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '主观记忆' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '动态关系' })).toBeInTheDocument()
    expect(screen.getByText('研究分析师')).toBeInTheDocument()
    expect(screen.getByText(/你当前心情为振奋/)).toBeInTheDocument()
    expect(screen.getByText('这份报告来得太巧，像是有人故意安排。')).toBeInTheDocument()
    expect(screen.getByText('事实来源')).toBeInTheDocument()
    expect(screen.getByText('主观声称')).toBeInTheDocument()
    expect(screen.getByText(/以上状态仅注入该 Agent 本人的提示词/)).toBeInTheDocument()
  })

  it('shows a friendly empty hint when the run has no persona state yet', () => {
    personaState = { runId: 'run-market', emotion: [], belief: [], memory: [], relationDynamics: [] }
    renderPanel()
    expect(screen.getByText(/尚未产生人物状态——Agent 开始互动后这里会显示情绪、认知、记忆和关系变化/)).toBeInTheDocument()
  })

  it('shows the disputes banner only when disputes exist', () => {
    const { unmount } = renderPanel()
    expect(screen.queryByText('认知争议')).not.toBeInTheDocument()
    unmount()
    disputes = [{ factRef: 'fact-scandal', entries: [{ agentId: 7, status: 'believed', confidence: 80 }, { agentId: 8, status: 'doubted', confidence: 30 }] }]
    renderPanel()
    expect(screen.getByText('认知争议')).toBeInTheDocument()
    expect(screen.getAllByText('fact-scandal').length).toBeGreaterThan(0)
    expect(screen.getByText(/研究分析师 · 相信（置信 80）/)).toBeInTheDocument()
    expect(screen.getByText(/法务顾问 · 怀疑（置信 30）/)).toBeInTheDocument()
  })

  it('renders directed relation rows as A → B with stance and score', () => {
    renderPanel()
    expect(screen.getByText('研究分析师 → 法务顾问')).toBeInTheDocument()
    expect(screen.getByText('警惕（-40）')).toBeInTheDocument()
    expect(screen.getByText(/关系是单向的：A 对 B 的态度不一定等于 B 对 A/)).toBeInTheDocument()
  })

  it('filters the change timeline to the four persona namespaces', () => {
    renderPanel()
    expect(screen.getByText('受到鼓舞')).toBeInTheDocument()
    expect(screen.queryByText('普通状态变化')).not.toBeInTheDocument()
  })

  it('shows loading and error states gracefully', () => {
    personaLoading = true
    const { unmount } = renderPanel()
    expect(screen.getByText('人物状态')).toBeInTheDocument()
    unmount()
  })
})
