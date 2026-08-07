import apiClient from './client'

export interface Scene {
  id: number
  name: string
  agent: string
  title: string
  titleEn: string
  prompt: string
  promptEn: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface SceneCreatePayload {
  name: string
  agentId: number
  title: string
  titleEn: string
  prompt: string
  promptEn: string
}

export interface SceneUpdatePayload {
  agentId?: number
  title?: string
  titleEn?: string
  prompt?: string
  promptEn?: string
  enabled?: boolean
}

export const sceneApi = {
  list: (agentId?: number) => {
    const params = agentId ? `?agentId=${encodeURIComponent(agentId)}` : ''
    return apiClient.get(`/api/v1/scenes${params}`)
  },
  get: (name: string) => apiClient.get(`/api/v1/scenes/${name}`),
  adminList: () => apiClient.get('/api/v1/admin/scenes'),
  create: (data: SceneCreatePayload) => apiClient.post('/api/v1/admin/scenes', data),
  update: (name: string, data: SceneUpdatePayload) =>
    apiClient.put(`/api/v1/admin/scenes/${name}`, data),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/scenes/${name}`)
}
