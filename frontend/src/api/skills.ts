import apiClient from './client'

export interface Skill {
  id: number
  name: string
  type: string
  title: string
  titleEn: string
  description: string
  descriptionEn: string
  url: string
  fileHash: string
  fileSize: number
  createdAt: string
  updatedAt: string
}

export interface SkillUpdatePayload {
  title?: string
  titleEn?: string
  description?: string
  descriptionEn?: string
  file?: File
}

export const skillApi = {
  list: (type?: string) => {
    const params = type ? `?type=${encodeURIComponent(type)}` : ''
    return apiClient.get(`/api/v1/skills${params}`)
  },
  get: (name: string) => apiClient.get(`/api/v1/skills/${name}`),
  download: (name: string) => apiClient.get(`/api/v1/skills/${name}/download`),
  adminList: (type?: string) => {
    const params = type ? `?type=${encodeURIComponent(type)}` : ''
    return apiClient.get(`/api/v1/admin/skills${params}`)
  },
  create: (formData: FormData) =>
    apiClient.post('/api/v1/admin/skills', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    }),
  update: (name: string, data: SkillUpdatePayload) => {
    const formData = new FormData()
    if (data.title !== undefined) formData.append('title', data.title)
    if (data.titleEn !== undefined) formData.append('titleEn', data.titleEn)
    if (data.description !== undefined) formData.append('description', data.description)
    if (data.descriptionEn !== undefined) formData.append('descriptionEn', data.descriptionEn)
    if (data.file) formData.append('file', data.file)
    return apiClient.put(`/api/v1/admin/skills/${name}`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },
  getSkillMd: (name: string) =>
    apiClient.get(`/api/v1/admin/skills/${encodeURIComponent(name)}/skill-md`),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/skills/${name}`)
}
