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

// 临时保留：Task 9 ToolForm 改造后删除（issue #88）
export function useCreateTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Tool>) => toolApi.create(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tools'] })
      message.success('工具已创建')
    },
    onError: (err) => message.error(parseApiError(err))
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

// 临时保留：Task 9 收窄为 { title?; description? }（issue #88）——ToolForm 仍传 isDefault
export function useUpdateTool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: Partial<Tool> }) =>
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
