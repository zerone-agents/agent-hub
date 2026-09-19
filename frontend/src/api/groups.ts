import apiClient from './client'

export type GroupVisibility = 'private' | 'tenant'
export type GroupMemberRole = 'leader' | 'member' | 'observer' | 'guest'
export type SubscriptionMode = 'all' | 'mentions' | 'none'
export type SessionStatus = 'draft' | 'active' | 'completed'

export interface GroupMember {
  id?: number
  agentId: number
  agentName?: string
  role: GroupMemberRole
  joinedAt?: string
}

export interface CollaborationGroup {
  id: string
  name: string
  description?: string
  visibility: GroupVisibility
  memberCount?: number
  channelCount?: number
  members?: GroupMember[]
  createdAt?: string
  updatedAt?: string
}

export interface ChannelSubscription {
  agentId: number
  agentName?: string
  mode: SubscriptionMode
}

export interface GroupMessage {
  id: string
  dispatchId?: string
  groupId?: string
  channelId: string
  sessionId?: string
  sourceAgentId?: number
  sourceAgent?: string
  targetAgent?: string
  action?: string
  content: string
  status?: string
  audience?: string
  reply?: string
  error?: string
  completedAt?: string
  verified?: boolean
  verification?: string
  createdAt: string
}

export interface GroupChannel {
  id: string
  groupId: string
  name: string
  topic?: string
  visibility: 'group' | 'members'
  messageCount?: number
  subscribers?: ChannelSubscription[]
  messages?: GroupMessage[]
  createdAt?: string
}

export interface ConversationSession {
  id: string
  channelId: string
  agenda: string
  hostAgentId?: number
  hostAgentName?: string
  participantAgentIds?: number[]
  participants?: { id: number; sessionId: string; agentId: number; role: 'host' | 'participant'; joinedAt: string }[]
  status: SessionStatus
  summary?: string
  startedAt?: string
  completedAt?: string
  createdAt?: string
}

export interface GroupAuditEvent {
  id: string
  action: string
  resourceType?: 'group' | 'member' | 'channel' | 'subscription' | 'session'
  resourceId?: string
  agentId?: number
  actorId?: string
  before?: Record<string, unknown>
  after?: Record<string, unknown>
  actorName?: string
  description?: string
  createdAt: string
}

export const groupApi = {
  list: () => apiClient.get('/api/v1/admin/groups'),
  create: (data: Pick<CollaborationGroup, 'name' | 'description' | 'visibility'>) => apiClient.post('/api/v1/admin/groups', data),
  get: (id: string) => apiClient.get(`/api/v1/admin/groups/${id}`),
  update: (id: string, data: Partial<CollaborationGroup>) => apiClient.put(`/api/v1/admin/groups/${id}`, data),
  delete: (id: string) => apiClient.delete(`/api/v1/admin/groups/${id}`),
  members: (groupId: string) => apiClient.get(`/api/v1/admin/groups/${groupId}/members`),
  addMember: (groupId: string, data: Pick<GroupMember, 'agentId' | 'role'>) => apiClient.post(`/api/v1/admin/groups/${groupId}/members`, data),
  updateMember: (groupId: string, agentId: number, role: GroupMemberRole) => apiClient.patch(`/api/v1/admin/groups/${groupId}/members/${agentId}`, { role }),
  removeMember: (groupId: string, agentId: number) => apiClient.delete(`/api/v1/admin/groups/${groupId}/members/${agentId}`),
  audit: (groupId: string) => apiClient.get(`/api/v1/admin/groups/${groupId}/audit`),
  channels: (groupId: string) => apiClient.get(`/api/v1/admin/groups/${groupId}/channels`),
  addChannel: (groupId: string, data: Pick<GroupChannel, 'name' | 'topic' | 'visibility'>) => apiClient.post(`/api/v1/admin/groups/${groupId}/channels`, data),
  getChannel: (id: string) => apiClient.get(`/api/v1/admin/channels/${id}`),
  subscriptions: (channelId: string) => apiClient.get(`/api/v1/admin/channels/${channelId}/subscriptions`),
  messages: (channelId: string) => apiClient.get(`/api/v1/admin/channels/${channelId}/messages`, { params: { limit: 100 } }),
  updateSubscription: (channelId: string, agentId: number, mode: SubscriptionMode) => apiClient.put(`/api/v1/admin/channels/${channelId}/subscriptions`, { agentId, mode }),
  sessions: (channelId: string) => apiClient.get(`/api/v1/admin/channels/${channelId}/sessions`),
  createSession: (channelId: string, data: Pick<ConversationSession, 'agenda' | 'hostAgentId' | 'participantAgentIds'>) => apiClient.post(`/api/v1/admin/channels/${channelId}/sessions`, data),
  getSession: (id: string) => apiClient.get(`/api/v1/admin/sessions/${id}`),
  startSession: (id: string) => apiClient.post(`/api/v1/admin/sessions/${id}/start`),
  completeSession: (id: string, summary: string) => apiClient.post(`/api/v1/admin/sessions/${id}/complete`, { summary }),
}
