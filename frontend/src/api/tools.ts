import apiClient from './client'

export interface Tool {
  id: number
  name: string
  title: string
  description: string
  isDefault: boolean
  createdAt: string
  updatedAt: string
}

export const toolApi = {
  list: () => apiClient.get('/api/v1/admin/tools'),
  get: (name: string) => apiClient.get(`/api/v1/admin/tools/${name}`),
  create: (data: Partial<Tool>) => apiClient.post('/api/v1/admin/tools', data),
  update: (name: string, data: Partial<Tool>) => apiClient.put(`/api/v1/admin/tools/${name}`, data),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/tools/${name}`)
}
