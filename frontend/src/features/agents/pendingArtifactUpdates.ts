import type { Agent } from '@/api/agents'

/**
 * 是否存在待更新工件（工具/技能任一非空）。
 * 「待更新」徽标（AgentCard）、待更新计数与「全选待更新」（AgentListPage）
 * 共用同一判定，避免语义漂移（review 提示去重）。
 */
export function hasPendingArtifactUpdates(a: Agent): boolean {
  return (a.pendingArtifactUpdates?.tools.length ?? 0) > 0 ||
    (a.pendingArtifactUpdates?.skills.length ?? 0) > 0
}
