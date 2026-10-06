import apiClient from './client'
import { ownedRequestConfig, type RequestOwner } from './requestOwnership'

export type MultiRAGModelType = 'embedding' | 'ocr' | 'image2text' | 'rerank'

export interface MultiRAGModel {
  name: string
  factory: string
  type: string
  status: string
  fullId: string
}

export const multiragApi = {
  // Candidates that already exist in MultiRAG's configured providers list.
  getModels: (type: MultiRAGModelType, owner?: RequestOwner) =>
    apiClient.get('/api/v1/admin/knowledge/multirag/models', { params: { type }, ...ownedRequestConfig(owner) }),
}
