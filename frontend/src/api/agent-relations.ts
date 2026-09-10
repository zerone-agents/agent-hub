import apiClient from './client'

export type RelationType =
  | 'reports_to'
  | 'peer'
  | 'advisor'
  | 'reviewer'
  | 'oversight'
  | 'representative'
  | 'opponent'
  | 'external'

export type RelationStance = 'allied' | 'friendly' | 'neutral' | 'wary' | 'competitive' | 'hostile'

export type RelationAction =
  | 'inform'
  | 'consult'
  | 'assign'
  | 'report'
  | 'submit'
  | 'review'
  | 'challenge'
  | 'handoff'
  | 'escalate'
  | 'invite'

export type ContextPolicy = 'none' | 'summary_only' | 'shared_thread'
export type DeliveryPolicy = 'sync' | 'async'
export type RelationEventVisibility = 'private' | 'participants' | 'public'
export type RelationEventType =
  | 'task_completed'
  | 'task_failed'
  | 'promise_kept'
  | 'promise_broken'
  | 'helped'
  | 'obstructed'
  | 'protected'
  | 'betrayed'
  | 'credit_shared'
  | 'credit_stolen'
  | 'public_praise'
  | 'public_humiliation'
  | 'truth_verified'
  | 'lied'
  | 'reconciled'
  | 'admin_stance_reset'

export type RecordableRelationEventType = Exclude<RelationEventType, 'admin_stance_reset'>

export interface AgentRelation {
  id: number
  scope: string
  sourceAgentId: number
  sourceAgentName: string
  targetAgentId: number
  targetAgentName: string
  relationType: RelationType
  relationTypeTemplateName?: string
  relationTypeTemplateVersion?: number
  stance: RelationStance
  relationshipScore: number
  lastChangedAt?: string
  allowedActions: RelationAction[]
  contextPolicy: ContextPolicy
  deliveryPolicy: DeliveryPolicy
  constraint: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface AgentRelationEvent {
  id: string
  relationId: number
  scope: string
  sourceAgentId: number
  targetAgentId: number
  eventType: RelationEventType
  severity: number
  delta: number
  scoreBefore: number
  scoreAfter: number
  stanceBefore: RelationStance
  stanceAfter: RelationStance
  reason: string
  visibility: RelationEventVisibility
  actorType: 'agent' | 'admin' | 'system'
  actorId?: string
  sourceKind: string
  sourceId?: string
  idempotencyKey: string
  ruleVersion: string
  occurredAt: string
  createdAt: string
}

export interface RecordAgentRelationEventPayload {
  eventType: RecordableRelationEventType
  severity: 1 | 2 | 3
  reason: string
  visibility: RelationEventVisibility
  sourceKind?: string
  sourceId?: string
  idempotencyKey?: string
}

export interface AgentRelationEventResult {
  relation: AgentRelation
  event: AgentRelationEvent
}

export interface AgentRelationCreatePayload {
  sourceAgentId: number
  targetAgentId: number
  scope: string
  relationType: RelationType
  relationTypeTemplateName?: string
  stance: RelationStance
  allowedActions: RelationAction[]
  contextPolicy: ContextPolicy
  deliveryPolicy: DeliveryPolicy
  constraint: string
  enabled: boolean
  bidirectional: boolean
}

export type AgentRelationUpdatePayload = Partial<
  Omit<AgentRelationCreatePayload, 'sourceAgentId' | 'targetAgentId' | 'bidirectional'>
>

export const agentRelationApi = {
  list: () => apiClient.get('/api/v1/admin/agent-relations'),
  create: (data: AgentRelationCreatePayload) => apiClient.post('/api/v1/admin/agent-relations', data),
  update: (id: number, data: AgentRelationUpdatePayload) => apiClient.put(`/api/v1/admin/agent-relations/${id}`, data),
  delete: (id: number) => apiClient.delete(`/api/v1/admin/agent-relations/${id}`),
  events: (id: number, limit = 30) =>
    apiClient.get(`/api/v1/admin/agent-relations/${id}/events`, {
      params: { limit },
    }),
  recordEvent: (id: number, data: RecordAgentRelationEventPayload) =>
    apiClient.post(`/api/v1/admin/agent-relations/${id}/events`, data),
}
