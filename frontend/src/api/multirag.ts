import apiClient from './client'

export interface MultiRAGModel {
  name: string
  factory: string
  type: string
  status: '0' | '1'
  fullId: string
}

export const multiragApi = {
  // Candidates that already exist in MultiRAG's configured providers list.
  // type: 'embedding' | 'ocr' | (other MultiRAG types pass through).
  getModels: (type: 'embedding' | 'ocr') =>
    apiClient.get('/api/v1/admin/knowledge/multirag/models', { params: { type } }),
}
