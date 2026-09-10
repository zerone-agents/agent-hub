import type { Agent, DeploymentStatus } from '@/api/agents'

/** 批量操作类型（issue #141 第一阶段 + 用户新增的停止）。 */
export type BulkOperation = 'deploy' | 'redeploy' | 'stop' | 'delete'

/** 操作中文标签（确认弹窗、进度 Modal 等共用，review 提示去重）。 */
export const BULK_OPERATION_LABEL: Record<BulkOperation, string> = {
  deploy: '部署',
  redeploy: '重新部署',
  stop: '停止',
  delete: '删除',
}

/**
 * 预检结果判别联合（spec §5）：
 * - success：GET /deploy 请求成功，status 是真实部署状态（可为 'unknown' 或未识别值）
 * - error：预检请求失败（网络/5xx），与真实 unknown 状态语义不同
 */
export type PrecheckResult =
  | { kind: 'success'; status: DeploymentStatus }
  | { kind: 'error'; error: unknown }

export type Classification = 'executable' | 'skipped' | 'blocked'

export interface ClassifiedItem {
  agent: Agent
  precheck: PrecheckResult
  classification: Classification
  reason?: string
}

const NO_CONTAINER = new Set(['not_found', 'archived'])
const HAS_CONTAINER = new Set(['running', 'stopped', 'exited', 'error', 'created', 'restarting', 'paused'])
const TRANSITIONING = new Set(['created', 'restarting', 'paused'])
const KNOWN_STATUSES = new Set([...NO_CONTAINER, ...HAS_CONTAINER, 'unknown'])

/**
 * 单项分类（spec §5 规则表）。删除的 blocked 判定必须与后端 Delete handler 的
 * 409 条件完全对齐：状态非空且不为 not_found/archived 即活跃（含 unknown 与
 * 未识别值，fail-closed）。deploy × 中间态 spec 未覆盖，按 issue #141 交互
 * 要求 #4「正在部署的 Agent 不应重复提交冲突操作」细化为 skipped。
 */
export function classifyBulkOperation(
  op: BulkOperation,
  precheck: PrecheckResult,
): { classification: Classification; reason?: string } {
  if (precheck.kind === 'error') return { classification: 'executable' }
  const status = precheck.status.status
  // unknown 与未识别的新状态值：真实部署状态，非预检失败。
  const knownUnknown = status === 'unknown' || !KNOWN_STATUSES.has(status)

  switch (op) {
    case 'deploy': {
      if (NO_CONTAINER.has(status)) return { classification: 'executable' }
      if (knownUnknown) return { classification: 'executable' } // 幂等安全
      if (status === 'running') return { classification: 'skipped', reason: '已部署，建议重新部署' }
      if (TRANSITIONING.has(status)) return { classification: 'skipped', reason: `容器状态转换中（${status}），稍后再试` }
      return { classification: 'skipped', reason: '容器已停止或出错，建议重新部署' } // stopped/exited/error
    }
    case 'redeploy': {
      if (HAS_CONTAINER.has(status)) return { classification: 'executable' }
      if (knownUnknown) return { classification: 'executable' } // force 重建，无论容器是否存在都达目标
      return { classification: 'skipped', reason: '未部署' } // not_found/archived
    }
    case 'stop': {
      if (status === 'running' || TRANSITIONING.has(status)) return { classification: 'executable' }
      if (knownUnknown) return { classification: 'executable' } // 失败可见
      if (NO_CONTAINER.has(status)) return { classification: 'skipped', reason: '未部署' }
      if (status === 'stopped' || status === 'exited') return { classification: 'skipped', reason: '已停止' }
      return { classification: 'skipped', reason: '部署出错，建议重新部署' } // error
    }
    case 'delete': {
      if (NO_CONTAINER.has(status)) return { classification: 'executable' }
      // 其余一切（含 unknown/未识别）→ blocked，与后端 409 对齐（fail-closed）
      return { classification: 'blocked', reason: '有活跃部署，需先删除部署' }
    }
  }
}

/** 批量分类：prechecks 缺失的项按预检失败（error）处理。 */
export function classifyAllAgents(
  op: BulkOperation,
  agents: Agent[],
  prechecks: ReadonlyMap<string, PrecheckResult>,
): ClassifiedItem[] {
  return agents.map((agent) => {
    const precheck = prechecks.get(agent.name) ?? { kind: 'error', error: new Error('预检结果缺失') }
    const { classification, reason } = classifyBulkOperation(op, precheck)
    return { agent, precheck, classification, reason }
  })
}
