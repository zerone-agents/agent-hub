import apiClient, { type ApiEnvelope } from './client'

export interface Tool {
  id: number
  name: string
  title: string
  description: string
  isDefault: boolean
  source: string // 'builtin' | 'custom'
  artifactStatus?: string // 'ready' | 'missing'（custom 才有）
  fileName?: string
  fileHash?: string
  fileSize?: number
  createdAt: string
  updatedAt: string
}

export interface CustomToolCreateInput {
  name: string
  title?: string
  description?: string
  file: File
}

export interface ToolDownloadResult {
  url: string
  expiresIn: number
}

const multipartHeaders = { 'Content-Type': 'multipart/form-data' }

export const toolApi = {
  list: () => apiClient.get('/api/v1/admin/tools'),
  get: (name: string) => apiClient.get(`/api/v1/admin/tools/${encodeURIComponent(name)}`),
  createCustom: (data: CustomToolCreateInput) => {
    const formData = new FormData()
    formData.append('name', data.name)
    if (data.title) formData.append('title', data.title)
    if (data.description) formData.append('description', data.description)
    formData.append('file', data.file)
    return apiClient.post('/api/v1/admin/tools', formData, { headers: multipartHeaders })
  },
  update: (name: string, data: { title?: string; description?: string }) =>
    apiClient.put(`/api/v1/admin/tools/${encodeURIComponent(name)}`, data),
  uploadFile: (name: string, file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    return apiClient.put(`/api/v1/admin/tools/${encodeURIComponent(name)}/file`, formData, {
      headers: multipartHeaders
    })
  },
  download: (name: string) =>
    apiClient.get<ApiEnvelope<ToolDownloadResult>>(
      `/api/v1/admin/tools/${encodeURIComponent(name)}/download`
    ),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/tools/${encodeURIComponent(name)}`)
}
