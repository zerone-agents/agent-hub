// H7.3 模板库管理 API 客户端类型与请求封装。
import apiClient from './client'

export interface TemplateSpecSummary {
  agents: number
  personalityTemplates: number
  groups: number
  relations: number
  workflows: number
  stateSchemas: number
  extensionDeps: number
  sampleData: number
}

export interface TemplateListItem {
  id: number
  name: string
  displayName: string
  description: string
  category: string
  icon: string
  source: string
  createdAt: string
  updatedAt: string
  versionCount: number
  latestVersion: string
}

export interface TemplateListResult {
  items: TemplateListItem[]
  total: number
  page: number
  pageSize: number
}

export interface TemplateVersionItem {
  id: number
  templateId: number
  version: string
  contentHash: string
  createdBy: string
  createdAt: string
  summary: TemplateSpecSummary
}

export interface TemplateDetail {
  id: number
  name: string
  displayName: string
  description: string
  category: string
  icon: string
  source: string
  createdAt: string
  updatedAt: string
  versions: TemplateVersionItem[]
}

export interface TemplateVersionDetail {
  id: number
  templateId: number
  version: string
  spec: string
  contentHash: string
  createdBy: string
  createdAt: string
}

export interface RegisterTemplateResult {
  template: { id: number; name: string; displayName: string; source: string }
  version: { id: number; version: string; contentHash: string }
  alreadyExisted: boolean
}

export interface TemplateMapping {
  modelRefs?: Record<string, string>
  namePrefix?: string
}

export interface TemplatePlanItem {
  section: string
  kind: string
  name: string
  detail?: string
}

export interface TemplateConflict {
  resourceType: string
  name: string
  reason: string
}

export interface TemplateRename {
  resourceType: string
  from: string
  to: string
}

export interface TemplateInstallPlan {
  templateId: number
  templateName: string
  version: string
  sections: string[]
  items: TemplatePlanItem[]
  conflicts: TemplateConflict[]
  missingExtensions?: string[]
  renames?: TemplateRename[]
}

export interface TemplateInstallRequest {
  version?: string
  mapping: TemplateMapping
  sections?: string[]
  strategy?: 'fail' | 'rename'
  force?: boolean
  idempotencyKey?: string
  targetRunId?: string
}

export interface TemplateInstallResult {
  plan: TemplateInstallPlan
  created: Record<string, string[]>
  skipped?: string[]
  idempotentReplay?: boolean
  installId?: number
}

export interface TemplateSpec {
  agents?: {
    name: string
    title?: string
    systemPrompt?: string
    personalityPrompt?: string
    modelRef?: string
    tools?: string[]
  }[]
  personalityTemplates?: { name: string; content: string }[]
  groups?: {
    name: string
    description?: string
    channels?: string[]
    memberRefs?: string[]
  }[]
  relations?: {
    fromRef: string
    toRef: string
    relationType: string
    allowedActions?: string[]
  }[]
  workflows?: {
    name: string
    description?: string
    steps: {
      key: string
      name?: string
      type?: string
      actorType?: string
      actorRef?: string
      dependsOn?: string[]
      config?: Record<string, unknown>
      timeoutSeconds?: number
      maxRetries?: number
    }[]
  }[]
  stateSchemas?: {
    namespace: string
    name: string
    version: string
    schema: Record<string, unknown>
  }[]
  extensionDeps?: { name: string; versionRange?: string }[]
  sampleData?: {
    kind: string
    namespace: string
    subjectType: string
    subjectId: string
    data: Record<string, unknown>
  }[]
}

export const templateApi = {
  register: (body: {
    name: string
    displayName: string
    description: string
    category: string
    icon?: string
    version: string
    spec: TemplateSpec
  }) => apiClient.post('/api/v1/admin/templates', body),
  list: (params: { category?: string; page: number; pageSize: number }) =>
    apiClient.get('/api/v1/admin/templates', { params }),
  get: (id: number) => apiClient.get(`/api/v1/admin/templates/${id}`),
  getVersion: (id: number, version: string) =>
    apiClient.get(`/api/v1/admin/templates/${id}/versions/${encodeURIComponent(version)}`),
  preview: (id: number, body: TemplateInstallRequest) =>
    apiClient.post(`/api/v1/admin/templates/${id}/preview`, body),
  install: (id: number, body: TemplateInstallRequest) =>
    apiClient.post(`/api/v1/admin/templates/${id}/install`, body),
  export: (id: number, body: { agentIds?: number[]; groupIds?: string[] }) =>
    apiClient.post(`/api/v1/admin/templates/${id}/export`, body)
}
