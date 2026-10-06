import { useQuery } from '@tanstack/react-query'
import { multiragApi, type MultiRAGModel, type MultiRAGModelType } from '@/api/multirag'
import apiClient, { getAccessToken, unwrapResponse } from '@/api/client'
import type { UserInfoResponse } from '@/api/auth'
import type { ApiResponse } from '@/types/api'
import { ownedRequestConfig, type RequestOwner } from '@/api/requestOwnership'
import { useAuthStore } from '@/stores/auth'
import i18next from '@/i18n'

export const multiragKeys = {
  all: ['multirag'] as const,
  models: (type: string) => [...multiragKeys.all, 'models', type] as const,
}

// Credentials stay in private metadata, outside query keys and query data.
const modelOrigins = new WeakMap<MultiRAGModel[], { id: string; token: string | null }>()

export async function readOwnedMultiragModels(type: MultiRAGModelType, signal: AbortSignal): Promise<MultiRAGModel[]> {
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
  const identityResponse = await apiClient.get<ApiResponse<UserInfoResponse>>('/auth/userinfo', ownedRequestConfig(owner))
  owner.assertCurrent()
  const identity = unwrapResponse<UserInfoResponse>(identityResponse)
  if ((identity.user_id ?? identity.id) !== id) throw new Error(i18next.t('knowledge.list.deleteOwnerChanged'))
  const response = await multiragApi.getModels(type, owner)
  owner.assertCurrent()
  const models = unwrapResponse<MultiRAGModel[]>(response)
  if (!Array.isArray(models)) throw new Error(i18next.t('apiErrors.requestFailed'))
  modelOrigins.set(models, { id: id ?? '', token })
  return models
}

export function useMultiragModels(type: MultiRAGModelType) {
  const accountId = useAuthStore((state) => state.user?.id)
  const query = useQuery<MultiRAGModel[]>({
    queryKey: [...multiragKeys.models(type), accountId],
    queryFn: ({ signal }) => readOwnedMultiragModels(type, signal),
    enabled: Boolean(accountId),
    structuralSharing: false,
  })
  const origin = query.data ? modelOrigins.get(query.data) : undefined
  const ownsData = origin?.id === accountId && origin?.token === getAccessToken()
  const ownerChanged = Boolean(query.data) && !ownsData
  return {
    ...query,
    data: ownsData && !query.isError ? query.data : undefined,
    isError: query.isError || ownerChanged,
    isSuccess: query.isSuccess && !ownerChanged,
    error: query.error ?? (ownerChanged ? new Error(i18next.t('knowledge.list.deleteOwnerChanged')) : null),
  }
}
