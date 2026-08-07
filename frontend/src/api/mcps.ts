import apiClient from './client'

export type McpTransportType = 'sse' | 'http'

export interface McpTool {
  name: string
  description: string
}

export interface McpProbeInput {
  transportType: McpTransportType
  url?: string
  headers?: Record<string, string>
}

export interface McpProbeResult {
  status: 'success' | 'failed' | 'unsupported'
  tools?: McpTool[]
  error?: string
}

export interface Mcp {
  id: number
  name: string
  title: string
  description: string
  transportType: McpTransportType
  url?: string
  hasHeaders: boolean
  isBuiltin: boolean
  retryMaxRetries?: number | null
  retryTimeoutMs?: number | null
  tools?: McpTool[]
  probeStatus: 'pending' | 'success' | 'failed'
  lastProbedAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface McpDetail extends Mcp {
  headers: Record<string, string>
}

export interface McpInput {
  name?: string
  title?: string
  description?: string
  transportType?: McpTransportType
  url?: string
  headers?: Record<string, string>
  retryMaxRetries?: number | null
  retryTimeoutMs?: number | null
  tools?: McpTool[]
}

export const mcpApi = {
  list: () => apiClient.get('/api/v1/admin/mcps'),
  get: (name: string) => apiClient.get(`/api/v1/admin/mcps/${name}`),
  create: (data: McpInput) => apiClient.post('/api/v1/admin/mcps', data),
  update: (name: string, data: McpInput) => apiClient.put(`/api/v1/admin/mcps/${name}`, data),
  delete: (name: string) => apiClient.delete(`/api/v1/admin/mcps/${name}`),
  // Agent ↔ MCP 绑定
  getAgentMcps: (agentName: string) =>
    apiClient.get(`/api/v1/admin/agents/${agentName}/mcps`),
  updateAgentMcps: (agentName: string, mcpNames: string[]) =>
    apiClient.put(`/api/v1/admin/agents/${agentName}/mcps`, { mcpNames }),
  probeByConfig: (data: McpProbeInput) =>
    apiClient.post('/api/v1/admin/mcps/probe', data),
  probeByName: (name: string) =>
    apiClient.post(`/api/v1/admin/mcps/${name}/probe`)
}
