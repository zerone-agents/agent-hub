import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { chatApi, type ChatSession, type ChatMessage } from '@/api/chat'
import type { PaginatedData } from '@/types/api'
import { parseApiError, unwrapResponse } from '@/api/client'

export function useChatSessions() {
  const [page, setPage] = useState(1)
  const [pageSize] = useState(30)

  const query = useQuery<PaginatedData<ChatSession>>({
    queryKey: ['chat-sessions', page, pageSize],
    queryFn: async () =>
      unwrapResponse<PaginatedData<ChatSession>>(await chatApi.listSessions({ page, pageSize }))
  })

  return { ...query, page, pageSize, setPage }
}

export function useChatMessages(sessionId: string | null) {
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)

  const query = useQuery<PaginatedData<ChatMessage>>({
    queryKey: ['chat-messages', sessionId, page, pageSize],
    queryFn: async () => {
      if (!sessionId) throw new Error('No session selected')
      return unwrapResponse<PaginatedData<ChatMessage>>(
        await chatApi.listMessages(sessionId, { page, pageSize })
      )
    },
    enabled: !!sessionId
  })

  return { ...query, page, pageSize, setPage }
}

export function useDeleteChatSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => chatApi.deleteSession(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['chat-sessions'] })
      message.success('会话已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
