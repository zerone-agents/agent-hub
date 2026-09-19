import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router'
import GroupWorkspacePage from './GroupWorkspacePage'

vi.mock('@/hooks/useCanWrite', () => ({ useCanWrite: () => true }))
vi.mock('@/queries/useAgents', () => ({ useAgents: () => ({ data: [{ id: 1, name: 'a', config: { title: { zh: '财务 Agent' } } }] }) }))
vi.mock('@/queries/useGroups', () => ({
  useGroups: () => ({ data: [{ id: 'g-1', name: '资产处置组', description: '共同评估资产方案', visibility: 'private', memberCount: 1, channelCount: 1 }], isLoading: false }),
  useGroup: () => ({ data: { id: 'g-1', name: '资产处置组', description: '共同评估资产方案', visibility: 'private' } }),
  useGroupMembers: () => ({ data: [{ id: 1, agentId: 1, agentName: '财务 Agent', role: 'leader' }] }),
  useGroupChannels: () => ({ data: [{ id: 'c-1', groupId: 'g-1', name: '风险评估', topic: '评估资产处置风险', visibility: 'group' }] }),
  useChannelSubscriptions: () => ({ data: [{ agentId: 1, mode: 'all' }] }),
  useChannelMessages: () => ({ data: [] }),
  useChannelSessions: () => ({ data: [{ id: 's-1', channelId: 'c-1', agenda: '是否暂停出售', status: 'active' }] }),
  useGroupAudit: () => ({ data: [{ id: 'a-1', action: 'member_added', actorName: '管理员', description: '财务 Agent 加入群组', createdAt: '2026-09-13T00:00:00Z' }] }),
  useGroupAction: () => ({ mutate: vi.fn(), mutateAsync: vi.fn() }),
}))

describe('GroupWorkspacePage', () => {
  it('用通俗入口展示群组、频道、会话和真实留痕', () => {
    render(<MemoryRouter><GroupWorkspacePage /></MemoryRouter>)
    expect(screen.getAllByText('资产处置组').length).toBeGreaterThan(0)
    expect(screen.getByText(/让多个 Agent 在明确的成员范围和频道里共同工作/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /频道/ }))
    expect(screen.getByText('# 风险评估')).toBeInTheDocument()
    expect(screen.getByText('接收全部消息')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: '会话房间' }))
    expect(screen.getByText('会话房间不是审批会议')).toBeInTheDocument()
    expect(screen.getByText('是否暂停出售')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: '完整留痕' }))
    expect(screen.getByText('财务 Agent 加入群组')).toBeInTheDocument()
  })
})
