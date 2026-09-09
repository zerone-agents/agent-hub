import type {
  ContextPolicy,
  DeliveryPolicy,
  RelationAction,
  RelationStance,
  RelationType
} from '@/api/agent-relations'

export const RELATION_TYPES: { value: RelationType; label: string; description: string }[] = [
  { value: 'reports_to', label: '向其汇报', description: '发起方是下属，接收方是负责人' },
  { value: 'peer', label: '同事协作', description: '同级协商、并行或顺序交接任务' },
  { value: 'advisor', label: '顾问咨询', description: '向对方征询意见与风险判断' },
  { value: 'reviewer', label: '复核关系', description: '向对方提交结果并接受质疑' },
  { value: 'oversight', label: '监督关系', description: '对对方进行审计、质询与升级' },
  { value: 'representative', label: '代表关系', description: '允许对方代表发起方参与沟通' },
  { value: 'opponent', label: '对抗关系', description: '目标冲突，允许挑战但仍受规则约束' },
  { value: 'external', label: '外部关系', description: '组织边界外的有限协作' }
]

export const STANCES: { value: RelationStance; label: string; color: string }[] = [
  { value: 'allied', label: '同盟', color: 'green' },
  { value: 'friendly', label: '友好', color: 'cyan' },
  { value: 'neutral', label: '中立', color: 'default' },
  { value: 'wary', label: '戒备', color: 'gold' },
  { value: 'competitive', label: '竞争', color: 'orange' },
  { value: 'hostile', label: '敌对', color: 'red' }
]

export const ACTIONS: { value: RelationAction; label: string }[] = [
  { value: 'inform', label: '通知' },
  { value: 'consult', label: '协商' },
  { value: 'assign', label: '指派' },
  { value: 'report', label: '汇报' },
  { value: 'submit', label: '提交' },
  { value: 'review', label: '复核' },
  { value: 'challenge', label: '挑战' },
  { value: 'handoff', label: '交接' },
  { value: 'escalate', label: '升级' },
  { value: 'invite', label: '邀请' }
]

export const CONTEXT_POLICIES: { value: ContextPolicy; label: string }[] = [
  { value: 'none', label: '不共享上下文' },
  { value: 'summary_only', label: '仅共享摘要' },
  { value: 'shared_thread', label: '共享完整会话' }
]

export const DELIVERY_POLICIES: { value: DeliveryPolicy; label: string }[] = [
  { value: 'async', label: '异步收件箱' },
  { value: 'sync', label: '同步等待回复' }
]

export const DEFAULT_ACTIONS: Record<RelationType, RelationAction[]> = {
  reports_to: ['report', 'consult', 'escalate'],
  peer: ['inform', 'consult', 'submit', 'handoff'],
  advisor: ['consult', 'inform'],
  reviewer: ['submit', 'review', 'challenge'],
  oversight: ['report', 'review', 'challenge', 'escalate'],
  representative: ['inform', 'report', 'invite'],
  opponent: ['challenge', 'submit'],
  external: ['inform', 'consult', 'invite']
}

export function optionLabel<T extends string>(
  options: { value: T; label: string }[],
  value: T
): string {
  return options.find((option) => option.value === value)?.label ?? value
}
