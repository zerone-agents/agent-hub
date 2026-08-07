import apiClient from './client'

export interface AigcConfig {
  configured: boolean
  uscc?: string
  companyName?: string
  contentProducer?: string
  signingKeyConfigured?: boolean
}

export const aigcApi = {
  get: () => apiClient.get<{ success: boolean; data: AigcConfig }>('/api/v1/admin/aigc/config'),

  save: (uscc: string, companyName: string) =>
    apiClient.put<{ success: boolean; data: AigcConfig }>('/api/v1/admin/aigc/config', {
      uscc,
      companyName
    }),

  rotateKey: () =>
    apiClient.post<{ success: boolean; data: AigcConfig }>('/api/v1/admin/aigc/config/rotate-key'),

  clear: () => apiClient.delete<{ success: boolean; message: string }>('/api/v1/admin/aigc/config')
}
