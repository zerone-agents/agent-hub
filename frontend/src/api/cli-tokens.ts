import apiClient from './client'

export interface CLIToken {
  id: number
  name: string
  createdAt: string
  lastUsedAt?: string | null
  expiresAt: string
}

export interface IssueTokenResponse {
  token: string
  expiresAt: string
}

export const cliTokensApi = {
  list: () =>
    apiClient.get<{ success: boolean; data: { items: CLIToken[] } }>('/api/v1/cli/tokens'),

  issue: (name: string, ttlDays: number) =>
    apiClient.post<{ success: boolean; data: IssueTokenResponse }>('/api/v1/cli/issue-token', {
      name,
      ttlDays
    }),

  revoke: (id: number) =>
    apiClient.delete<{ success: boolean; message: string }>(`/api/v1/cli/tokens/${id}`)
}
