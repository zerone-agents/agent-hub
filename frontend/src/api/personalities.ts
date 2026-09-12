import apiClient from './client'
import type { BehaviorProfile } from './agents'

export interface PersonalityVersion {
  id: number
  templateId: number
  version: number
  prompt: string
  /** @deprecated Read-only compatibility data from an historical version. */
  behaviorProfile?: BehaviorProfile | null
  changeNote: string
  createdAt: string
}

export interface Personality {
  id: number
  name: string
  title: string
  description: string
  prompt: string
  /** @deprecated Read-only compatibility data from an historical template. */
  behaviorProfile?: BehaviorProfile | null
  currentVersion: number
  enabled: boolean
  isBuiltin: boolean
  usageCount: number
  createdAt: string
  updatedAt: string
  versions?: PersonalityVersion[]
}

export interface PersonalityCreatePayload {
  name: string
  title: string
  description?: string
  prompt: string
  enabled?: boolean
}

export interface PersonalityUpdatePayload {
  title?: string
  description?: string
  prompt?: string
  enabled?: boolean
  changeNote?: string
}

export const personalityApi = {
  list: () => apiClient.get('/api/v1/admin/personalities'),
  get: (name: string) =>
    apiClient.get(`/api/v1/admin/personalities/${encodeURIComponent(name)}`),
  create: (data: PersonalityCreatePayload) =>
    apiClient.post('/api/v1/admin/personalities', data),
  update: (name: string, data: PersonalityUpdatePayload) =>
    apiClient.put(
      `/api/v1/admin/personalities/${encodeURIComponent(name)}`,
      data,
    ),
  delete: (name: string) =>
    apiClient.delete(`/api/v1/admin/personalities/${encodeURIComponent(name)}`),
}
