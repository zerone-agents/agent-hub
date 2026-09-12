import apiClient from './client'

export type RunStatus = 'draft' | 'running' | 'paused' | 'completed' | 'archived'

export interface RunAgent {
  id: number
  agentId: number
  agentNameSnapshot: string
  role: string
}

export interface RunCapabilityBinding {
  id?: number
  namespace: string
  packageName: string
  version: string
  contentHash?: string
  manifestYAML?: string
  snapshot?: Record<string, unknown>
}

export interface CapabilityPackage {
  id: number
  namespace: string
  name: string
  version: string
  displayName?: string
  contentHash: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface RunCapabilitySelection {
  namespace: string
  packageName: string
  version: string
}

export interface RunState {
  id: number
  namespace: string
  schemaName: string
  schemaVersion: string
  subjectType: string
  subjectId: string
  revision: number
  data: Record<string, unknown>
  createdAt?: string
  updatedAt?: string
}

export interface RunStateChange {
  id: string
  runStateId?: number
  revisionBefore: number
  revisionAfter: number
  before: Record<string, unknown>
  after: Record<string, unknown>
  reason?: string
  source?: string
  createdAt: string
}

export interface RunActivity {
  id: string
  kind: 'participant' | 'started' | 'tool_started' | 'tool_finished' | 'completed' | 'failed' | string
  status?: string
  actorType?: string
  actorId?: string
  stepId?: string
  name?: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  error?: string
  occurredAt: string
}

export interface Run {
  id: string
  name: string
  description?: string
  status: RunStatus
  metadata?: Record<string, unknown>
  createdBy?: string
  startedAt?: string
  pausedAt?: string
  completedAt?: string
  archivedAt?: string
  createdAt: string
  updatedAt: string
  agents?: RunAgent[]
  capabilityBindings?: RunCapabilitySelection[]
}

export interface RunDetail {
  run: Run
  states: RunState[]
}

export interface CreateRunInput {
  name: string
  description?: string
  metadata?: Record<string, unknown>
  capabilityBindings?: RunCapabilityBinding[]
}

export const runApi = {
  list: () => apiClient.get('/api/v1/admin/runs'),
  create: (input: CreateRunInput) => apiClient.post('/api/v1/admin/runs', input),
  get: (id: string) => apiClient.get(`/api/v1/admin/runs/${id}`),
  listStateChanges: (id: string) =>
    apiClient.get(`/api/v1/admin/runs/${id}/state-changes`),
  listActivities: (id: string) =>
    apiClient.get(`/api/v1/admin/runs/${id}/activities`),
  transition: (id: string, status: RunStatus) =>
    apiClient.post(`/api/v1/admin/runs/${id}/transitions`, { status }),
  addAgent: (id: string, agentId: number, role: string) =>
    apiClient.post(`/api/v1/admin/runs/${id}/agents`, { agentId, role }),
  listCapabilityPackages: () =>
    apiClient.get('/api/v1/admin/capability-packages', { params: { enabled: 'true' } }),
}
