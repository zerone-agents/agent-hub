import apiClient from './client'
import type { ContextPolicy, DeliveryPolicy, RelationAction, RelationStance, RelationType } from './agent-relations'

export interface RelationTypeTemplate {
  id: number
  name: string
  title: string
  description: string
  baseType: RelationType
  directionPolicy: 'one_way' | 'bidirectional_allowed'
  /** @deprecated Legacy backend compatibility; not a connection setting. */
  defaultStance: RelationStance
  defaultAllowedActions: RelationAction[]
  defaultContextPolicy: ContextPolicy
  defaultDeliveryPolicy: DeliveryPolicy
  defaultConstraint: string
  lineColor: string
  lineStyle: 'solid' | 'dashed' | 'dotted'
  currentVersion: number
  enabled: boolean
  isBuiltin: boolean
  usageCount: number
}

export type RelationTypePayload = Omit<RelationTypeTemplate, 'id' | 'currentVersion' | 'isBuiltin' | 'usageCount' | 'defaultStance'> & {
  /** @deprecated Accepted only by older servers; current clients omit it. */
  defaultStance?: RelationStance
}

export const relationTypeApi = {
  list: () => apiClient.get('/api/v1/admin/relation-types'),
  create: (data: RelationTypePayload) => apiClient.post('/api/v1/admin/relation-types', data),
  update: (name: string, data: RelationTypePayload) => apiClient.put(`/api/v1/admin/relation-types/${name}`, data),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/relation-types/${name}`),
}
