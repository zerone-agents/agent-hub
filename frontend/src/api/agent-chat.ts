import apiClient from './client'

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
    signal?: AbortSignal
  ): Promise<Response> => {
    const resp = await fetch(
      `/api/v1/agents/${encodeURIComponent(agentName)}/chat/sessions/${sessionId}/messages`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...authHeaders(),
        },
        body: JSON.stringify({ content }),
        signal,
      }
    )
    if (!resp.ok) {
      const text = await resp.text().catch(() => '')
      throw new Error(`HTTP ${resp.status}: ${text}`)
    }
    if (!resp.body) {
      throw new Error('response body is null; streaming not supported')
    }
    return resp
  },
}
