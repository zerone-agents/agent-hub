import apiClient from './client'
import type { PaginatedData } from '@/types/api'

export interface ChatSession {
  user_id: string
  id: string
  title: string
  created_at: string
  updated_at: string
  model: string
  model_selection_id?: string
  system_prompt: string
  status: string
  mode: string
  provider_id: string
  agent_id: string
  permission_profile: string
  hidden: boolean
  extra_directories: string
  is_user_bound: boolean
  user_name?: string
  display_name?: string
}

export interface ChatMessage {
  user_id: string
  id: string
  session_id: string
  role: string
  content: string
  created_at: string
  hidden: boolean
  token_usage: string
  feedback: string
  aigc?: string
}

export interface ChatListParams {
  page?: number
  pageSize?: number
}

export const chatApi = {
  listSessions: ({ page = 1, pageSize = 30 }: ChatListParams = {}) =>
    apiClient.get(`/api/v1/admin/chat/sessions?page=${page}&page_size=${pageSize}`),
  getSession: (id: string) => apiClient.get(`/api/v1/admin/chat/sessions/${id}`),
  listMessages: (sessionId: string, { page = 1, pageSize = 50 }: ChatListParams = {}) =>
    apiClient.get(
      `/api/v1/admin/chat/sessions/${sessionId}/messages?page=${page}&page_size=${pageSize}`
    ),
  deleteSession: (id: string) => apiClient.delete(`/api/v1/admin/chat/sessions/${id}`)
}

export type { PaginatedData }
