import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { chatApi, type ChatSession, type ChatMessage } from '@/api/chat'
import type { PaginatedData } from '@/types/api'
import { parseApiError, unwrapResponse } from '@/api/client'
import { useTranslation } from 'react-i18next'

export function useChatSessions() {
  const [page, setPage] = useState(1)
  // 20 必须在条数下拉的选项集合（antd 默认 10/20/50/100）内：rc-pagination 会把当前
  // pageSize 并进选项，默认值若在集合外就会多出一个"野"选项，且与其它列表页（审计 20、
  // 知识库 10）不一致。历史上这里是 30。
  const [pageSize, setPageSize] = useState(20)

  const query = useQuery<PaginatedData<ChatSession>>({
    queryKey: ['chat-sessions', page, pageSize],
    queryFn: async () =>
      unwrapResponse<PaginatedData<ChatSession>>(await chatApi.listSessions({ page, pageSize }))
  })

  return { ...query, page, pageSize, setPage, setPageSize }
}

export function useChatMessages(sessionId: string | null) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)

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

  return { ...query, page, pageSize, setPage, setPageSize }
}

export function useDeleteChatSession() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => chatApi.deleteSession(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['chat-sessions'] })
      message.success(t('chat.toast.sessionDeleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
