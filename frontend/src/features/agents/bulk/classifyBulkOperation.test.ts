import { describe, it, expect } from 'vitest'
import { classifyBulkOperation, classifyAllAgents } from './classifyBulkOperation'
import type { Agent, DeploymentStatus } from '@/api/agents'

function status(s: string): DeploymentStatus {
  return { status: s }
}

function success(s: string) {
  return { kind: 'success' as const, status: status(s) }
}

const precheckError = { kind: 'error' as const, error: new Error('network') }

describe('classifyBulkOperation — deploy', () => {
  it('not_found / archived → executable', () => {
    expect(classifyBulkOperation('deploy', success('not_found')).classification).toBe('executable')
    expect(classifyBulkOperation('deploy', success('archived')).classification).toBe('executable')
  })

  it('running → skipped with reason', () => {
    const r = classifyBulkOperation('deploy', success('running'))
    expect(r.classification).toBe('skipped')
    expect(r.reason).toContain('重新部署')
  })

  it('stopped / exited / error → skipped with reason', () => {
    for (const s of ['stopped', 'exited', 'error']) {
      const r = classifyBulkOperation('deploy', success(s))
      expect(r.classification, s).toBe('skipped')
      expect(r.reason, s).toContain('重新部署')
    }
  })

  it('transitioning states → skipped（issue #141 交互要求 #4：不重复提交）', () => {
    for (const s of ['created', 'restarting', 'paused']) {
      expect(classifyBulkOperation('deploy', success(s)).classification, s).toBe('skipped')
    }
  })
})

describe('classifyBulkOperation — redeploy (force=true)', () => {
  it('all container-exists states → executable', () => {
    for (const s of ['running', 'stopped', 'exited', 'error', 'created', 'restarting', 'paused']) {
      expect(classifyBulkOperation('redeploy', success(s)).classification, s).toBe('executable')
    }
  })

  it('not_found / archived → skipped 未部署', () => {
    for (const s of ['not_found', 'archived']) {
      const r = classifyBulkOperation('redeploy', success(s))
      expect(r.classification, s).toBe('skipped')
      expect(r.reason, s).toContain('未部署')
    }
  })
})

describe('classifyBulkOperation — stop', () => {
  it('running / created / restarting / paused → executable', () => {
    for (const s of ['running', 'created', 'restarting', 'paused']) {
      expect(classifyBulkOperation('stop', success(s)).classification, s).toBe('executable')
    }
  })

  it('not_found / archived → skipped；stopped / exited → skipped 已停止；error → skipped', () => {
    expect(classifyBulkOperation('stop', success('not_found')).classification).toBe('skipped')
    expect(classifyBulkOperation('stop', success('stopped')).reason).toContain('已停止')
    expect(classifyBulkOperation('stop', success('error')).reason).toContain('重新部署')
  })
})

describe('classifyBulkOperation — delete（blocked 判定与后端 409 完全对齐）', () => {
  it('not_found / archived → executable；其余一切状态（含 unknown 与未识别值）→ blocked', () => {
    expect(classifyBulkOperation('delete', success('not_found')).classification).toBe('executable')
    expect(classifyBulkOperation('delete', success('archived')).classification).toBe('executable')
    for (const s of ['running', 'stopped', 'exited', 'error', 'created', 'restarting', 'paused', 'unknown', 'whatever-new-state']) {
      const r = classifyBulkOperation('delete', success(s))
      expect(r.classification, s).toBe('blocked')
      expect(r.reason, s).toContain('活跃部署')
    }
  })
})

describe('classifyBulkOperation — unknown 与 precheckError 三态分离', () => {
  it('unknown / 未识别状态 × deploy|redeploy|stop → executable（幂等/失败可见）', () => {
    for (const op of ['deploy', 'redeploy', 'stop'] as const) {
      expect(classifyBulkOperation(op, success('unknown')).classification, op).toBe('executable')
      expect(classifyBulkOperation(op, success('brand-new-status')).classification, op).toBe('executable')
    }
  })

  it('precheckError × 任意操作 → executable（后端 fail-closed 兜底）', () => {
    for (const op of ['deploy', 'redeploy', 'stop', 'delete'] as const) {
      expect(classifyBulkOperation(op, precheckError).classification, op).toBe('executable')
    }
  })
})

describe('classifyAllAgents', () => {
  const agents: Agent[] = [
    { id: 1, name: 'a', config: {} },
    { id: 2, name: 'b', config: {} },
  ]

  it('分类每项并携带 agent 与 precheck；缺预检结果按 error 处理', () => {
    const prechecks = new Map([['a', success('not_found')]])
    const items = classifyAllAgents('deploy', agents, prechecks)
    expect(items).toHaveLength(2)
    expect(items[0]).toMatchObject({ classification: 'executable' })
    expect(items[0]?.agent.name).toBe('a')
    expect(items[1]?.classification).toBe('executable') // 缺失 → precheckError → executable
    expect(items[1]?.precheck.kind).toBe('error')
  })
})
