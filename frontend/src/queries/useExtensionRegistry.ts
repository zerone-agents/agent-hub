// H7.0 扩展注册中心的 react-query hooks。
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { unwrapResponse } from '@/api/client'
import {
  extensionRegistryApi,
  type ExtensionDetail,
  type ExtensionListResult,
  type ExtensionVersionDetail,
  type RegisterExtensionResult
} from '@/api/extensionRegistry'

export interface ExtensionListParams {
  status?: string
  source?: string
  page: number
  pageSize: number
}

export function useExtensionList(params: ExtensionListParams) {
  return useQuery<ExtensionListResult>({
    queryKey: ['extensions', 'registry', 'list', params],
    queryFn: async () =>
      unwrapResponse<ExtensionListResult>(await extensionRegistryApi.list(params))
  })
}

export function useExtensionDetail(id: number | undefined) {
  return useQuery<ExtensionDetail>({
    queryKey: ['extensions', 'registry', 'detail', id],
    queryFn: async () =>
      unwrapResponse<ExtensionDetail>(await extensionRegistryApi.get(id as number)),
    enabled: typeof id === 'number' && id > 0
  })
}

export function useExtensionVersion(id: number | undefined, version: string | undefined) {
  return useQuery<ExtensionVersionDetail>({
    queryKey: ['extensions', 'registry', 'version', id, version],
    queryFn: async () =>
      unwrapResponse<ExtensionVersionDetail>(
        await extensionRegistryApi.getVersion(id as number, version as string)
      ),
    enabled: typeof id === 'number' && id > 0 && !!version
  })
}

export function useRegisterExtension() {
  const queryClient = useQueryClient()
  return useMutation<RegisterExtensionResult, Error, { manifest: string; source?: string; changelog?: string }>({
    mutationFn: async ({ manifest, source, changelog }) =>
      unwrapResponse<RegisterExtensionResult>(
        await extensionRegistryApi.register(manifest, source, changelog)
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['extensions', 'registry'] })
    }
  })
}
