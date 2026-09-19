import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { groupApi, type CollaborationGroup, type ConversationSession, type GroupAuditEvent, type GroupChannel, type GroupMember, type ChannelSubscription, type GroupMessage } from '@/api/groups'
import { parseApiError, unwrapResponse } from '@/api/client'

function listFrom<T>(value: T[] | { items?: T[]; groups?: T[]; events?: T[] }): T[] {
  if (Array.isArray(value)) return value
  return value.items ?? value.groups ?? value.events ?? []
}

export function useGroups() {
  return useQuery({ queryKey: ['groups'], queryFn: async () => listFrom(unwrapResponse<CollaborationGroup[] | { groups?: CollaborationGroup[] }>(await groupApi.list())) })
}

export function useGroup(id?: string) {
  return useQuery({ queryKey: ['groups', id], queryFn: async () => unwrapResponse<CollaborationGroup>(await groupApi.get(id!)), enabled: Boolean(id) })
}

export function useGroupAudit(id?: string) {
  return useQuery({ queryKey: ['groups', id, 'audit'], queryFn: async () => listFrom(unwrapResponse<GroupAuditEvent[] | { events?: GroupAuditEvent[] }>(await groupApi.audit(id!))), enabled: Boolean(id) })
}

export function useGroupMembers(id?: string) { return useQuery({ queryKey:['groups',id,'members'], queryFn:async()=>listFrom(unwrapResponse<GroupMember[]>(await groupApi.members(id!))), enabled:Boolean(id) }) }
export function useGroupChannels(id?: string) { return useQuery({ queryKey:['groups',id,'channels'], queryFn:async()=>listFrom(unwrapResponse<GroupChannel[]>(await groupApi.channels(id!))), enabled:Boolean(id) }) }
export function useChannelSubscriptions(id?: string) { return useQuery({ queryKey:['channels',id,'subscriptions'], queryFn:async()=>listFrom(unwrapResponse<ChannelSubscription[]>(await groupApi.subscriptions(id!))), enabled:Boolean(id) }) }
export function useChannelMessages(id?: string) { return useQuery({ queryKey:['channels',id,'messages'], queryFn:async()=>listFrom(unwrapResponse<GroupMessage[]>(await groupApi.messages(id!))), enabled:Boolean(id), refetchInterval:3000 }) }
export function useChannelSessions(id?: string) { return useQuery({ queryKey:['channels',id,'sessions'], queryFn:async()=>listFrom(unwrapResponse<ConversationSession[]>(await groupApi.sessions(id!))), enabled:Boolean(id), refetchInterval:3000 }) }
export function useChannel(id?: string) {
  return useQuery({ queryKey: ['channels', id], queryFn: async () => unwrapResponse<GroupChannel>(await groupApi.getChannel(id!)), enabled: Boolean(id), refetchInterval: 3000 })
}

export function useGroupAction<T>(mutationFn: (input: T) => Promise<unknown>, success: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: () => { void client.invalidateQueries({ queryKey: ['groups'] }); void client.invalidateQueries({ queryKey: ['channels'] }); message.success(success) },
    onError: (error) => message.error(parseApiError(error)),
  })
}
