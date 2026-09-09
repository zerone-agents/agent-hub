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

export type RelationStance =
  | 'allied'
  | 'friendly'
  | 'neutral'
  | 'wary'
  | 'competitive'
  | 'hostile'

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

export interface AgentRelation {
  id: number
  scope: string
  sourceAgentId: number
  sourceAgentName: string
  targetAgentId: number
  targetAgentName: string
  relationType: RelationType
  stance: RelationStance
  allowedActions: RelationAction[]
  contextPolicy: ContextPolicy
  deliveryPolicy: DeliveryPolicy
  constraint: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface AgentRelationCreatePayload {
  sourceAgentId: number
  targetAgentId: number
  scope: string
  relationType: RelationType
  stance: RelationStance
  allowedActions: RelationAction[]
  contextPolicy: ContextPolicy
  deliveryPolicy: DeliveryPolicy
  constraint: string
  enabled: boolean
  bidirectional: boolean
}

export type AgentRelationUpdatePayload = Omit<
  AgentRelationCreatePayload,
  'sourceAgentId' | 'targetAgentId' | 'bidirectional'
>

export const agentRelationApi = {
  list: () => apiClient.get('/api/v1/admin/agent-relations'),
  create: (data: AgentRelationCreatePayload) =>
    apiClient.post('/api/v1/admin/agent-relations', data),
  update: (id: number, data: AgentRelationUpdatePayload) =>
    apiClient.put(`/api/v1/admin/agent-relations/${id}`, data),
  delete: (id: number) => apiClient.delete(`/api/v1/admin/agent-relations/${id}`)
}
