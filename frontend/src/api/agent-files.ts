import apiClient, { getAccessToken } from './client'

// Mirrors runtime GET /v1/files entry shape. Optional fields are kept
// optional because the runtime omits them rather than returning null
// (see open-agent-runtime cwd file interface spec §1.1).
export interface FileEntry {
  name: string
  type: 'file' | 'directory' | 'symlink' | 'other'
  size?: number
  mtime?: string
  mime?: string           // only for type === 'file'
  target?: string         // only for type === 'symlink' && target inside cwd
}

export interface ListFilesResponse {
  path: string
  entries: FileEntry[]
}

export interface ListFilesParams {
  path?: string
  recursive?: boolean
  depth?: number
}

const baseUrl = (agentName: string) =>
  `/api/v1/admin/agents/${encodeURIComponent(agentName)}/files`

const contentUrl = (agentName: string, path: string) =>
  `${baseUrl(agentName)}/content?path=${encodeURIComponent(path)}`

/**
 * agentFilesApi — typed client for the control-panel file proxy endpoints.
 *
 * `list` goes through axios so it inherits the Authorization interceptor
 * and silent refresh.
 *
 * `getContent` / `head` use raw fetch instead of axios so we can stream the
 * body (axios buffers it fully). The 512KB preview cap is enforced by the
 * caller, not here — this function returns the raw Response and the caller
 * decides how much to read.
 */
export const agentFilesApi = {
  list: (agentName: string, params: ListFilesParams = {}) =>
    apiClient.get<ListFilesResponse>(baseUrl(agentName), { params }),

  getContent: (
    agentName: string,
    path: string,
    init: { rangeHeader?: string; signal?: AbortSignal } = {}
  ) => {
    const headers: Record<string, string> = {}
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`
    if (init.rangeHeader) headers.Range = init.rangeHeader
    return fetch(contentUrl(agentName, path), {
      method: 'GET',
      headers,
      signal: init.signal,
    })
  },

  head: (agentName: string, path: string) => {
    const headers: Record<string, string> = {}
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`
    return fetch(contentUrl(agentName, path), {
      method: 'HEAD',
      headers,
    })
  },
}
