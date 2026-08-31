import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { toolApi, type Tool } from '@/api/tools'
import { parseApiError, unwrapResponse } from '@/api/client'

export function useTools() {
  return useQuery<Tool[]>({
    queryKey: ['tools'],
    queryFn: async () => {
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- defensive: backend may omit data field
      return unwrapResponse<Tool[]>(await toolApi.list()) ?? []
    }
  })
}

export function useCreateCustomTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { name: string; title?: string; description?: string; file: File }) =>
      toolApi.createCustom(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('自定义工具已上传')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUploadToolFile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, file }: { name: string; file: File }) => toolApi.uploadFile(name, file),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('工具文件已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: { title?: string; description?: string } }) =>
      toolApi.update(name, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tools'] })
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
      void qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('工具已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
