// H7.0 扩展注册中心的管理 API 客户端类型与请求封装。
import apiClient from './client'

export interface ExtensionPermission {
  permission: string
  scope: string
  actions: string[]
}

export interface ExtensionListItem {
  id: number
  name: string
  displayName: string
  description: string
  icon: string
  source: string
  status: string
  createdAt: string
  updatedAt: string
  versionCount: number
  latestVersion: string
}

export interface ExtensionListResult {
  items: ExtensionListItem[]
  total: number
  page: number
  pageSize: number
}

export interface ExtensionVersionManifestSummary {
  displayName: string
  description: string
  slots?: string[]
  stateSchemaCount: number
  eventCount: number
  toolCount: number
  relationCount: number
  promptInjectionCount: number
}

export interface ExtensionVersionSummary {
  id: number
  extensionId: number
  version: string
  contentHash: string
  changelog: string
  createdBy: string
  createdAt: string
  manifestSummary: ExtensionVersionManifestSummary
  permissions: ExtensionPermission[]
}

export interface ExtensionDetail {
  id: number
  name: string
  displayName: string
  description: string
  icon: string
  source: string
  status: string
  createdAt: string
  updatedAt: string
  versions: ExtensionVersionSummary[]
}

export interface ExtensionVersionDetail {
  id: number
  extensionId: number
  version: string
  manifest: string
  contentHash: string
  changelog: string
  createdBy: string
  createdAt: string
}

export interface RegisterExtensionResult {
  extension: {
    id: number
    name: string
    displayName: string
    source: string
    status: string
  }
  version: {
    id: number
    version: string
    contentHash: string
    changelog: string
  }
  alreadyExisted: boolean
}

export const extensionRegistryApi = {
  register: (manifest: string, source?: string, changelog?: string) =>
    apiClient.post('/api/v1/admin/extensions', manifest, {
      headers: { 'Content-Type': 'application/json' },
      params: { source, changelog }
    }),
  list: (params: { status?: string; source?: string; page: number; pageSize: number }) =>
    apiClient.get('/api/v1/admin/extensions', { params }),
  get: (id: number) => apiClient.get(`/api/v1/admin/extensions/${id}`),
  getVersion: (id: number, version: string) =>
    apiClient.get(`/api/v1/admin/extensions/${id}/versions/${encodeURIComponent(version)}`)
}
