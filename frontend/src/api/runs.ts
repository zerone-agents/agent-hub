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

export interface PromptProvenance {
  stage: string
  label: string
  sourceType: string
  sourceId: string
  sourceVersion: string
  contentHash: string
  tokenEstimate: number
}

export interface PromptSnapshot {
  id: string
  runId: string
  runAgentId: number
  agentId: number
  renderedText: string
  renderedHash: string
  userInputHash?: string
  deliveryHash?: string
  deliveryStatus: 'preview' | 'prepared' | 'delivered' | 'failed' | string
  provenance: PromptProvenance[]
  createdAt: string
}

export interface RunEventItem {
  event: { id: string; type: string; source: string; actor?: { type: string; id: string }; subject?: { type: string; id: string }; causationId?: string; rootEventId?: string; occurredAt: string; recordedAt: string }
  delivery?: { status: 'pending' | 'processing' | 'retry' | 'delivered' | 'cancelled' | 'dead_letter' | string; attempts?: number; lastError?: string }
}

export interface ToolResultRecord {
  id: string
  toolName: string
  actorId?: string
  status: 'accepted' | 'rejected' | 'applied' | string
  decisionReason?: string
  result?: Record<string, unknown>
  stateProposals?: { stateId: number; reason?: string }[]
  committedChangeIds?: string[]
  createdAt: string
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
  composePrompt: (id: string, agentId: number) =>
    apiClient.post(`/api/v1/admin/runs/${id}/agents/${agentId}/prompt`),
  latestPrompt: (id: string, agentId: number) =>
    apiClient.get(`/api/v1/admin/runs/${id}/agents/${agentId}/prompt`),
  listEvents: (id: string) => apiClient.get(`/api/v1/admin/runs/${id}/events`),
  listToolResults: (id: string) => apiClient.get(`/api/v1/admin/runs/${id}/tool-results`),
  listCapabilityPackages: () =>
    apiClient.get('/api/v1/admin/capability-packages', { params: { enabled: 'true' } }),
}
