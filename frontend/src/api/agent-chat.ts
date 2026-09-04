import apiClient, { unwrapResponse } from './client'

export interface AgentChatSession {
  id: string
  user_id: string
  agent_id: string
  title: string
  model: string
  system_prompt: string
  status: string
  mode: string
  provider_id: string
  permission_profile: string
  created_at: string
  updated_at: string
}

export interface AgentChatMessage {
  id: string
  user_id: string
  session_id: string
  role: string
  content: string
  created_at: string
  aigc?: string
}

export interface AgentChatListParams {
  page?: number
  pageSize?: number
  source?: string
}

export interface AttachmentDesc {
  id: string
  name: string
  mime: string
  size: number
  path: string
}

export interface AgentChatCapabilities {
  attachmentsEnabled: boolean
}

/** 带 HTTP 状态与后端稳定错误码的错误（issue #94 契约）。 */
export class ApiError extends Error {
  readonly status: number
  readonly code?: string
  constructor(message: string, status: number, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

/** 从非 2xx 响应解析 envelope，尽可能携带 error + code。 */
async function apiErrorFromResponse(resp: Response): Promise<ApiError> {
  const text = await resp.text().catch(() => '')
  try {
    const body = JSON.parse(text) as { error?: string; code?: string }
    return new ApiError(body.error ?? `HTTP ${resp.status}`, resp.status, body.code)
  } catch {
    return new ApiError(`HTTP ${resp.status}: ${text.slice(0, 200)}`, resp.status)
  }
}

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('access_token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export const agentChatApi = {
  listSessions: (agentName: string, { page = 1, pageSize = 30, source }: AgentChatListParams = {}) => {
    let url = `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions?page=${page}&page_size=${pageSize}`
    if (source) url += `&source=${encodeURIComponent(source)}`
    return apiClient.get(url)
  },

  createSession: (agentName: string, title?: string) =>
    apiClient.post(
      `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions`,
      title ? { title } : {}
    ),

  listMessages: (agentName: string, sessionId: string, { page = 1, pageSize = 50 }: AgentChatListParams = {}) =>
    apiClient.get(
      `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions/${sessionId}/messages?page=${page}&page_size=${pageSize}`
    ),

  deleteSession: (agentName: string, sessionId: string) =>
    apiClient.delete(
      `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions/${sessionId}`
    ),

  getCapabilities: async (agentName: string): Promise<AgentChatCapabilities> => {
    const res = await apiClient.get(`/api/v1/agents/${encodeURIComponent(agentName)}/chat/capabilities`)
    const body = unwrapResponse<{ attachmentsEnabled: boolean } | null>(res)
    return body ?? { attachmentsEnabled: false }
  },

  uploadFiles: async (agentName: string, sessionId: string, files: File[]): Promise<AttachmentDesc[]> => {
    const form = new FormData()
    for (const f of files) form.append('files', f)
    // axios 默认 10s 超时对 50MB 上传不够；原生 fetch + 120s 显式超时。
    // 不要手动设置 Content-Type —— boundary 必须由浏览器生成。
    const resp = await fetch(
      `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions/${sessionId}/uploads`,
      { method: 'POST', headers: { ...authHeaders() }, body: form, signal: AbortSignal.timeout(120_000) }
    )
    if (!resp.ok) throw await apiErrorFromResponse(resp)
    const body = (await resp.json()) as {
      success: boolean
      data?: { files: AttachmentDesc[] }
      error?: string
    }
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- cross-boundary JSON: runtime may omit data.files
    if (!body.success || !body.data?.files?.length) {
      throw new ApiError(body.error ?? '上传失败', resp.status)
    }
    return body.data.files
  },

  /**
   * Sends a message and returns a raw fetch Response whose body is a
   * ReadableStream of SSE bytes. We intentionally bypass axios here because
   * axios does not expose a streaming reader in the browser.
   *
   * The optional AbortSignal lets the caller cancel an in-flight stream
   * (stop button or idle-timeout guard in useChatStream).
   */
  sendMessageStream: async (
    agentName: string,
    sessionId: string,
    content: string,
    signal?: AbortSignal,
    attachments?: AttachmentDesc[]
  ): Promise<Response> => {
    const resp = await fetch(
      `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions/${sessionId}/messages`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...authHeaders(),
        },
        body: JSON.stringify(
          attachments && attachments.length > 0 ? { content, attachments } : { content }
        ),
        signal,
      }
    )
    if (!resp.ok) throw await apiErrorFromResponse(resp)
    if (!resp.body) {
      throw new Error('response body is null; streaming not supported')
    }
    return resp
  },
}

/** 用户态 session 级附件内容代理 URL（图片预览/下载共用，token 走 header）。 */
export function attachmentContentUrl(agentName: string, sessionId: string, path: string): string {
  return `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions/${encodeURIComponent(sessionId)}/attachments/content?path=${encodeURIComponent(path)}`
}

/** 鉴权 fetch → blob。URL 本身不携带 token（spec：Token 不写入 URL）。 */
export async function authFetchBlob(url: string, signal?: AbortSignal): Promise<Blob> {
  const resp = await fetch(url, { headers: { ...authHeaders() }, signal })
  if (!resp.ok) throw await apiErrorFromResponse(resp)
  return resp.blob()
}
