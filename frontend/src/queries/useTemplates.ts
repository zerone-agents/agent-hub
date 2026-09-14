// H7.3 模板库 react-query hooks。
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { unwrapResponse } from '@/api/client'
import {
  templateApi,
  type RegisterTemplateResult,
  type TemplateDetail,
  type TemplateInstallPlan,
  type TemplateInstallRequest,
  type TemplateInstallResult,
  type TemplateListResult,
  type TemplateSpec,
  type TemplateVersionDetail
} from '@/api/templates'

export interface TemplateListParams {
  category?: string
  page: number
  pageSize: number
}

export function useTemplateList(params: TemplateListParams) {
  return useQuery<TemplateListResult>({
    queryKey: ['templates', 'list', params],
    queryFn: async () => unwrapResponse<TemplateListResult>(await templateApi.list(params))
  })
}

export function useTemplateDetail(id: number | undefined) {
  return useQuery<TemplateDetail>({
    queryKey: ['templates', 'detail', id],
    queryFn: async () => unwrapResponse<TemplateDetail>(await templateApi.get(id as number)),
    enabled: typeof id === 'number' && id > 0
  })
}

export function useTemplateVersion(id: number | undefined, version: string | undefined) {
  return useQuery<TemplateVersionDetail>({
    queryKey: ['templates', 'version', id, version],
    queryFn: async () =>
      unwrapResponse<TemplateVersionDetail>(
        await templateApi.getVersion(id as number, version as string)
      ),
    enabled: typeof id === 'number' && id > 0 && !!version
  })
}

export function useRegisterTemplate() {
  const queryClient = useQueryClient()
  return useMutation<
    RegisterTemplateResult,
    Error,
    {
      name: string
      displayName: string
      description: string
      category: string
      icon?: string
      version: string
      spec: TemplateSpec
    }
  >({
    mutationFn: async (body) => unwrapResponse<RegisterTemplateResult>(await templateApi.register(body)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['templates'] })
    }
  })
}

export function usePreviewTemplate(id: number) {
  return useMutation<TemplateInstallPlan, Error, TemplateInstallRequest>({
    mutationFn: async (body) =>
      unwrapResponse<TemplateInstallPlan>(await templateApi.preview(id, body))
  })
}

export function useInstallTemplate(id: number) {
  const queryClient = useQueryClient()
  return useMutation<TemplateInstallResult, Error, TemplateInstallRequest>({
    mutationFn: async (body) =>
      unwrapResponse<TemplateInstallResult>(await templateApi.install(id, body)),
    onSuccess: () => {
      // 安装会创建 agents/groups/workflows 等资源：失效相关缓存
      void queryClient.invalidateQueries({ queryKey: ['templates'] })
      void queryClient.invalidateQueries({ queryKey: ['agents'] })
      void queryClient.invalidateQueries({ queryKey: ['groups'] })
      void queryClient.invalidateQueries({ queryKey: ['workflows'] })
      void queryClient.invalidateQueries({ queryKey: ['personalities'] })
    }
  })
}

export function useExportTemplate(id: number) {
  return useMutation<TemplateSpec, Error, { agentIds?: number[]; groupIds?: string[] }>({
    mutationFn: async (body) => unwrapResponse<TemplateSpec>(await templateApi.export(id, body))
  })
}
