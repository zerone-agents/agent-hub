import type { KnowledgeWriteOwner } from '@/api/knowledge'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { providerApi, type Provider, type ProbeConfig, type ProbeOverride, type AttrRules } from '@/api/providers'
import apiClient, { getAccessToken, parseApiError, unwrapResponse } from '@/api/client'
import { useTranslation } from 'react-i18next'
import i18next from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { ownedRequestConfig, type RequestOwner } from '@/api/requestOwnership'
import type { UserInfoResponse } from '@/api/auth'
import type { ApiResponse } from '@/types/api'

type ProviderType = 'llm' | 'ocr' | 'embedding' | 'vlm' | 'chat'
const providerOrigins = new WeakMap<Provider[], { id: string; token: string | null }>()

export async function readOwnedProviders(type: ProviderType | undefined, signal: AbortSignal): Promise<Provider[]> {
  const id = useAuthStore.getState().user?.id
  let token = getAccessToken()
  const current = () => Boolean(id && !signal.aborted && useAuthStore.getState().user?.id === id)
  const owner: RequestOwner = {
    signal,
    isCurrent: () => current() && getAccessToken() === token,
    assertCurrent: (refresh) => {
      if (refresh && current()) token = getAccessToken()
      if (!current() || getAccessToken() !== token) throw new Error(i18next.t('knowledge.list.deleteOwnerChanged'))
    },
  }
  owner.assertCurrent()
  const response = await apiClient.get<ApiResponse<UserInfoResponse>>('/auth/userinfo', ownedRequestConfig(owner))
  owner.assertCurrent()
  const identity = unwrapResponse<UserInfoResponse>(response)
  if ((identity.user_id ?? identity.id) !== id) throw new Error(i18next.t('knowledge.list.deleteOwnerChanged'))
  const providers = unwrapResponse<Provider[]>(await providerApi.list(type, owner))
  owner.assertCurrent()
  if (!Array.isArray(providers)) throw new Error(i18next.t('apiErrors.requestFailed'))
  providerOrigins.set(providers, { id: id ?? '', token })
  return providers
}

export function useProviders(type?: ProviderType, options: { owned?: boolean } = {}) {
  const accountId = useAuthStore((state) => state.user?.id)
  const query = useQuery<Provider[]>({
    queryKey: options.owned ? ['providers', type ?? 'all', 'owned', accountId] : ['providers', type ?? 'all'],
    queryFn: ({ signal }) => options.owned ? readOwnedProviders(type, signal) : providerApi.list(type).then(unwrapResponse<Provider[]>),
    enabled: !options.owned || Boolean(accountId),
    structuralSharing: options.owned ? false : undefined,
  })
  const origin = query.data ? providerOrigins.get(query.data) : undefined
  const ownerChanged = Boolean(options.owned && query.data && (origin?.id !== accountId || origin?.token !== getAccessToken()))
  return {
    ...query,
    data: ownerChanged || (options.owned && query.isError) ? undefined : query.data,
    isError: query.isError || ownerChanged,
    isSuccess: query.isSuccess && !ownerChanged,
    error: query.error ?? (ownerChanged ? new Error(i18next.t('knowledge.list.deleteOwnerChanged')) : null),
  }
}

// useProviderAttrRules loads the attribute rules used to dynamically
// render the provider form. Fetched once (full map) and cached;
// the form filters by the selected protocol locally.
export function useProviderAttrRules() {
  return useQuery<AttrRules>({
    queryKey: ['provider-attr-rules'],
    queryFn: async () => unwrapResponse<AttrRules>(await providerApi.attrRules()),
  })
}

export function useCreateProvider() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Provider>) => providerApi.create(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['providers'] })
      message.success(t('providers.toast.created'))
    },
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useUpdateProvider() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Provider> }) =>
      providerApi.update(id, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['providers'] })
      message.success(t('providers.toast.updated'))
    },
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useDeleteProvider() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => providerApi.delete(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['providers'] })
      message.success(t('providers.toast.deleted'))
    },
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useProbeProvider() {
  return useMutation({
    mutationFn: ({ id, ...payload }: { id: number } & ProbeOverride) =>
      providerApi.probe(id, payload),
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useProbeConfig() {
  return useMutation({
    mutationFn: (config: ProbeConfig) => providerApi.probeConfig(config),
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useSyncProviderMultiRAG(options: { getOwner?: () => KnowledgeWriteOwner | undefined } = {}) {
  return useMutation({
    onMutate: () => options.getOwner?.(),
    mutationFn: ({ id, verifyOnly = false, modelIds }: { id: number; verifyOnly?: boolean; modelIds?: string[] }) =>
      providerApi.syncMultiRAG(id, { verifyOnly, modelIds }, options.getOwner?.()).then((res) => unwrapResponse(res)),
    onError: (err, _variables, owner) => { if (!owner || owner.isCurrent()) message.error(parseApiError(err)) },
  })
}
