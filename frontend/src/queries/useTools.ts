import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { toolApi, type Tool } from '@/api/tools'
import { parseApiError } from '@/api/client'

export function useTools() {
  return useQuery<Tool[]>({
    queryKey: ['tools'],
    queryFn: async () => {
      const res = await toolApi.list()
      if (!res.data.success) throw new Error(res.data.message)
      return res.data.data ?? []
    }
  })
}

export function useCreateTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Tool>) => toolApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('工具已创建')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: Partial<Tool> }) =>
      toolApi.update(name, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('工具已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => toolApi.delete(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('工具已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
