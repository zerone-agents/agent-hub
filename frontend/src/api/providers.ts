import apiClient from './client'

export interface CatalogModel {
  selectionId?: string
  modelId: string
  displayName: string
  contextWindow?: number
  modelType: 'llm' | 'ocr' | 'embedding' | 'vlm'
  efforts?: string[]
  status?: '0' | '1'
  // AIGC code auto-assigned by the backend (Task 1). Read-only in the UI;
  // surfaced via the toCatalogModels mapper on provider API responses.
  aigcCode?: string
}

export interface PresetField {
  key: string
  label: string
  labelEn: string
  type: 'text' | 'password' | 'select'
  required?: boolean
  secret?: boolean
}

// AttrValue is the weakly-typed attribute stored in the EAV table.
export interface AttrValue {
  type: 'string' | 'bool' | 'int'
  value: string
}

// AttrRule describes one provider-specific attribute for a protocol.
// Drives both backend validation and the frontend dynamic form.
export interface AttrRule {
  key: string
  type: 'string' | 'bool' | 'int'
  required: boolean
  enum?: string[]
  default?: string
  label: string
  labelEn: string
}

export type AttrRules = Record<string, AttrRule[]>

export interface Provider {
  id: number
  key: string
  name: string
  description: string
  descriptionEn: string
  protocol: 'anthropic' | 'openai' | 'mineru' | 'paddleocr'
  // type field removed — derive from defaultModels if needed
  authStyle: 'api_key' | 'auth_token' | 'no_auth'
  baseUrl: string
  defaultModels: CatalogModel[]
  fields: PresetField[]
  iconKey: string
  builtin: boolean
  attributes: Record<string, AttrValue>
  lockedApiKey: string
  createdAt: string
  updatedAt: string
}

export interface ModelInput {
  modelId: string
  displayName?: string
  modelType: 'llm' | 'ocr' | 'embedding' | 'vlm'
  contextWindow?: number
  efforts?: string[]
}

export interface ModelPatch {
  displayName?: string
  modelType?: 'llm' | 'ocr' | 'embedding' | 'vlm'
  contextWindow?: number
  status?: '0' | '1'
  efforts?: string[]
}

export interface ProbeResult {
  success: boolean
  latencyMs: number
  statusCode: number
  error?: string
}

export interface ProbeConfig {
  baseUrl: string
  apiKey: string
  protocol: string
  authStyle: string
  models: CatalogModel[]
}

export interface SyncMultiRAGRequest {
  verifyOnly?: boolean
  modelIds?: string[]
}

export const providerApi = {
  list: (type?: 'llm' | 'ocr' | 'embedding' | 'vlm') =>
    apiClient.get('/api/v1/admin/providers', { params: type ? { type } : {} }),
  get: (id: number) => apiClient.get(`/api/v1/admin/providers/${id}`),
  create: (data: Partial<Provider>) => apiClient.post('/api/v1/admin/providers', data),
  update: (id: number, data: Partial<Provider>) =>
    apiClient.put(`/api/v1/admin/providers/${id}`, data),
  delete: (id: number) => apiClient.delete(`/api/v1/admin/providers/${id}`),
  probe: (id: number, payload?: { apiKey?: string; baseUrl?: string; models?: CatalogModel[] }) =>
    apiClient.post(`/api/v1/admin/providers/${id}/probe`, payload ?? {}),
  probeConfig: (config: ProbeConfig) =>
    apiClient.post('/api/v1/admin/providers/probe', config),
  attrRules: (protocol?: string) =>
    apiClient.get('/api/v1/admin/providers/attr-rules', { params: protocol ? { protocol } : {} }),
  addModel: (providerId: number, model: ModelInput) =>
    apiClient.post(`/api/v1/admin/providers/${providerId}/models`, model),
  updateModel: (providerId: number, selectionId: string, patch: ModelPatch) =>
    apiClient.patch(`/api/v1/admin/providers/${providerId}/models/${selectionId}`, patch),
  deleteModel: (providerId: number, selectionId: string) =>
    apiClient.delete(`/api/v1/admin/providers/${providerId}/models/${selectionId}`),
  syncMultiRAG: (id: number, body: SyncMultiRAGRequest = {}) =>
    apiClient.post(`/api/v1/admin/providers/${id}/sync-multirag`, body),
}
