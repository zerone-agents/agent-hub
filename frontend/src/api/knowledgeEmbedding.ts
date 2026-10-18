import apiClient, { unwrapResponse } from './client'
import { ownedRequestConfig, type RequestOwner } from './requestOwnership'

export interface EmbeddingCheck {
  code: 0 | 10
  compatible: boolean
  summary: {
    kb_id: string
    model: string
    sampled: number
    valid: number
    avg_cos_sim: number
    min_cos_sim: number
    max_cos_sim: number
    match_mode: string
  }
  results: { chunk_id: string; reason?: string; cos_sim?: number; vector_dim?: number }[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export async function checkDatasetEmbedding(datasetId: string, embdId: string, owner: RequestOwner): Promise<EmbeddingCheck> {
  owner.assertCurrent()
  const response = await apiClient.post(`/api/v1/admin/knowledge/datasets/${encodeURIComponent(datasetId)}/embedding/check`,
    { embd_id: embdId }, { timeout: 65000, ...ownedRequestConfig(owner) })
  owner.assertCurrent()
  const value = unwrapResponse<unknown>(response)
  if (!isRecord(value) || (value.code !== 0 && value.code !== 10) || value.compatible !== (value.code === 0)
    || !isRecord(value.summary) || value.summary.kb_id !== datasetId || value.summary.model !== embdId || !Array.isArray(value.results)) throw new Error('Invalid embedding check response')
  const summary = value.summary
  if (!['sampled', 'valid', 'avg_cos_sim', 'min_cos_sim', 'max_cos_sim'].every(key =>
    typeof summary[key] === 'number' && Number.isFinite(summary[key]))) throw new Error('Invalid embedding check response')
  return value as unknown as EmbeddingCheck
}
