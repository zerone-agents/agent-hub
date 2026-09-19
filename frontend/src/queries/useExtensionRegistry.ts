// H7.0 扩展注册中心的 react-query hooks。
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { unwrapResponse } from '@/api/client'
import {
  extensionRegistryApi,
  type ExtensionDetail,
  type ExtensionImpact,
  type ExtensionInstallResult,
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
      unwrapResponse<ExtensionDetail>(await extensionRegistryApi.get(id!)),
    enabled: typeof id === 'number' && id > 0
  })
}

export function useExtensionVersion(id: number | undefined, version: string | undefined) {
  return useQuery<ExtensionVersionDetail>({
    queryKey: ['extensions', 'registry', 'version', id, version],
    queryFn: async () =>
      unwrapResponse<ExtensionVersionDetail>(
        await extensionRegistryApi.getVersion(id!, version!)
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

// H7.1 生命周期 mutations：成功后统一失效注册中心缓存
function useLifecycleMutation<TVariables>(
  mutationFn: (vars: TVariables) => Promise<ExtensionInstallResult>
) {
  const queryClient = useQueryClient()
  return useMutation<ExtensionInstallResult, Error, TVariables>({
    mutationFn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['extensions', 'registry'] })
    }
  })
}

export function useInstallExtension(id: number) {
  return useLifecycleMutation(async (version: string) =>
    unwrapResponse<ExtensionInstallResult>(await extensionRegistryApi.install(id, version))
  )
}

export function useEnableExtension(id: number) {
  return useLifecycleMutation<void>(async () =>
    unwrapResponse<ExtensionInstallResult>(await extensionRegistryApi.enable(id))
  )
}

export function useDisableExtension(id: number) {
  return useLifecycleMutation<void>(async () =>
    unwrapResponse<ExtensionInstallResult>(await extensionRegistryApi.disable(id))
  )
}

export function useUpgradeExtension(id: number) {
  return useLifecycleMutation(async (targetVersion: string) =>
    unwrapResponse<ExtensionInstallResult>(await extensionRegistryApi.upgrade(id, targetVersion))
  )
}

export function useRollbackExtension(id: number) {
  return useLifecycleMutation(async (targetVersion?: string) =>
    unwrapResponse<ExtensionInstallResult>(await extensionRegistryApi.rollback(id, targetVersion))
  )
}

export function useUninstallExtension(id: number) {
  return useLifecycleMutation(
    async ({ force, purge }: { force?: boolean; purge?: boolean }) =>
      unwrapResponse<ExtensionInstallResult>(await extensionRegistryApi.uninstall(id, force, purge))
  )
}

export function useExtensionImpact(id: number | undefined) {
  return useQuery<ExtensionImpact>({
    queryKey: ['extensions', 'registry', 'impact', id],
    queryFn: async () =>
      unwrapResponse<ExtensionImpact>(await extensionRegistryApi.impact(id!)),
    enabled: typeof id === 'number' && id > 0
  })
}
