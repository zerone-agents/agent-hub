import apiClient from './client'

export interface AgentConfig {
  id?: number
  name?: string
  contentHash?: string
  systemPrompt?: string
  permissionMode?: string
  maxTurns?: number
  maxSessionQueries?: number
  disallowedTools?: string[]
  title?: Record<string, string>
  description?: Record<string, string>
  icon?: string
  iconName?: string
  iconColor?: string
  iconBgColor?: string
  providerId?: number | null
  modelId?: string
  modelSelectionId?: string
  fieldOverrides?: Record<string, string>
  source?: string
  isDefault?: boolean
  group?: string
  createdAt?: string
  updatedAt?: string
}

export interface DeploymentStatus {
  /** Known values: running, stopped, exited, not_found, error, unknown. May include other Docker container states. */
  status: string
  health?: 'starting' | 'healthy' | 'unhealthy' | 'none'
  hostPort?: number
  runtimeUrl?: string
  containerName?: string
  deployedAt?: string
  message?: string
  apiKey?: string
}

export interface Agent {
  id: number
  name: string
  contentHash?: string
  config: AgentConfig
  subagents?: string[]
  tools?: string[]
  skills?: string[]
  mcps?: string[]
  datasets?: string[]
  subAgentCount?: number
  toolCount?: number
  skillCount?: number
  desktopEnabled?: boolean
  mobileEnabled?: boolean
  isDefault?: boolean
  group?: string
  createdAt?: string
  updatedAt?: string
}

export const agentApi = {
  list: () => apiClient.get('/api/v1/admin/agents'),
  get: (name: string) => apiClient.get(`/api/v1/agents/${encodeURIComponent(name)}`),
  create: (data: Partial<Agent>) => apiClient.post('/api/v1/admin/agents', data),
  update: (name: string, data: Partial<Agent>) => apiClient.put(`/api/v1/admin/agents/${encodeURIComponent(name)}`, data),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/agents/${encodeURIComponent(name)}`),
  updateSubagents: (name: string, subagents: string[]) =>
    apiClient.put(`/api/v1/admin/agents/${encodeURIComponent(name)}/subagents`, { subagents }),
  getTools: (name: string) => apiClient.get(`/api/v1/admin/agents/${encodeURIComponent(name)}/tools`),
  updateTools: (name: string, toolNames: string[]) =>
    apiClient.put(`/api/v1/admin/agents/${encodeURIComponent(name)}/tools`, { toolNames }),
  getSkills: (name: string) => apiClient.get(`/api/v1/admin/agents/${encodeURIComponent(name)}/skills`),
  updateSkills: (name: string, skillNames: string[]) =>
    apiClient.put(`/api/v1/admin/agents/${encodeURIComponent(name)}/skills`, { skillNames }),
  getMcps: (name: string) => apiClient.get(`/api/v1/admin/agents/${encodeURIComponent(name)}/mcps`),
  updateMcps: (name: string, mcpNames: string[]) =>
    apiClient.put(`/api/v1/admin/agents/${encodeURIComponent(name)}/mcps`, { mcpNames }),
  getKnowledgeDatasets: (name: string) =>
    apiClient.get(`/api/v1/admin/agents/${encodeURIComponent(name)}/knowledge`),
  updateKnowledgeDatasets: (name: string, datasetIds: string[]) =>
    apiClient.put(`/api/v1/admin/agents/${encodeURIComponent(name)}/knowledge`, { dataset_ids: datasetIds }),
  deploy: (name: string, force?: boolean, rotateKey?: boolean) =>
    apiClient.post(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`, {}, {
      params: {
        ...(force ? { force: 'true' } : {}),
        ...(rotateKey ? { rotate_key: 'true' } : {}),
      },
      timeout: 120000,
    }),
  getDeployment: (name: string) =>
    apiClient.get(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`),
  stopDeployment: (name: string) =>
    apiClient.post(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy/stop`, {}, {
      timeout: 120000,
    }),
  startDeployment: (name: string) =>
    apiClient.post(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy/start`, {}, {
      timeout: 120000,
    }),
  deleteDeployment: (name: string) =>
    apiClient.delete(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`, {
      timeout: 120000,
    }),
  purgeDeployment: (name: string) =>
    apiClient.delete(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy?purge=true`, {
      timeout: 120000,
    }),
  probe: (name: string, data: { providerId?: number; apiKey: string; baseUrl: string }) =>
    apiClient.post(`/api/v1/admin/agents/${encodeURIComponent(name)}/probe`, data, { timeout: 120000 }),
  getDetail: (name: string) =>
    apiClient.get(`/api/v1/admin/agents/${encodeURIComponent(name)}/detail`),
}

// ============================================
// AgentDetail — mirrors runtime GET /v1/agents/:agentId response.
// Optional fields are kept optional (the runtime omits unset fields
// rather than returning null). See Agent Detail API spec.
// ============================================

export interface McpServerSummary {
  transport: 'stdio' | 'sse' | 'http'
  // stdio-specific (only present when transport === 'stdio')
  command?: string
  args?: string[]
  env?: Record<string, string>      // values pre-redacted to "***" by runtime
  // sse / http-specific (only present when transport === 'sse' | 'http')
  url?: string
  headers?: Record<string, string>  // values pre-redacted to "***" by runtime
}

export interface AgentDetailSkill {
  name: string
  description: string
  source: string
  location: string
}

export interface AgentDetailSubagent {
  description: string
}

export interface AgentDetail {
  // Always returned
  id: string
  name: string
  model: string
  status: 'ready' | 'unavailable'
  maxTurns: number
  hasSystemPrompt: boolean
  // Optional (only present when configured)
  maxSessionQueries?: number
  permissionMode?: string
  allowedTools?: string[]
  disallowedTools?: string[]
  availableSkills?: AgentDetailSkill[]
  settingSources?: ('user' | 'project' | 'local')[]
  extraUserSkillDirs?: string[]
  extraProjectSkillDirs?: string[]
  mcpServers?: Record<string, McpServerSummary>
  subagents?: Record<string, AgentDetailSubagent>
  datasets?: Record<string, string>
}
